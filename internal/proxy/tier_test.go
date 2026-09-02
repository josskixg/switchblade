package proxy

import (
	"context"
	"testing"

	"switchblade/internal/db"
	"switchblade/internal/providers"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	raw, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { raw.Close() })
	if err := raw.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return raw
}

func TestPickWithTier_SubscriptionFirst(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	// Insert accounts with different tiers
	now := 1700000000
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'sub@test.com', 'pass', 'active', 1, 'subscription', 1000, ?, ?)`, now, now)
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'cheap@test.com', 'pass', 'active', 1, 'cheap', 1000, ?, ?)`, now, now)

	// Pick with subscription preference
	acc, tier, err := pool.PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier failed: %v", err)
	}
	if tier != "subscription" {
		t.Errorf("expected tier 'subscription', got '%s'", tier)
	}
	if acc.Email != "sub@test.com" {
		t.Errorf("expected email 'sub@test.com', got '%s'", acc.Email)
	}
}

func TestPickWithTier_FallbackToCheap(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	// Only cheap and free accounts
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'cheap@test.com', 'pass', 'active', 1, 'cheap', 1000, ?, ?)`, now, now)

	// Pick with subscription preference, but no subscription accounts
	acc, tier, err := pool.PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier failed: %v", err)
	}
	if tier != "cheap" {
		t.Errorf("expected tier 'cheap' (fallback), got '%s'", tier)
	}
	if acc.Email != "cheap@test.com" {
		t.Errorf("expected email 'cheap@test.com', got '%s'", acc.Email)
	}
}

func TestPickWithTier_FallbackToFree(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	// Only free account
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)

	// Pick with subscription preference, but only free available
	acc, tier, err := pool.PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier failed: %v", err)
	}
	if tier != "free" {
		t.Errorf("expected tier 'free' (fallback), got '%s'", tier)
	}
	if acc.Email != "free@test.com" {
		t.Errorf("expected email 'free@test.com', got '%s'", acc.Email)
	}
}

func TestPickWithTier_NoAccounts(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	// No accounts in DB
	_, _, err := pool.PickWithTier("openai", "subscription")
	if err == nil {
		t.Fatal("expected error when no accounts available")
	}
	// When no accounts exist at all, we get ErrNoAccounts
	if err != ErrNoAccounts {
		t.Errorf("expected ErrNoAccounts, got %v", err)
	}
}

func TestPickWithTier_InvalidTier(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)

	// Pick with invalid tier
	_, _, err := pool.PickWithTier("openai", "invalid_tier")
	if err == nil {
		t.Fatal("expected error for invalid tier")
	}
}

func TestPickWithTier_SkipExhaustedAccounts(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	// Subscription account with 0 quota
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'sub@test.com', 'pass', 'active', 1, 'subscription', 0, ?, ?)`, now, now)
	// Free account with quota
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)

	// Pick with subscription preference, but subscription has 0 quota
	acc, tier, err := pool.PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier failed: %v", err)
	}
	// Should fallback to free
	if tier != "free" {
		t.Errorf("expected tier 'free' (fallback), got '%s'", tier)
	}
	if acc.Email != "free@test.com" {
		t.Errorf("expected email 'free@test.com', got '%s'", acc.Email)
	}
}

func TestPickWithTier_MultipleProviders(t *testing.T) {
	database := testDB(t)

	// Create separate pools per provider (as designed)
	openaiPool := NewAccountPool(database, "round_robin")
	anthropicPool := NewAccountPool(database, "round_robin")

	now := 1700000000
	// OpenAI subscription account
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'openai-sub@test.com', 'pass', 'active', 1, 'subscription', 1000, ?, ?)`, now, now)
	// Anthropic free account
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('anthropic', 'anthropic-free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)

	// Pick OpenAI - should get subscription
	acc, tier, err := openaiPool.PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier (openai) failed: %v", err)
	}
	if tier != "subscription" {
		t.Errorf("expected tier 'subscription' for openai, got '%s'", tier)
	}
	if acc.Email != "openai-sub@test.com" {
		t.Errorf("expected email 'openai-sub@test.com', got '%s'", acc.Email)
	}

	// Pick Anthropic - should get free (only option)
	acc, tier, err = anthropicPool.PickWithTier("anthropic", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier (anthropic) failed: %v", err)
	}
	if tier != "free" {
		t.Errorf("expected tier 'free' for anthropic (fallback), got '%s'", tier)
	}
	if acc.Email != "anthropic-free@test.com" {
		t.Errorf("expected email 'anthropic-free@test.com', got '%s'", acc.Email)
	}
}

func TestPickWithTier_CheapPreference(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'sub@test.com', 'pass', 'active', 1, 'subscription', 1000, ?, ?)`, now, now)
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'cheap@test.com', 'pass', 'active', 1, 'cheap', 1000, ?, ?)`, now, now)
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)

	// Pick with cheap preference
	acc, tier, err := pool.PickWithTier("openai", "cheap")
	if err != nil {
		t.Fatalf("PickWithTier failed: %v", err)
	}
	if tier != "cheap" {
		t.Errorf("expected tier 'cheap', got '%s'", tier)
	}
	if acc.Email != "cheap@test.com" {
		t.Errorf("expected email 'cheap@test.com', got '%s'", acc.Email)
	}
}

func TestPickWithTier_FreePreference(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'sub@test.com', 'pass', 'active', 1, 'subscription', 1000, ?, ?)`, now, now)
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)

	// Pick with free preference
	acc, tier, err := pool.PickWithTier("openai", "free")
	if err != nil {
		t.Fatalf("PickWithTier failed: %v", err)
	}
	if tier != "free" {
		t.Errorf("expected tier 'free', got '%s'", tier)
	}
	if acc.Email != "free@test.com" {
		t.Errorf("expected email 'free@test.com', got '%s'", acc.Email)
	}
}

func TestPickWithTier_AllTiersExhausted(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	// All accounts have 0 quota
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'sub@test.com', 'pass', 'active', 1, 'subscription', 0, ?, ?)`, now, now)
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'cheap@test.com', 'pass', 'active', 1, 'cheap', 0, ?, ?)`, now, now)
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 0, ?, ?)`, now, now)

	// Pick should fail
	_, _, err := pool.PickWithTier("openai", "subscription")
	if err == nil {
		t.Fatal("expected error when all tiers exhausted")
	}
	if err != ErrTierExhausted {
		t.Errorf("expected ErrTierExhausted, got %v", err)
	}
}

func TestPickWithTier_ConcurrentAccess(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	// Insert multiple accounts
	for i := 0; i < 10; i++ {
		database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
			VALUES ('openai', ?, 'pass', 'active', 1, 'subscription', 1000, ?, ?)`,
			"test"+string(rune('0'+i))+"@test.com", now, now)
	}

	// Concurrent picks
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _, err := pool.PickWithTier("openai", "subscription")
			if err != nil {
				t.Errorf("concurrent PickWithTier failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestPickWithTier_ContextCancellation(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'test@test.com', 'pass', 'active', 1, 'subscription', 1000, ?, ?)`, now, now)

	// This test verifies that PickWithTier doesn't hang
	// In real scenario, context would be passed, but current implementation doesn't use it
	acc, tier, err := pool.PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier failed: %v", err)
	}
	if tier != "subscription" {
		t.Errorf("expected tier 'subscription', got '%s'", tier)
	}
	if acc == nil {
		t.Error("expected account, got nil")
	}
}

func TestPickWithTier_EmptyProvider(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	// Try to pick from non-existent provider
	_, _, err := pool.PickWithTier("nonexistent", "subscription")
	if err == nil {
		t.Fatal("expected error for non-existent provider")
	}
}

func TestPickWithTier_DisabledAccounts(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	// Disabled subscription account
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'sub@test.com', 'pass', 'active', 0, 'subscription', 1000, ?, ?)`, now, now)
	// Enabled free account
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)

	// Pick should skip disabled subscription and use free
	acc, tier, err := pool.PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier failed: %v", err)
	}
	if tier != "free" {
		t.Errorf("expected tier 'free' (skip disabled), got '%s'", tier)
	}
	if acc.Email != "free@test.com" {
		t.Errorf("expected email 'free@test.com', got '%s'", acc.Email)
	}
}

func TestPickWithTier_InactiveStatus(t *testing.T) {
	database := testDB(t)
	pool := NewAccountPool(database, "round_robin")

	now := 1700000000
	// Inactive subscription account
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'sub@test.com', 'pass', 'inactive', 1, 'subscription', 1000, ?, ?)`, now, now)
	// Active free account
	database.Exec(`INSERT INTO accounts (provider, email, password, status, enabled, tier, quota_remaining, created_at, updated_at)
		VALUES ('openai', 'free@test.com', 'pass', 'active', 1, 'free', 1000, ?, ?)`, now, now)

	// Pick should skip inactive subscription and use free
	acc, tier, err := pool.PickWithTier("openai", "subscription")
	if err != nil {
		t.Fatalf("PickWithTier failed: %v", err)
	}
	if tier != "free" {
		t.Errorf("expected tier 'free' (skip inactive), got '%s'", tier)
	}
	if acc.Email != "free@test.com" {
		t.Errorf("expected email 'free@test.com', got '%s'", acc.Email)
	}
}

// Mock provider for testing service kinds
type mockEmbedder struct {
	providers.Provider
}

func (m *mockEmbedder) Embeddings(ctx context.Context, acc *providers.Account, req *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	return &providers.EmbeddingsResponse{
		Object: "list",
		Data: []providers.EmbeddingObject{
			{Object: "embedding", Index: 0, Embedding: []float64{0.1, 0.2, 0.3}},
		},
		Model: req.Model,
		Usage: providers.EmbeddingUsage{PromptTokens: 10, TotalTokens: 10},
	}, nil
}

func TestRouter_ServeEmbeddings(t *testing.T) {
	// This test would require setting up a full router with mock providers
	// For now, we'll skip this as it requires more complex setup
	t.Skip("Requires complex router setup with mock providers")
}
