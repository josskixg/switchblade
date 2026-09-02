package db

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return database
}

func TestMigrateIdempotent(t *testing.T) {
	database := openTestDB(t)
	// Second run must not fail: schema is CREATE TABLE IF NOT EXISTS.
	if err := database.Migrate(); err != nil {
		t.Errorf("second Migrate: %v", err)
	}
}

// The DSN must actually reach the driver: WAL mode proves the _pragma
// params are being applied (mattn-style params are silently ignored).
func TestJournalModeWAL(t *testing.T) {
	database := openTestDB(t)
	var mode string
	if err := database.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestMigrateCreatesCoreTables(t *testing.T) {
	database := openTestDB(t)

	core := []string{
		"tenants", "tiers", "users", "usage_records", "api_key_scopes",
		"accounts", "tier_config", "request_logs", "usage_summary",
		"settings", "filter_rules", "model_mappings", "model_combos",
		"custom_models", "proxy_pool", "vcc_cards", "vcc_transactions",
		"image_studio_chats", "image_studio_results", "response_cache",
		"provider_config", "api_keys", "webhooks", "webhook_logs",
		"relay_nodes", "mitm_sessions", "mitm_certs",
	}
	for _, table := range core {
		var name string
		err := database.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("core table %q missing after migrate: %v", table, err)
		}
	}
}

func TestTenantAccountRoundtrip(t *testing.T) {
	database := openTestDB(t)

	_, err := database.Exec(
		`INSERT INTO tenants (id, name, email) VALUES (?, ?, ?)`,
		"tenant-test", "Test Tenant", "test@example.com",
	)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	now := time.Now().Unix()
	res, err := database.Exec(
		`INSERT INTO accounts (tenant_id, provider, email, password, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"tenant-test", "groq", "acct@example.com", "enc-password", "active", now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	var provider, tenantID string
	var status string
	err = database.QueryRow(
		`SELECT provider, tenant_id, status FROM accounts WHERE id = ?`, id,
	).Scan(&provider, &tenantID, &status)
	if err != nil {
		t.Fatalf("select account: %v", err)
	}
	if provider != "groq" || tenantID != "tenant-test" || status != "active" {
		t.Errorf("account roundtrip mismatch: provider=%s tenant=%s status=%s", provider, tenantID, status)
	}
}

// The DSN enables _foreign_keys=on, so an account referencing an unknown
// tenant must be rejected.
func TestAccountForeignKeyEnforced(t *testing.T) {
	database := openTestDB(t)

	now := time.Now().Unix()
	_, err := database.Exec(
		`INSERT INTO accounts (tenant_id, provider, email, password, created_at)
		 VALUES ('no-such-tenant', 'groq', 'x@example.com', 'pw', ?)`, now,
	)
	if err == nil {
		t.Error("insert account with unknown tenant_id: no error, want FK violation")
	}
}

func TestSettingsCompositeKey(t *testing.T) {
	database := openTestDB(t)

	_, err := database.Exec(
		`INSERT INTO tenants (id, name, email) VALUES ('t1', 'T1', 't1@example.com')`)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	// Same key under different tenants is allowed (PK is key+tenant_id).
	for _, tc := range []struct{ key, tenant, value string }{
		{"autowarmup", "_system", "900"},
		{"autowarmup", "t1", "300"},
	} {
		if _, err := database.Exec(
			`INSERT INTO settings (key, tenant_id, value, updated_at) VALUES (?, ?, ?, ?)`,
			tc.key, tc.tenant, tc.value, time.Now().Unix(),
		); err != nil {
			t.Fatalf("insert setting %s/%s: %v", tc.key, tc.tenant, err)
		}
	}

	var sysVal, tenantVal string
	if err := database.QueryRow(
		`SELECT value FROM settings WHERE key='autowarmup' AND tenant_id='_system'`).Scan(&sysVal); err != nil {
		t.Fatalf("select _system setting: %v", err)
	}
	if err := database.QueryRow(
		`SELECT value FROM settings WHERE key='autowarmup' AND tenant_id='t1'`).Scan(&tenantVal); err != nil {
		t.Fatalf("select tenant setting: %v", err)
	}
	if sysVal != "900" || tenantVal != "300" {
		t.Errorf("settings values mismatch: system=%s tenant=%s", sysVal, tenantVal)
	}
}
