package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"switchblade/internal/db"
)

func setupTestDB(t *testing.T) *db.DB {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create tables
	_, err = database.Exec(`
		CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tenant_id TEXT NOT NULL DEFAULT '_system',
			provider TEXT NOT NULL,
			email TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			enabled INTEGER NOT NULL DEFAULT 1,
			quota_limit REAL DEFAULT 0,
			quota_remaining REAL DEFAULT 0,
			tier TEXT DEFAULT 'free'
		);
		CREATE TABLE IF NOT EXISTS request_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tenant_id TEXT NOT NULL DEFAULT '_system',
			account_id INTEGER,
			provider TEXT NOT NULL,
			model TEXT,
			service_kind TEXT DEFAULT 'chat',
			status TEXT NOT NULL,
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		);
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	return database
}

func TestHandleTierQuotaStatus(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	// Insert test accounts with different tiers
	_, err := database.Exec(`
		INSERT INTO accounts (provider, email, status, enabled, quota_limit, quota_remaining, tier) VALUES
		('openai', 'test1@example.com', 'active', 1, 1000, 500, 'subscription'),
		('openai', 'test2@example.com', 'active', 1, 1000, 300, 'subscription'),
		('anthropic', 'test3@example.com', 'active', 1, 500, 200, 'cheap'),
		('openai', 'test4@example.com', 'active', 1, 200, 100, 'free'),
		('openai', 'test5@example.com', 'inactive', 1, 100, 50, 'free')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	handler := HandleTierQuotaStatus(database)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tiers/quota-status", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var quotas []struct {
		Tier           string  `json:"tier"`
		TotalAccounts  int     `json:"total_accounts"`
		ActiveAccounts int     `json:"active_accounts"`
		TotalQuota     float64 `json:"total_quota"`
		UsedQuota      float64 `json:"used_quota"`
		RemainingQuota float64 `json:"remaining_quota"`
	}

	if err := json.NewDecoder(w.Body).Decode(&quotas); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify we got data for all tiers
	if len(quotas) == 0 {
		t.Error("expected quota data, got empty array")
	}

	// Find subscription tier
	var subQuota *struct {
		Tier           string  `json:"tier"`
		TotalAccounts  int     `json:"total_accounts"`
		ActiveAccounts int     `json:"active_accounts"`
		TotalQuota     float64 `json:"total_quota"`
		UsedQuota      float64 `json:"used_quota"`
		RemainingQuota float64 `json:"remaining_quota"`
	}
	for i := range quotas {
		if quotas[i].Tier == "subscription" {
			subQuota = &quotas[i]
			break
		}
	}

	if subQuota == nil {
		t.Fatal("subscription tier not found in response")
	}

	if subQuota.TotalAccounts != 2 {
		t.Errorf("expected 2 subscription accounts, got %d", subQuota.TotalAccounts)
	}

	if subQuota.ActiveAccounts != 2 {
		t.Errorf("expected 2 active subscription accounts, got %d", subQuota.ActiveAccounts)
	}

	if subQuota.TotalQuota != 2000 {
		t.Errorf("expected total quota 2000, got %f", subQuota.TotalQuota)
	}

	expectedUsed := 1200.0 // (1000-500) + (1000-300)
	if subQuota.UsedQuota != expectedUsed {
		t.Errorf("expected used quota %f, got %f", expectedUsed, subQuota.UsedQuota)
	}

	expectedRemaining := 800.0 // 500 + 300
	if subQuota.RemainingQuota != expectedRemaining {
		t.Errorf("expected remaining quota %f, got %f", expectedRemaining, subQuota.RemainingQuota)
	}
}

func TestHandleTierQuotaStatus_EmptyDB(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	handler := HandleTierQuotaStatus(database)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tiers/quota-status", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var quotas []interface{}
	if err := json.NewDecoder(w.Body).Decode(&quotas); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return empty array, not error
	if quotas == nil {
		t.Error("expected empty array, got nil")
	}
}
