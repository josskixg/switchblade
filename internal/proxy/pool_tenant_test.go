package proxy

import (
	"errors"
	"testing"

	"switchblade/internal/db"
)

// insertTenant seeds the tenant row required by the accounts FK.
func insertTenant(t *testing.T, database *db.DB, id string) {
	t.Helper()
	_, err := database.Exec(`INSERT INTO tenants (id, name, email) VALUES (?, ?, ?)
		ON CONFLICT(id) DO NOTHING`, id, id, id+"@test.local")
	if err != nil {
		t.Fatalf("insert tenant %s: %v", id, err)
	}
}

// insertAccount adds an active account owned by tenant and returns its id.
func insertAccount(t *testing.T, database *db.DB, tenant, provider, email, tier string) int64 {
	t.Helper()
	insertTenant(t, database, tenant)
	const now = 1700000000
	res, err := database.Exec(`INSERT INTO accounts (tenant_id, provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES (?, ?, ?, 'pass', 'active', 1, ?, 1000, ?, ?)`, tenant, provider, email, tier, now, now)
	if err != nil {
		t.Fatalf("insert account %s: %v", email, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

// drain picks every account the pool is willing to hand out, so a leak of
// another tenant's credentials cannot hide behind round-robin ordering.
func drain(t *testing.T, p *AccountPool, provider string, n int) map[string]bool {
	t.Helper()
	seen := make(map[string]bool)
	for i := 0; i < n; i++ {
		acc, err := p.Pick(provider)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		seen[acc.Email] = true
	}
	return seen
}

func TestPoolManager_TenantCannotSeeAnotherTenant(t *testing.T) {
	database := testDB(t)
	mgr := NewPoolManager(database, "round_robin")

	insertAccount(t, database, "tenant_a", "openai", "a@test.com", "free")
	insertAccount(t, database, "tenant_b", "openai", "b@test.com", "free")
	insertAccount(t, database, SharedTenant, "openai", "shared@test.com", "free")

	// Twenty picks against a two-account pool cycles the cursor ten times over:
	// any leak would have surfaced.
	got := drain(t, mgr.For("tenant_a", "openai"), "openai", 20)
	if got["b@test.com"] {
		t.Fatal("tenant_a was served tenant_b's account")
	}
	if !got["a@test.com"] || !got["shared@test.com"] {
		t.Fatalf("tenant_a should see its own and the shared account, got %v", got)
	}

	got = drain(t, mgr.For("tenant_b", "openai"), "openai", 20)
	if got["a@test.com"] {
		t.Fatal("tenant_b was served tenant_a's account")
	}
	if !got["b@test.com"] || !got["shared@test.com"] {
		t.Fatalf("tenant_b should see its own and the shared account, got %v", got)
	}
}

func TestPoolManager_SharedPoolExcludesTenantAccounts(t *testing.T) {
	database := testDB(t)
	mgr := NewPoolManager(database, "round_robin")

	insertAccount(t, database, "tenant_a", "openai", "a@test.com", "free")
	insertAccount(t, database, SharedTenant, "openai", "shared@test.com", "free")

	got := drain(t, mgr.Shared("openai"), "openai", 10)
	if got["a@test.com"] {
		t.Fatal("shared pool leaked a tenant-owned account")
	}
	if !got["shared@test.com"] {
		t.Fatalf("shared pool should serve the shared account, got %v", got)
	}
}

// An unauthenticated or legacy-key request carries no tenant, so it must be
// confined to the shared inventory rather than picking up whoever came first.
func TestPoolManager_EmptyTenantResolvesToShared(t *testing.T) {
	database := testDB(t)
	mgr := NewPoolManager(database, "round_robin")

	insertAccount(t, database, "tenant_a", "openai", "a@test.com", "free")
	insertAccount(t, database, SharedTenant, "openai", "shared@test.com", "free")

	pool := mgr.For("", "openai")
	if pool.Tenant() != SharedTenant {
		t.Fatalf("expected tenant %q, got %q", SharedTenant, pool.Tenant())
	}
	if got := drain(t, pool, "openai", 10); got["a@test.com"] {
		t.Fatal("tenantless request was served a tenant-owned account")
	}
}

func TestPoolManager_NoAccountsForForeignTenant(t *testing.T) {
	database := testDB(t)
	mgr := NewPoolManager(database, "round_robin")

	insertAccount(t, database, "tenant_a", "openai", "a@test.com", "free")

	if _, err := mgr.For("tenant_b", "openai").Pick("openai"); !errors.Is(err, ErrNoAccounts) {
		t.Fatalf("expected ErrNoAccounts, got %v", err)
	}
}

func TestPoolManager_ReusesPoolPerKey(t *testing.T) {
	database := testDB(t)
	mgr := NewPoolManager(database, "round_robin")

	if mgr.For("tenant_a", "openai") != mgr.For("tenant_a", "openai") {
		t.Fatal("same (tenant, provider) should resolve to one pool")
	}
	if mgr.For("tenant_a", "openai") == mgr.For("tenant_b", "openai") {
		t.Fatal("different tenants must not share a pool")
	}
	if mgr.For("tenant_a", "openai") == mgr.For("tenant_a", "anthropic") {
		t.Fatal("different providers must not share a pool")
	}
}

func TestPickWithTier_TenantScoped(t *testing.T) {
	database := testDB(t)
	mgr := NewPoolManager(database, "round_robin")

	insertAccount(t, database, "tenant_b", "openai", "b-sub@test.com", "subscription")
	insertAccount(t, database, "tenant_a", "openai", "a-cheap@test.com", "cheap")

	acc, tier, err := mgr.For("tenant_a", "openai").PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier: %v", err)
	}
	// tenant_b's subscription account is invisible, so the preferred tier is empty
	// and selection falls through to tenant_a's own cheap account.
	if tier != "cheap" || acc.Email != "a-cheap@test.com" {
		t.Fatalf("expected tenant_a's cheap account, got %s tier %s", acc.Email, tier)
	}
}

func TestPickWithTier_SharedAccountsVisibleToTenant(t *testing.T) {
	database := testDB(t)
	mgr := NewPoolManager(database, "round_robin")

	insertAccount(t, database, SharedTenant, "openai", "shared-sub@test.com", "subscription")
	insertAccount(t, database, "tenant_a", "openai", "a-free@test.com", "free")

	acc, tier, err := mgr.For("tenant_a", "openai").PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier: %v", err)
	}
	if tier != "subscription" || acc.Email != "shared-sub@test.com" {
		t.Fatalf("expected the shared subscription account, got %s tier %s", acc.Email, tier)
	}
}

// A pool asked for a provider other than the one it last loaded must refill
// rather than answer from the rows it happens to be holding.
func TestAccountPool_ProviderChangeInvalidatesCache(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	insertAccount(t, database, SharedTenant, "openai", "openai@test.com", "free")
	insertAccount(t, database, SharedTenant, "anthropic", "anthropic@test.com", "free")

	if acc, err := pool.Pick("openai"); err != nil || acc.Email != "openai@test.com" {
		t.Fatalf("openai pick: %v %+v", err, acc)
	}
	if acc, err := pool.Pick("anthropic"); err != nil || acc.Email != "anthropic@test.com" {
		t.Fatalf("anthropic pick: %v %+v", err, acc)
	}
}

// Every tenant's pool holds the same shared row, so the lease counter has to be
// common to them all or least_connections reads an account as idle while
// another tenant is still streaming from it.
func TestPoolManager_InFlightSharedAcrossTenants(t *testing.T) {
	database := testDB(t)
	mgr := NewPoolManager(database, "least_connections")

	busy := insertAccount(t, database, SharedTenant, "openai", "busy@test.com", "free")
	insertAccount(t, database, SharedTenant, "openai", "idle@test.com", "free")

	mgr.For("tenant_a", "openai").MarkUsed(busy)

	acc, err := mgr.For("tenant_b", "openai").Pick("openai")
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if acc.ID == busy {
		t.Fatal("least_connections picked an account already in flight for another tenant")
	}
}

func TestAccountPool_MarkDoneEvictsOnError(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	id := insertAccount(t, database, SharedTenant, "openai", "bad@test.com", "free")
	if _, err := pool.Pick("openai"); err != nil {
		t.Fatalf("pick: %v", err)
	}

	pool.MarkDone(id, errors.New("401 unauthorized"))

	pool.mu.Lock()
	remaining := len(pool.accounts)
	pool.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected the failing account to be evicted, %d left", remaining)
	}
}
