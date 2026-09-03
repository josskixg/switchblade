// Package proxy — model combos and streaming fallback engine.
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"switchblade/internal/db"
	"switchblade/internal/providers"
	"switchblade/internal/reqctx"
)

// Combo is a named fallback chain stored in model_combos table.
type Combo struct {
	Name   string
	Models []string // ordered fallback list
}

// ComboStore loads combos from DB with a short cache.
type ComboStore struct {
	db       *db.DB
	combos   []Combo
	cachedAt time.Time
	ttl      time.Duration
}

// NewComboStore creates a combo store with 10s TTL.
func NewComboStore(database *db.DB) *ComboStore {
	return &ComboStore{db: database, ttl: 10 * time.Second}
}

// FindChain returns the fallback model list for a given model name.
// Returns nil if no combo matches.
func (cs *ComboStore) FindChain(model string) []string {
	cs.maybeReload()
	for _, c := range cs.combos {
		if c.Name == model || (len(c.Models) > 0 && c.Models[0] == model) {
			return c.Models
		}
	}
	return nil
}

func (cs *ComboStore) maybeReload() {
	if time.Since(cs.cachedAt) < cs.ttl {
		return
	}
	rows, err := cs.db.Query(`SELECT name, models_json FROM model_combos WHERE enabled = 1`)
	if err != nil {
		return
	}
	defer rows.Close()
	var combos []Combo
	for rows.Next() {
		var name, modelsJSON string
		if err := rows.Scan(&name, &modelsJSON); err != nil {
			continue
		}
		var models []string
		if err := json.Unmarshal([]byte(modelsJSON), &models); err != nil {
			continue
		}
		combos = append(combos, Combo{Name: name, Models: models})
	}
	cs.combos = combos
	cs.cachedAt = time.Now()
}

// FallbackExecutor runs a request through a chain of providers with retries.
type FallbackExecutor struct {
	registry    *providers.Registry
	pools       map[string]*AccountPool
	poolSrc     PoolSource
	maxAttempts int
	timeoutMS   int
}

// NewFallbackExecutor creates an executor over a static provider → pool map.
// The map cannot keep tenants apart; NewTenantFallbackExecutor is the
// multi-tenant wiring.
func NewFallbackExecutor(registry *providers.Registry, pools map[string]*AccountPool, maxAttempts, timeoutMS int) *FallbackExecutor {
	return &FallbackExecutor{
		registry:    registry,
		pools:       pools,
		maxAttempts: maxAttempts,
		timeoutMS:   timeoutMS,
	}
}

// NewTenantFallbackExecutor creates an executor that resolves each attempt's
// pool from the requesting tenant, read from the context Execute is given.
// Fallback spends other backends' credentials on the caller's behalf, so it
// has to honour the same ownership boundary as the primary path — otherwise
// every retry is a cross-tenant leak.
func NewTenantFallbackExecutor(registry *providers.Registry, pools PoolSource, maxAttempts, timeoutMS int) *FallbackExecutor {
	return &FallbackExecutor{
		registry:    registry,
		poolSrc:     pools,
		maxAttempts: maxAttempts,
		timeoutMS:   timeoutMS,
	}
}

// poolFor mirrors Router.poolFor: nil means the provider is unpooled.
func (fe *FallbackExecutor) poolFor(tenant, provider string) *AccountPool {
	if fe.poolSrc != nil {
		return fe.poolSrc.For(tenant, provider)
	}
	return fe.pools[provider]
}

// FallbackOutcome reports what actually served the request. The account is part
// of it so usage can be attributed to the credential that paid for the call
// rather than only to the chain as a whole.
type FallbackOutcome struct {
	Response  *providers.ChatResponse
	Provider  string
	Model     string
	Account   *providers.Account
	Attempted []string
}

// retryableStatus reports whether an upstream status is worth spending the next
// entry of the chain on. A timeout, a conflict, a throttle or a server fault is
// transient and a different provider may well answer; every other 4xx is the
// caller's own request coming back and moving it to another backend would only
// produce the same rejection under a different name.
func retryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests:
		return true
	}
	return code >= 500
}

// upstreamMessage renders a failed exchange for the audit log, falling back to
// the status text when the provider sent nothing back.
func upstreamMessage(code int, body []byte) string {
	if len(body) == 0 {
		return http.StatusText(code)
	}
	return truncateBody(body)
}

func closeBody(resp *providers.ChatResponse) {
	if resp != nil && resp.BodyStream != nil {
		_ = resp.BodyStream.Close()
	}
}

// streamGuard defers the teardown of an attempt until its body has been drained.
//
// A streaming Chat returns the moment response headers arrive, with the body
// still an open connection bound to the attempt's context and the account still
// in flight. Cancelling that context or releasing the account there kills the
// stream the router is about to read and reports the account idle while it is
// still generating, so both are attached to Close instead.
type streamGuard struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (g *streamGuard) Close() error {
	err := g.ReadCloser.Close()
	g.once.Do(g.release)
	return err
}

// Execute attempts the request on the primary provider, then falls back through
// the chain. On success the outcome carries the winning provider, model and
// account; on failure it still carries what was attempted so the audit trail
// shows the cost of getting there.
func (fe *FallbackExecutor) Execute(
	ctx context.Context,
	req *providers.ChatRequest,
	chain []string,
) (*FallbackOutcome, error) {
	if len(chain) == 0 {
		return nil, errors.New("empty fallback chain")
	}

	max := fe.maxAttempts
	if max <= 0 || max > len(chain) {
		max = len(chain)
	}

	out := &FallbackOutcome{Attempted: make([]string, 0, max)}
	timeout := time.Duration(fe.timeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	var lastErr error
	for i := 0; i < max; i++ {
		model := chain[i]
		p := fe.registry.Route(model)
		if p == nil {
			slog.Info(fmt.Sprintf("[fallback] no provider for model %q, skipping", model))
			continue
		}

		out.Attempted = append(out.Attempted, p.Name()+"/"+model)

		// Pick account
		var acc *providers.Account
		pool := fe.poolFor(reqctx.Tenant(ctx), p.Name())
		if pool != nil {
			var err error
			acc, err = pool.Pick(p.Name())
			if err != nil {
				slog.Warn("[fallback] no accounts for provider", "provider", p.Name(), "err", err)
				lastErr = err
				continue
			}
			pool.MarkUsed(acc.ID)
		}

		// The per-attempt timer bounds the wait for a response only. It is a
		// timer rather than a context deadline so a streaming attempt can be
		// disarmed once headers arrive — a deadline would keep ticking under
		// the handed-off body and cut the generation mid-stream.
		attemptCtx, cancel := context.WithCancel(ctx)
		attemptTimer := time.AfterFunc(timeout, cancel)
		attemptReq := &providers.ChatRequest{
			Model:    model,
			Messages: req.Messages,
			Stream:   req.Stream,
			Body:     req.Body,
		}
		resp, err := p.Chat(attemptCtx, acc, attemptReq)

		release := func(cause error) {
			attemptTimer.Stop()
			cancel()
			if pool != nil && acc != nil {
				pool.MarkDone(acc.ID, cause)
			}
		}

		// Providers surface an upstream 4xx/5xx as a successful exchange carrying
		// the status, so the chain classifies it here rather than trusting err
		// alone — otherwise only DNS and TCP failures ever advance the chain, and
		// an account whose key was revoked stays in rotation answering 401s.
		if err == nil && resp != nil && retryableStatus(resp.StatusCode) {
			err = &providers.ProviderError{
				Code:      resp.StatusCode,
				Message:   upstreamMessage(resp.StatusCode, resp.Body),
				Provider:  p.Name(),
				Retryable: true,
			}
		}

		if err != nil {
			closeBody(resp)
			release(err)
			lastErr = err

			var pe *providers.ProviderError
			if errors.As(err, &pe) && !pe.Retryable {
				slog.Error("[fallback] non-retryable error", "provider", p.Name(), "err", err)
				return out, err
			}
			slog.Warn("[fallback] attempt failed", "attempt", i+1, "max", max, "provider", p.Name(), "err", err)
			continue
		}

		if resp != nil && resp.BodyStream != nil {
			attemptTimer.Stop()
			resp.BodyStream = &streamGuard{ReadCloser: resp.BodyStream, release: func() { release(nil) }}
		} else {
			release(nil)
		}

		out.Response, out.Provider, out.Model, out.Account = resp, p.Name(), model, acc
		return out, nil
	}

	// Surfacing the last upstream error keeps its status: a chain that ran out of
	// capacity should reach the client as 429, not as a generic gateway failure.
	if lastErr != nil {
		return out, lastErr
	}
	return out, errors.New("all fallback attempts exhausted")
}
