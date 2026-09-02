package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"switchblade/internal/providers"
)

// chainProvider answers for exactly one model so a fallback chain can be built
// out of distinct backends and each hop counted independently.
type chainProvider struct {
	name  string
	model string
	calls int
	fn    func() (*providers.ChatResponse, error)
}

func (p *chainProvider) Name() string                                     { return p.name }
func (p *chainProvider) OwnsModel(m string) bool                          { return m == p.model }
func (p *chainProvider) Healthy(context.Context, *providers.Account) bool { return true }
func (p *chainProvider) Chat(context.Context, *providers.Account, *providers.ChatRequest) (*providers.ChatResponse, error) {
	p.calls++
	return p.fn()
}

func executorFor(t *testing.T, ps ...providers.Provider) *FallbackExecutor {
	t.Helper()
	reg := &providers.Registry{}
	for _, p := range ps {
		reg.Register(p)
	}
	return NewFallbackExecutor(reg, nil, 0, 1000)
}

func statusResponse(code int) func() (*providers.ChatResponse, error) {
	return func() (*providers.ChatResponse, error) {
		return &providers.ChatResponse{StatusCode: code, Body: []byte(`{"error":"upstream said so"}`)}, nil
	}
}

// TestExecute_StatusDecidesWhetherTheChainAdvances pins the contract the whole
// pool depends on: a provider reports an upstream 4xx/5xx as a successful
// exchange carrying the status, so the executor — not the transport — decides
// which of those are worth another backend.
func TestExecute_StatusDecidesWhetherTheChainAdvances(t *testing.T) {
	tests := []struct {
		name          string
		primaryStatus int
		wantSecondary bool
		wantStatus    int
		wantProvider  string
	}{
		{"success short-circuits", 200, false, 200, "primary"},
		{"request timeout advances", http.StatusRequestTimeout, true, 200, "secondary"},
		{"conflict advances", http.StatusConflict, true, 200, "secondary"},
		{"throttled advances", http.StatusTooManyRequests, true, 200, "secondary"},
		{"internal error advances", http.StatusInternalServerError, true, 200, "secondary"},
		{"bad gateway advances", http.StatusBadGateway, true, 200, "secondary"},
		{"service unavailable advances", http.StatusServiceUnavailable, true, 200, "secondary"},
		{"bad request relayed as-is", http.StatusBadRequest, false, 400, "primary"},
		{"unauthorized relayed as-is", http.StatusUnauthorized, false, 401, "primary"},
		{"forbidden relayed as-is", http.StatusForbidden, false, 403, "primary"},
		{"not found relayed as-is", http.StatusNotFound, false, 404, "primary"},
		{"unprocessable relayed as-is", http.StatusUnprocessableEntity, false, 422, "primary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &chainProvider{name: "primary", model: "a", fn: statusResponse(tt.primaryStatus)}
			secondary := &chainProvider{name: "secondary", model: "b", fn: statusResponse(200)}

			out, err := executorFor(t, primary, secondary).
				Execute(context.Background(), &providers.ChatRequest{}, []string{"a", "b"})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if out.Response.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", out.Response.StatusCode, tt.wantStatus)
			}
			if out.Provider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", out.Provider, tt.wantProvider)
			}
			if got := secondary.calls > 0; got != tt.wantSecondary {
				t.Errorf("secondary attempted = %v, want %v", got, tt.wantSecondary)
			}
			if want := 1; primary.calls != want {
				t.Errorf("primary calls = %d, want %d", primary.calls, want)
			}
		})
	}
}

// TestExecute_ExhaustedChainKeepsTheUpstreamStatus guards against a chain that
// ran out of capacity reaching the client as a generic gateway failure — the
// caller needs the 429 to know it should back off.
func TestExecute_ExhaustedChainKeepsTheUpstreamStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantCode int
	}{
		{"all throttled", http.StatusTooManyRequests, http.StatusTooManyRequests},
		{"all unavailable", http.StatusServiceUnavailable, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &chainProvider{name: "primary", model: "a", fn: statusResponse(tt.status)}
			secondary := &chainProvider{name: "secondary", model: "b", fn: statusResponse(tt.status)}

			out, err := executorFor(t, primary, secondary).
				Execute(context.Background(), &providers.ChatRequest{}, []string{"a", "b"})
			if err == nil {
				t.Fatal("Execute returned nil error after exhausting the chain")
			}

			var pe *providers.ProviderError
			if !errors.As(err, &pe) {
				t.Fatalf("error = %T (%v), want *providers.ProviderError", err, err)
			}
			if pe.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", pe.Code, tt.wantCode)
			}
			if !pe.Retryable {
				t.Error("Retryable = false — the status was classified as worth retrying")
			}
			if len(out.Attempted) != 2 {
				t.Errorf("attempted = %v, want both chain entries recorded", out.Attempted)
			}
		})
	}
}

// TestExecute_ProviderErrorRetryability keeps the pre-existing contract: an
// error the provider raised itself governs the chain through its own Retryable
// flag, independently of any HTTP status.
func TestExecute_ProviderErrorRetryability(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantSecondary bool
		wantCode      int
	}{
		{
			name:          "transport failure advances",
			err:           &providers.ProviderError{Code: 502, Message: "dial tcp: refused", Provider: "primary", Retryable: true},
			wantSecondary: true,
		},
		{
			name:          "invalid credentials stop the chain",
			err:           &providers.ProviderError{Code: 401, Message: "invalid tokens", Provider: "primary"},
			wantSecondary: false,
			wantCode:      401,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &chainProvider{name: "primary", model: "a", fn: func() (*providers.ChatResponse, error) {
				return nil, tt.err
			}}
			secondary := &chainProvider{name: "secondary", model: "b", fn: statusResponse(200)}

			out, err := executorFor(t, primary, secondary).
				Execute(context.Background(), &providers.ChatRequest{}, []string{"a", "b"})

			if got := secondary.calls > 0; got != tt.wantSecondary {
				t.Errorf("secondary attempted = %v, want %v", got, tt.wantSecondary)
			}
			if tt.wantSecondary {
				if err != nil {
					t.Fatalf("Execute: %v", err)
				}
				if out.Provider != "secondary" {
					t.Errorf("provider = %q, want secondary", out.Provider)
				}
				return
			}

			var pe *providers.ProviderError
			if !errors.As(err, &pe) || pe.Code != tt.wantCode {
				t.Fatalf("error = %v, want *ProviderError with code %d", err, tt.wantCode)
			}
		})
	}
}

func TestExecute_EmptyChain(t *testing.T) {
	if _, err := executorFor(t).Execute(context.Background(), &providers.ChatRequest{}, nil); err == nil {
		t.Fatal("empty chain accepted")
	}
}

// TestExecute_UnroutableModelsExhaustTheChain covers the case where nothing in
// the chain has a backend at all: there is no upstream status to report, so the
// generic exhaustion error stands.
func TestExecute_UnroutableModelsExhaustTheChain(t *testing.T) {
	out, err := executorFor(t).Execute(context.Background(), &providers.ChatRequest{}, []string{"a", "b"})
	if err == nil {
		t.Fatal("Execute returned nil error for an unroutable chain")
	}
	if len(out.Attempted) != 0 {
		t.Errorf("attempted = %v, want empty", out.Attempted)
	}
}

// liveProvider issues a real HTTP request bound to the attempt context, which
// is the only way to reproduce the teardown ordering that matters here.
type liveProvider struct{ url string }

func (p *liveProvider) Name() string                                     { return "live" }
func (p *liveProvider) OwnsModel(string) bool                            { return true }
func (p *liveProvider) Healthy(context.Context, *providers.Account) bool { return true }
func (p *liveProvider) Chat(ctx context.Context, _ *providers.Account, req *providers.ChatRequest) (*providers.ChatResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	return providers.Exchange(httpReq, "live", req.Stream)
}

// TestExecute_StreamOutlivesTheAttemptContext is the regression guard for the
// per-attempt deadline being released too early. Chat returns the moment
// response headers arrive; the body is still an open connection owned by the
// attempt context, so tearing that context down there would leave the router
// reading a cancelled stream and every streaming request would answer empty.
func TestExecute_StreamOutlivesTheAttemptContext(t *testing.T) {
	const payload = "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		// Headers land well before the first token, exactly as a real prefill does.
		time.Sleep(30 * time.Millisecond)
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()

	out, err := executorFor(t, &liveProvider{url: srv.URL}).
		Execute(context.Background(), &providers.ChatRequest{Stream: true}, []string{"any"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Response.BodyStream == nil {
		t.Fatal("no BodyStream — the streaming path was not taken")
	}

	got, err := io.ReadAll(out.Response.BodyStream)
	if err != nil {
		t.Fatalf("reading the stream after Execute returned: %v", err)
	}
	if string(got) != payload {
		t.Errorf("stream = %q, want %q", got, payload)
	}

	// Closing releases the deadline and the account; the router closes once, but
	// doing it twice must not panic or double-release.
	if err := out.Response.BodyStream.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	_ = out.Response.BodyStream.Close()
}

func TestRetryableStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false}, {204, false}, {301, false},
		{400, false}, {401, false}, {403, false}, {404, false}, {422, false},
		{408, true}, {409, true}, {429, true},
		{500, true}, {502, true}, {503, true}, {504, true}, {529, true},
	}
	for _, tt := range tests {
		if got := retryableStatus(tt.code); got != tt.want {
			t.Errorf("retryableStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}
