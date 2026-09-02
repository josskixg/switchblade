// Package proxy — request scaffolding shared by every /v1 handler.
package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"switchblade/internal/billing"
	"switchblade/internal/config"
	"switchblade/internal/providers"
	"switchblade/internal/reqctx"
)

// newRequestID returns a correlation id shared by the response header, the
// usage record, the audit log, and the ledger entry for one request.
func newRequestID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "req_" + hex.EncodeToString(b[:])
}

// providerTimeout bounds the wait for an upstream response, and doubles as the
// idle budget between SSE frames once one arrives. Without it a provider that
// accepts the connection and never answers pins a goroutine, an account slot and
// its buffers forever — the proxy listener deliberately runs with no write
// deadline so that long streams are not truncated, and cannot save us here. It
// must never bound a whole stream: that would reintroduce, on the outbound leg,
// exactly the mid-stream truncation WriteTimeout 0 exists to prevent.
//
// config.C is nil unless config.Load() ran, which holds in main but not when the
// router is constructed directly.
func providerTimeout() time.Duration {
	if config.C != nil && config.C.ProviderRequestTimeoutMS > 0 {
		return time.Duration(config.C.ProviderRequestTimeoutMS) * time.Millisecond
	}
	return 120 * time.Second
}

// call is the per-request scaffolding chat, embeddings, TTS and STT all share: a
// correlation id the caller can quote in a support ticket, a deadline on the
// upstream, a leased account, and a billing event that is recorded on every exit
// path — early validation failures included, so the audit trail has no holes.
type call struct {
	router *Router
	ev     billing.Event
	start  time.Time

	ctx    context.Context
	cancel context.CancelFunc
	// headerTimer cancels ctx if the upstream never answers. It is a timer
	// rather than a context deadline so a streaming handler can disarm it once
	// headers arrive without also tearing down the connection it is reading.
	headerTimer *time.Timer

	acc     *providers.Account
	release func()
	err     error // terminal upstream error, handed to the pool on release
}

// begin opens the scaffolding for one request. The event starts in the error
// state so a handler that returns before reaching a provider still leaves a row
// behind; success has to be asserted.
func (r *Router) begin(w http.ResponseWriter, req *http.Request, kind string) *call {
	requestID := reqctx.ReqID(req.Context())
	if requestID == "" {
		requestID = newRequestID()
	}
	w.Header().Set("X-Request-Id", requestID)

	ctx, cancel := context.WithCancel(req.Context())
	return &call{
		router:      r,
		start:       time.Now(),
		ctx:         ctx,
		cancel:      cancel,
		headerTimer: time.AfterFunc(providerTimeout(), cancel),
		ev: billing.Event{
			TenantID:    reqctx.Tenant(req.Context()),
			APIKeyID:    reqctx.APIKeyID(req.Context()),
			RequestID:   requestID,
			ServiceKind: kind,
			Status:      "error",
		},
	}
}

// done returns the leased account, releases the deadline and hands the event to
// the meter. Handlers defer it as their first statement, so it runs after any
// response body has been closed.
func (c *call) done() {
	if c.release != nil {
		c.release()
		c.release = nil
	}
	c.headerTimer.Stop()
	c.cancel()

	if c.router.meter == nil {
		return
	}
	c.ev.LatencyMS = time.Since(c.start).Milliseconds()
	c.router.meter.Record(c.ev)
}

// streamStarted lifts the wall-clock deadline once the upstream has answered.
// From here only the upstream closing, the client disconnecting, or frame-level
// silence (the idle watchdog in pipeStream) may end the request.
func (c *call) streamStarted() {
	c.headerTimer.Stop()
}

// fail records why the request stopped and answers the client.
func (c *call) fail(w http.ResponseWriter, code int, msg string) {
	c.ev.ErrMessage = msg
	writeError(w, code, msg)
}

// failUpstream relays a provider failure under its own status where it carried
// one, so an upstream 429 stays a 429 for the client. fallbackCode covers plain
// errors that never reached the wire.
func (c *call) failUpstream(w http.ResponseWriter, err error, fallbackCode int) {
	c.err = err
	var pe *providers.ProviderError
	if errors.As(err, &pe) {
		c.fail(w, pe.Code, pe.Message)
		return
	}
	c.fail(w, fallbackCode, err.Error())
}

// succeed settles the event against the status the upstream actually returned
// and publishes the request to the dashboard feed.
func (c *call) succeed(status int, u billing.Usage) {
	c.ev.Usage = u
	if status < 400 {
		c.ev.Status = "success"
	}
	c.router.broadcast(c.ev.Model, c.ev.Provider, status, u)
}

// route resolves the provider that owns model and records it on the event.
func (c *call) route(w http.ResponseWriter, model string) providers.Provider {
	p := c.router.registry.Route(model)
	if p == nil {
		c.fail(w, http.StatusBadRequest, fmt.Sprintf("no provider for model %q", model))
		return nil
	}
	c.ev.Provider = p.Name()
	return p
}

// lease checks out an account from p's pool for the lifetime of the call.
// Providers without a pool (BYOK) take their credentials from the request
// instead and get a nil account.
func (c *call) lease(w http.ResponseWriter, p providers.Provider) bool {
	pool := c.router.poolFor(c.ev.TenantID, p.Name())
	if pool == nil {
		return true
	}

	acc, err := pool.Pick(p.Name())
	if err != nil {
		if errors.Is(err, ErrNoAccounts) {
			c.fail(w, http.StatusServiceUnavailable, "no active accounts for provider "+p.Name())
		} else {
			c.fail(w, http.StatusInternalServerError, err.Error())
		}
		return false
	}

	c.acc = acc
	c.ev.AccountID, c.ev.AccountEmail = acc.ID, acc.Email
	pool.MarkUsed(acc.ID)
	c.release = func() { pool.MarkDone(acc.ID, c.err) }
	return true
}
