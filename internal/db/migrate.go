// Package db provides SQLite database access for Switchblade.
package db

import (
	"database/sql"
	"fmt"
	"log"
)

// Migration represents a single schema migration. SQL runs first, then Fn.
// Fn exists for steps SQLite cannot express idempotently — notably ADD COLUMN,
// which has no IF NOT EXISTS form and would break re-running a migration that
// failed halfway through.
type Migration struct {
	Version int
	Name    string
	SQL     string
	Fn      func(*sql.DB) error
}

// addColumn adds a column only when it is missing.
func addColumn(database *sql.DB, table, column, decl string) error {
	var n int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column,
	).Scan(&n); err != nil {
		return fmt.Errorf("inspect %s.%s: %w", table, column, err)
	}
	if n > 0 {
		return nil
	}
	if _, err := database.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl)); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return nil
}

// migrations is the ordered list of all versioned migrations.
// v1 is the base schema applied by Migrate(); migrations start at v2.
var migrations = []Migration{
	{
		Version: 2,
		Name:    "add_accounts_metadata_column",
		// ponytail: schema_v2.sql already has metadata; guard for pre-v2 DBs.
		SQL: `INSERT INTO schema_migrations (version) SELECT 2 WHERE NOT EXISTS (SELECT 1 FROM pragma_table_info('accounts') WHERE name='metadata')`,
	},
	{
		Version: 3,
		Name:    "add_tier_column_and_tier_config",
		SQL: `
CREATE TABLE IF NOT EXISTS tier_config (
    tier        TEXT PRIMARY KEY,
    priority    INTEGER NOT NULL,
    max_cost    REAL DEFAULT 0,
    enabled     INTEGER NOT NULL DEFAULT 1
);
INSERT OR IGNORE INTO tier_config (tier, priority, max_cost, enabled) VALUES
    ('subscription', 1, 0, 1),
    ('cheap', 2, 0, 1),
    ('free', 3, 0, 1);
`,
	},
	{
		Version: 4,
		Name:    "billing_ledger_and_pricing",
		SQL:     billingSchemaSQL,
		Fn: func(database *sql.DB) error {
			// usage_records predates metering: it only had cost_cents, which
			// truncates every sub-cent request to zero. These columns carry the
			// nanodollar cost and the request attributes the dashboard needs.
			cols := []struct{ name, decl string }{
				{"cost_nano", "INTEGER NOT NULL DEFAULT 0"},
				{"provider", "TEXT NOT NULL DEFAULT ''"},
				{"service_kind", "TEXT NOT NULL DEFAULT 'chat'"},
				{"status", "TEXT NOT NULL DEFAULT 'success'"},
				{"latency_ms", "INTEGER NOT NULL DEFAULT 0"},
				{"request_id", "TEXT"},
			}
			for _, c := range cols {
				if err := addColumn(database, "usage_records", c.name, c.decl); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		Version: 5,
		Name:    "usage_records_estimated_flag",
		Fn: func(database *sql.DB) error {
			// A stream that ends without a usage frame is billed from a bytes/4
			// approximation; the flag keeps those rows distinguishable from
			// provider-reported metering.
			return addColumn(database, "usage_records", "estimated", "INTEGER NOT NULL DEFAULT 0")
		},
	},
	// Add future migrations here.
}

// billingSchemaSQL introduces the tables that make usage billable: a rate card,
// a prepaid balance, an append-only ledger, and the subscription period that
// supplies each tenant's included allowance.
const billingSchemaSQL = `
CREATE TABLE IF NOT EXISTS model_pricing (
    model                TEXT    PRIMARY KEY,
    input_nano_per_mtok  INTEGER NOT NULL DEFAULT 0,  -- nanodollars per 1M prompt tokens
    output_nano_per_mtok INTEGER NOT NULL DEFAULT 0,  -- nanodollars per 1M completion tokens
    margin_bps           INTEGER NOT NULL DEFAULT 0,  -- markup in basis points (2000 = +20%)
    enabled              INTEGER NOT NULL DEFAULT 1,
    updated_at           INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tenant_balance (
    tenant_id    TEXT    PRIMARY KEY REFERENCES tenants(id),
    balance_nano INTEGER NOT NULL DEFAULT 0,
    updated_at   INTEGER NOT NULL DEFAULT 0
);

-- Append-only. Every credit and debit lands here so an invoice can be rebuilt
-- from history rather than trusted from a mutable running total.
CREATE TABLE IF NOT EXISTS credit_ledger (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id          TEXT    NOT NULL REFERENCES tenants(id),
    kind               TEXT    NOT NULL,              -- topup|usage|subscription|refund|adjustment
    amount_nano        INTEGER NOT NULL,              -- positive credit, negative debit
    balance_after_nano INTEGER NOT NULL,
    request_id         TEXT,
    model              TEXT,
    note               TEXT,
    created_at         INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS credit_ledger_tenant_created_idx ON credit_ledger (tenant_id, created_at);
CREATE INDEX IF NOT EXISTS credit_ledger_kind_idx           ON credit_ledger (kind, created_at);

CREATE TABLE IF NOT EXISTS tenant_subscriptions (
    tenant_id       TEXT    PRIMARY KEY REFERENCES tenants(id),
    tier_id         TEXT    NOT NULL REFERENCES tiers(id),
    status          TEXT    NOT NULL DEFAULT 'active',  -- active|past_due|canceled
    period_start    INTEGER NOT NULL,
    period_end      INTEGER NOT NULL,
    included_nano   INTEGER NOT NULL DEFAULT 0,         -- allowance value for the period
    used_nano       INTEGER NOT NULL DEFAULT 0,
    overage_enabled INTEGER NOT NULL DEFAULT 1,         -- may spend credit past the allowance
    created_at      INTEGER NOT NULL DEFAULT 0,
    updated_at      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS tenant_subscriptions_period_idx ON tenant_subscriptions (period_end);

CREATE INDEX IF NOT EXISTS usage_records_key_created_idx ON usage_records (api_key_id, created_at);

-- Starter rate card. Prices are upstream list prices in nanodollars per million
-- tokens; margin_bps is the resale markup and is what the business earns.
-- Verify these against each provider's current pricing before selling.
INSERT OR IGNORE INTO model_pricing (model, input_nano_per_mtok, output_nano_per_mtok, margin_bps) VALUES
    ('gpt-4o',            2500000000, 10000000000, 2000),
    ('gpt-4o-mini',        150000000,   600000000, 2000),
    ('gpt-4.1',           2000000000,  8000000000, 2000),
    ('gpt-4.1-mini',       400000000,  1600000000, 2000),
    ('claude-opus',      15000000000, 75000000000, 2000),
    ('claude-sonnet',     3000000000, 15000000000, 2000),
    ('claude-haiku',       800000000,  4000000000, 2000),
    ('gemini-2.5-pro',    1250000000, 10000000000, 2000),
    ('gemini-2.5-flash',   300000000,  2500000000, 2000),
    ('deepseek-chat',      270000000,  1100000000, 2000),
    ('deepseek-reasoner',  550000000,  2190000000, 2000),
    ('llama-3.3-70b',      590000000,   790000000, 2000);
`

// schemaMigrationsSQL creates the migration tracking table.
const schemaMigrationsSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version    INTEGER PRIMARY KEY,
	applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// CreateSchemaMigrations creates the schema_migrations table if it does not exist.
func CreateSchemaMigrations(db *sql.DB) error {
	_, err := db.Exec(schemaMigrationsSQL)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

// GetCurrentVersion returns the highest applied migration version, or 0 if none.
func GetCurrentVersion(db *sql.DB) (int, error) {
	var version sql.NullInt64
	err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("get current version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

// ApplyVersionedMigrations applies all pending migrations in order.
// Already-applied migrations are skipped based on the schema_migrations table.
func ApplyVersionedMigrations(database *sql.DB) error {
	if err := CreateSchemaMigrations(database); err != nil {
		return err
	}

	current, err := GetCurrentVersion(database)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}

		log.Printf("[DB] applying migration v%d: %s", m.Version, m.Name)
		if m.SQL != "" {
			if _, err := database.Exec(m.SQL); err != nil {
				return fmt.Errorf("migration v%d (%s): %w", m.Version, m.Name, err)
			}
		}
		if m.Fn != nil {
			if err := m.Fn(database); err != nil {
				return fmt.Errorf("migration v%d (%s): %w", m.Version, m.Name, err)
			}
		}

		if _, err := database.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Version); err != nil {
			return fmt.Errorf("record migration v%d: %w", m.Version, err)
		}

		log.Printf("[DB] migration v%d applied successfully", m.Version)
	}

	if current == 0 && len(migrations) == 0 {
		log.Println("[DB] no migrations to apply")
	} else {
		latest, _ := GetCurrentVersion(database)
		log.Printf("[DB] schema up-to-date at version %d", latest)
	}

	return nil
}

// Rollback rolls back migrations down to targetVersion (exclusive).
// Currently unimplemented — migrations are forward-only until needed.
func Rollback(_ *sql.DB, _ int) error {
	return fmt.Errorf("rollback not implemented")
}
