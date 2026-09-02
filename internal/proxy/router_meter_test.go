package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"switchblade/internal/billing"
	"switchblade/internal/providers"
	"switchblade/internal/reqctx"
)

// stubProvider stands in for a real upstream so the router pipeline can be
// exercised without network access.
type stubProvider struct {
	name string
	fn   func(*providers.ChatRequest) (*providers.ChatResponse, error)
}

func (s *stubProvider) Name() string                                     { return s.name }
func (s *stubProvider) OwnsModel(string) bool                            { return true }
func (s *stubProvider) Healthy(context.Context, *providers.Account) bool { return true }
func (s *stubProvider) Chat(_ context.Context, _ *providers.Account, req *providers.ChatRequest) (*providers.ChatResponse, error) {
	return s.fn(req)
}

// capturingMeter records what the router hands to billing.
type capturingMeter struct{ events []billing.Event }

func (c *capturingMeter) Record(ev billing.Event) { c.events = append(c.events, ev) }

func (c *capturingMeter) only(t *testing.T) billing.Event {
	t.Helper()
	if len(c.events) != 1 {
		t.Fatalf("meter recorded %d events, want 1", len(c.events))
	}
	return c.events[0]
}

func routerWith(p providers.Provider, meter UsageMeter) *Router {
	reg := &providers.Registry{}
	if p != nil {
		reg.Register(p)
	}
	return NewRouter(reg, nil, nil, RouterOptions{Meter: meter})
}

// chatRequest builds an authenticated POST carrying a tenant, as the auth
// middleware would.
func chatRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	ctx := reqctx.WithTenant(r.Context(), "t1")
	ctx = context.WithValue(ctx, reqctx.KeyID, int64(7))
	return r.WithContext(ctx)
}

func TestServeChat_StreamingIsMeteredAndRelayed(t *testing.T) {
	upstream := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":34,\"total_tokens\":46}}\n\n" +
		"data: [DONE]\n\n"

	p := &stubProvider{name: "stub", fn: func(req *providers.ChatRequest) (*providers.ChatResponse, error) {
		if !req.Stream {
			t.Error("stream flag did not reach the provider")
		}
		return &providers.ChatResponse{
			StatusCode: 200,
			Stream:     true,
			BodyStream: io.NopCloser(strings.NewReader(upstream)),
			Headers:    map[string]string{"Content-Type": "text/event-stream"},
		}, nil
	}}

	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(p, meter).ServeChat(w, chatRequest(`{"model":"any","stream":true}`))

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	// The client never opted into usage reporting, so the frame the proxy
	// requested for metering is stripped on the way out; content and the
	// terminator are relayed untouched.
	relayed := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: [DONE]\n\n"
	if w.Body.String() != relayed {
		t.Errorf("relayed body:\n got %q\nwant %q", w.Body.String(), relayed)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if w.Header().Get("X-Request-Id") == "" {
		t.Error("X-Request-Id not set — requests cannot be correlated to their ledger entry")
	}
	if w.Header().Get("X-Switchblade-Provider") != "stub" {
		t.Errorf("X-Switchblade-Provider = %q", w.Header().Get("X-Switchblade-Provider"))
	}

	ev := meter.only(t)
	if ev.Status != "success" {
		t.Errorf("Status = %q, want success", ev.Status)
	}
	if ev.TenantID != "t1" || ev.APIKeyID != 7 {
		t.Errorf("tenant/key = %q/%d, want t1/7", ev.TenantID, ev.APIKeyID)
	}
	if ev.Provider != "stub" || ev.ServiceKind != "chat" {
		t.Errorf("provider/kind = %q/%q", ev.Provider, ev.ServiceKind)
	}
	if ev.Usage.PromptTokens != 12 || ev.Usage.CompletionTokens != 34 || ev.Usage.TotalTokens != 46 {
		t.Errorf("usage = %+v, want {12 34 46} scraped from the stream", ev.Usage)
	}
	if ev.RequestID == "" {
		t.Error("RequestID empty")
	}
}

func TestServeChat_BufferedIsMetered(t *testing.T) {
	body := `{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":9,"total_tokens":12}}`
	p := &stubProvider{name: "stub", fn: func(*providers.ChatRequest) (*providers.ChatResponse, error) {
		return &providers.ChatResponse{StatusCode: 200, Body: []byte(body)}, nil
	}}

	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(p, meter).ServeChat(w, chatRequest(`{"model":"any"}`))

	if w.Body.String() != body {
		t.Errorf("body = %q", w.Body.String())
	}
	ev := meter.only(t)
	if ev.Status != "success" || ev.Usage.TotalTokens != 12 {
		t.Errorf("event = %+v, want success with 12 total tokens", ev)
	}
}

// TestServeChat_UpstreamErrorIsMeteredAsError proves failures still produce an
// audit row — they just are not charged.
func TestServeChat_UpstreamErrorIsMeteredAsError(t *testing.T) {
	p := &stubProvider{name: "stub", fn: func(*providers.ChatRequest) (*providers.ChatResponse, error) {
		return nil, &providers.ProviderError{Code: 429, Message: "rate limited", Provider: "stub"}
	}}

	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(p, meter).ServeChat(w, chatRequest(`{"model":"any"}`))

	if w.Code != 429 {
		t.Errorf("status = %d, want 429", w.Code)
	}
	ev := meter.only(t)
	if ev.Status != "error" {
		t.Errorf("Status = %q, want error", ev.Status)
	}
	if ev.ErrMessage == "" {
		t.Error("ErrMessage empty — the failure reason is lost")
	}
}

func TestServeChat_NoProviderIsMetered(t *testing.T) {
	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(nil, meter).ServeChat(w, chatRequest(`{"model":"nonexistent"}`))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	ev := meter.only(t)
	if ev.Status != "error" || ev.Model != "nonexistent" {
		t.Errorf("event = %+v, want error for model nonexistent", ev)
	}
}

func TestServeChat_MalformedBodyIsMetered(t *testing.T) {
	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(nil, meter).ServeChat(w, chatRequest(`not json`))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if ev := meter.only(t); ev.Status != "error" {
		t.Errorf("Status = %q, want error", ev.Status)
	}
}

// TestServeChat_CacheIsTenantScoped pins the isolation rule: one tenant's
// cached completion must never answer another tenant's identical prompt —
// that is both a data leak and an unbilled request.
func TestServeChat_CacheIsTenantScoped(t *testing.T) {
	body := `{"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	calls := 0
	p := &stubProvider{name: "stub", fn: func(*providers.ChatRequest) (*providers.ChatResponse, error) {
		calls++
		return &providers.ChatResponse{StatusCode: 200, Body: []byte(body)}, nil
	}}

	reg := &providers.Registry{}
	reg.Register(p)
	meter := &capturingMeter{}
	router := NewRouter(reg, nil, nil, RouterOptions{
		Meter: meter,
		Cache: NewResponseCache(300, true),
	})

	serve := func(tenant string) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeChat(w, requestAs(tenant, `{"model":"any"}`))
		if w.Code != 200 {
			t.Fatalf("tenant %q: status %d", tenant, w.Code)
		}
	}

	serve("tenant_a")
	serve("tenant_b")
	if calls != 2 {
		t.Fatalf("provider called %d times — tenant_b was answered from tenant_a's cache", calls)
	}

	serve("tenant_a")
	if calls != 2 {
		t.Errorf("provider called %d times — tenant_a's own repeat should hit its cache", calls)
	}
	if len(meter.events) != 3 || !meter.events[2].Cached {
		t.Errorf("third request not flagged as a cache hit: %+v", meter.events)
	}
}

// TestServeChat_CacheHitIsNotCharged guards the rule that a cached reply is
// logged with its token counts but flagged so the meter skips charging.
func TestServeChat_CacheHitIsNotCharged(t *testing.T) {
	body := `{"usage":{"prompt_tokens":100,"completion_tokens":200,"total_tokens":300}}`
	calls := 0
	p := &stubProvider{name: "stub", fn: func(*providers.ChatRequest) (*providers.ChatResponse, error) {
		calls++
		return &providers.ChatResponse{StatusCode: 200, Body: []byte(body)}, nil
	}}

	reg := &providers.Registry{}
	reg.Register(p)
	meter := &capturingMeter{}
	router := NewRouter(reg, nil, nil, RouterOptions{
		Meter: meter,
		Cache: NewResponseCache(300, true),
	})

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		router.ServeChat(w, chatRequest(`{"model":"any"}`))
		if w.Code != 200 {
			t.Fatalf("request %d: status %d", i, w.Code)
		}
	}

	if calls != 1 {
		t.Errorf("provider called %d times, want 1 — the second request should hit cache", calls)
	}
	if len(meter.events) != 2 {
		t.Fatalf("recorded %d events, want 2", len(meter.events))
	}
	if meter.events[0].Cached {
		t.Error("first request marked cached")
	}
	second := meter.events[1]
	if !second.Cached {
		t.Error("cache hit not flagged — the tenant would be charged for a request that never left the proxy")
	}
	if second.Usage.TotalTokens != 300 {
		t.Errorf("cached usage = %+v, want 300 total tokens recorded for reporting", second.Usage)
	}
}
