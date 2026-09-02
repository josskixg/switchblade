package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"switchblade/internal/db"
	"switchblade/internal/providers"
	"switchblade/internal/reqctx"
)

// leaseStub records the email of every account it is handed, so a test can
// prove which credential actually served each request.
type leaseStub struct {
	name   string
	served []string
}

func (s *leaseStub) Name() string                                     { return s.name }
func (s *leaseStub) OwnsModel(string) bool                            { return true }
func (s *leaseStub) Healthy(context.Context, *providers.Account) bool { return true }
func (s *leaseStub) Chat(_ context.Context, acc *providers.Account, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
	if acc != nil {
		s.served = append(s.served, acc.Email)
	}
	return &providers.ChatResponse{
		StatusCode: 200,
		Body:       []byte(`{"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`),
	}, nil
}

// requestAs builds an authenticated chat request on behalf of tenant.
func requestAs(tenant, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	return r.WithContext(reqctx.WithTenant(r.Context(), tenant))
}

func tenantRouterFixture(t *testing.T, fallbackChain bool) (*leaseStub, *Router, *db.DB) {
	t.Helper()
	database := testDB(t)
	insertAccount(t, database, "tenant_a", "stub", "a@test.com", "free")
	insertAccount(t, database, "tenant_b", "stub", "b@test.com", "free")

	stub := &leaseStub{name: "stub"}
	reg := &providers.Registry{}
	reg.Register(stub)

	mgr := NewPoolManager(database, "round_robin")
	opts := RouterOptions{Pools: mgr}
	if fallbackChain {
		opts.Fallback = NewTenantFallbackExecutor(reg, mgr, 0, 1000)
	}
	return stub, NewRouter(reg, nil, nil, opts), database
}

// TestServeChat_PrimaryPathIsTenantScoped proves a request carrying tenant A's
// id is never served by an account owned only by tenant B on the direct path.
func TestServeChat_PrimaryPathIsTenantScoped(t *testing.T) {
	stub, router, _ := tenantRouterFixture(t, false)

	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		router.ServeChat(w, requestAs("tenant_a", `{"model":"any"}`))
		if w.Code != 200 {
			t.Fatalf("request %d: status %d, body %s", i, w.Code, w.Body.String())
		}
	}

	if len(stub.served) != 10 {
		t.Fatalf("provider saw %d accounts, want 10", len(stub.served))
	}
	for _, email := range stub.served {
		if email != "a@test.com" {
			t.Fatalf("tenant_a request served by %q", email)
		}
	}
}

// A tenant with no accounts of its own (and no shared inventory) must be
// refused rather than quietly handed another tenant's credentials.
func TestServeChat_PrimaryPathRefusesForeignTenant(t *testing.T) {
	stub, router, _ := tenantRouterFixture(t, false)

	w := httptest.NewRecorder()
	router.ServeChat(w, requestAs("tenant_c", `{"model":"any"}`))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if len(stub.served) != 0 {
		t.Fatalf("provider was reached with accounts %v", stub.served)
	}
}

// TestServeChat_FallbackPathIsTenantScoped hunts the failure mode where the
// primary path is tenant-scoped but the fallback executor still draws from a
// static pool and re-opens the leak.
func TestServeChat_FallbackPathIsTenantScoped(t *testing.T) {
	stub, router, _ := tenantRouterFixture(t, true)

	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		router.ServeChat(w, requestAs("tenant_a", `{"model":"any"}`))
		if w.Code != 200 {
			t.Fatalf("request %d: status %d, body %s", i, w.Code, w.Body.String())
		}
	}

	if len(stub.served) != 10 {
		t.Fatalf("provider saw %d accounts, want 10", len(stub.served))
	}
	for _, email := range stub.served {
		if email != "a@test.com" {
			t.Fatalf("tenant_a request served by %q via fallback", email)
		}
	}
}

func TestServeChat_FallbackPathRefusesForeignTenant(t *testing.T) {
	stub, router, _ := tenantRouterFixture(t, true)

	w := httptest.NewRecorder()
	router.ServeChat(w, requestAs("tenant_c", `{"model":"any"}`))
	if w.Code < 400 {
		t.Fatalf("status = %d, want an error", w.Code)
	}
	if len(stub.served) != 0 {
		t.Fatalf("provider was reached with accounts %v", stub.served)
	}
}

// Shared accounts stay visible to every tenant through the manager.
func TestServeChat_SharedAccountServesAllTenants(t *testing.T) {
	database := testDB(t)
	insertAccount(t, database, SharedTenant, "stub", "shared@test.com", "free")

	stub := &leaseStub{name: "stub"}
	reg := &providers.Registry{}
	reg.Register(stub)
	router := NewRouter(reg, nil, nil, RouterOptions{Pools: NewPoolManager(database, "round_robin")})

	for _, tenant := range []string{"tenant_a", "tenant_b", ""} {
		w := httptest.NewRecorder()
		router.ServeChat(w, requestAs(tenant, `{"model":"any"}`))
		if w.Code != 200 {
			t.Fatalf("tenant %q: status %d", tenant, w.Code)
		}
	}
	for _, email := range stub.served {
		if email != "shared@test.com" {
			t.Fatalf("served by %q, want the shared account", email)
		}
	}
}
