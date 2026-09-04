package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"switchblade/internal/config"
	"switchblade/internal/db"
	"switchblade/internal/version"
)

// ─── colors ──────────────────────────────────────────────────────────────────

const (
	green  = "\033[32m"
	red    = "\033[31m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	bold   = "\033[1m"
	reset  = "\033[0m"
)

func successf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, bold+green+"[OK]"+reset+" "+format+"\n", args...)
}
func errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, bold+red+"[ERROR]"+reset+" "+format+"\n", args...)
}
func warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, bold+yellow+"[WARN]"+reset+" "+format+"\n", args...)
}
func infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, bold+cyan+"[INFO]"+reset+" "+format+"\n", args...)
}

// ─── config ──────────────────────────────────────────────────────────────────

func dbPath() string {
	if p := os.Getenv("DATABASE_PATH"); p != "" {
		return p
	}
	return filepath.Join("data", "switchblade.db")
}

// ─── db helpers ──────────────────────────────────────────────────────────────

func openDB() (*sql.DB, func()) {
	path := dbPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		errorf("mkdir: %v", err)
		os.Exit(1)
	}
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		errorf("open db: %v", err)
		os.Exit(1)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		errorf("ping db: %v", err)
		os.Exit(1)
	}
	return db, func() { db.Close() }
}

// ─── schema (v2 multi-tenant) ────────────────────────────────────────────────

const schemaV2 = `
CREATE TABLE IF NOT EXISTS tiers (
    id                TEXT    PRIMARY KEY,
    name              TEXT    NOT NULL UNIQUE,
    description       TEXT    NOT NULL DEFAULT '',
    max_api_keys      INTEGER NOT NULL DEFAULT -1,
    max_models        INTEGER NOT NULL DEFAULT -1,
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 60,
    daily_token_limit INTEGER NOT NULL DEFAULT 100000,
    monthly_price_cents INTEGER NOT NULL DEFAULT 0,
    created_at        INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT    PRIMARY KEY DEFAULT ('tenant_' || lower(hex(randomblob(6)))),
    name        TEXT    NOT NULL,
    email       TEXT    NOT NULL,
    tier_id     TEXT    NOT NULL REFERENCES tiers(id),
    status      TEXT    NOT NULL DEFAULT 'active',
    metadata    TEXT    NOT NULL DEFAULT '{}',
    created_at  INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS tenants_email_idx ON tenants (email);
CREATE INDEX IF NOT EXISTS tenants_status_idx ON tenants (status);

CREATE TABLE IF NOT EXISTS users (
    id          TEXT    PRIMARY KEY DEFAULT ('user_' || lower(hex(randomblob(6)))),
    tenant_id   TEXT    NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    username    TEXT    NOT NULL,
    password    TEXT    NOT NULL,
    password_salt TEXT  NOT NULL DEFAULT '',
    role        TEXT    NOT NULL DEFAULT 'developer',
    status      TEXT    NOT NULL DEFAULT 'active',
    last_login  INTEGER,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS users_tenant_idx ON users (tenant_id);

CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT    PRIMARY KEY DEFAULT ('key_' || lower(hex(randomblob(6)))),
    tenant_id   TEXT    NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    key_hash    TEXT    NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'active',
    last_used_at INTEGER,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS api_keys_tenant_idx ON api_keys (tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_hash_idx ON api_keys (key_hash);

CREATE TABLE IF NOT EXISTS api_key_scopes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    key_id      TEXT    NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    scope       TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS api_key_scopes_key_idx ON api_key_scopes (key_id);
`

const seedData = `
INSERT OR IGNORE INTO tiers (id, name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents)
VALUES ('tier_free', 'free', 'Free plan for evaluation', 5, 10, 60, 100000, 0);
INSERT OR IGNORE INTO tiers (id, name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents)
VALUES ('tier_pro', 'pro', 'Professional plan', 50, 100, 600, 1000000, 2900);
INSERT OR IGNORE INTO tiers (id, name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents)
VALUES ('tier_enterprise', 'enterprise', 'Enterprise plan with unlimited keys and models', -1, -1, 6000, -1, 9900);
`

// ─── password helpers ────────────────────────────────────────────────────────

func hashPassword(password string) (hash, salt string) {
	saltBytes := make([]byte, 16)
	rand.Read(saltBytes)
	salt = hex.EncodeToString(saltBytes)
	h := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(h[:]), salt
}

func generateKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "sk_live_" + hex.EncodeToString(b)
}

// ─── output helpers ──────────────────────────────────────────────────────────

var useJSON bool

func printTable(headers []string, rows [][]string) {
	if useJSON {
		type row map[string]interface{}
		items := make([]row, 0, len(rows))
		for _, r := range rows {
			m := make(row)
			for i, h := range headers {
				if i < len(r) {
					m[h] = r[i]
				}
			}
			items = append(items, m)
		}
		data, _ := json.MarshalIndent(items, "", "  ")
		fmt.Println(string(data))
		return
	}
	// column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	// format string
	fmtStr := ""
	for _, w := range widths {
		fmtStr += "%-" + strconv.Itoa(w+2) + "s"
	}
	fmtStr += "\n"
	fmt.Printf(fmtStr, interfaceSlice(headers)...)
	sep := ""
	for _, w := range widths {
		sep += strings.Repeat("─", w+2)
	}
	fmt.Println(sep)
	for _, r := range rows {
		fmt.Printf(fmtStr, interfaceSlice(r)...)
	}
}

func interfaceSlice(s []string) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// ─── commands ────────────────────────────────────────────────────────────────

func cmdInit() {
	db, close := openDB()
	defer close()
	if _, err := db.Exec(schemaV2); err != nil {
		errorf("create tables: %v", err)
		os.Exit(1)
	}
	if _, err := db.Exec(seedData); err != nil {
		errorf("seed tiers: %v", err)
		os.Exit(1)
	}
	successf("database initialized, tables created, tiers seeded")
}

func cmdStart() {
	runServer()
}

func cmdStatus() {
	db, close := openDB()
	defer close()

	var tenantCount, userCount, keyCount int
	var dbVersion string
	db.QueryRow("SELECT COUNT(*) FROM tenants").Scan(&tenantCount)
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	db.QueryRow("SELECT COUNT(*) FROM api_keys").Scan(&keyCount)
	db.QueryRow("SELECT sqlite_version()").Scan(&dbVersion)

	if useJSON {
		fmt.Println(toJSON(map[string]interface{}{
			"db_version":   dbVersion,
			"db_path":      dbPath(),
			"tenant_count": tenantCount,
			"user_count":   userCount,
			"key_count":    keyCount,
		}))
		return
	}
	fmt.Println(bold + "Switchblade Status" + reset)
	fmt.Printf("  DB Path:    %s\n", dbPath())
	fmt.Printf("  SQLite:     %s\n", dbVersion)
	fmt.Printf("  Tenants:    %d\n", tenantCount)
	fmt.Printf("  Users:      %d\n", userCount)
	fmt.Printf("  API Keys:   %d\n", keyCount)
}

func toJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// ─── tenants ─────────────────────────────────────────────────────────────────

func cmdTenantsList() {
	db, close := openDB()
	defer close()

	rows, err := db.Query(`SELECT t.id, t.name, t.email, ti.name as tier, t.status, t.created_at
		FROM tenants t JOIN tiers ti ON t.tier_id = ti.id ORDER BY t.created_at`)
	if err != nil {
		errorf("query tenants: %v", err)
		os.Exit(1)
	}
	defer rows.Close()

	var data [][]string
	for rows.Next() {
		var id, name, email, tier, status string
		var created int64
		rows.Scan(&id, &name, &email, &tier, &status, &created)
		data = append(data, []string{id, name, email, tier, status, time.Unix(created, 0).Format("2006-01-02 15:04")})
	}
	printTable([]string{"ID", "Name", "Email", "Tier", "Status", "Created"}, data)
}

func cmdTenantsCreate(fs *flagSet) {
	name := fs.String("name", "", "Tenant name")
	email := fs.String("email", "", "Tenant email")
	tier := fs.String("tier", "free", "Tier: free|pro|enterprise")
	fs.MustParse()

	if *name == "" || *email == "" {
		errorf("name and email are required")
		os.Exit(1)
	}

	db, close := openDB()
	defer close()

	var tierID string
	db.QueryRow("SELECT id FROM tiers WHERE name = ?", *tier).Scan(&tierID)
	if tierID == "" {
		errorf("tier %q not found", *tier)
		os.Exit(1)
	}

	ts := time.Now().Unix()
	id := fmt.Sprintf("tenant_%s", randHex(8))
	_, err := db.Exec("INSERT INTO tenants (id, name, email, tier_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, 'active', ?, ?)",
		id, *name, *email, tierID, ts, ts)
	if err != nil {
		errorf("create tenant: %v", err)
		os.Exit(1)
	}
	successf("tenant created: %s (%s)", *name, id)
}

func cmdTenantsSuspend(id string)  { tenantSetStatus(id, "suspended") }
func cmdTenantsActivate(id string) { tenantSetStatus(id, "active") }

func tenantSetStatus(id, status string) {
	if id == "" {
		errorf("tenant ID required")
		os.Exit(1)
	}
	db, close := openDB()
	defer close()

	res, err := db.Exec("UPDATE tenants SET status = ?, updated_at = unixepoch() WHERE id = ?", status, id)
	if err != nil {
		errorf("update tenant: %v", err)
		os.Exit(1)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		errorf("tenant %q not found", id)
		os.Exit(1)
	}
	successf("tenant %s set to %s", id, status)
}

func cmdTenantsDelete(id string) {
	if id == "" {
		errorf("tenant ID required")
		os.Exit(1)
	}
	db, close := openDB()
	defer close()

	tx, _ := db.Begin()
	if _, err := tx.Exec("DELETE FROM api_key_scopes WHERE key_id IN (SELECT id FROM api_keys WHERE tenant_id = ?)", id); err != nil {
		tx.Rollback()
		errorf("delete key scopes: %v", err)
		os.Exit(1)
	}
	if _, err := tx.Exec("DELETE FROM api_keys WHERE tenant_id = ?", id); err != nil {
		tx.Rollback()
		errorf("delete api keys: %v", err)
		os.Exit(1)
	}
	if _, err := tx.Exec("DELETE FROM users WHERE tenant_id = ?", id); err != nil {
		tx.Rollback()
		errorf("delete users: %v", err)
		os.Exit(1)
	}
	res, err := tx.Exec("DELETE FROM tenants WHERE id = ?", id)
	if err != nil {
		tx.Rollback()
		errorf("delete tenant: %v", err)
		os.Exit(1)
	}
	tx.Commit()
	n, _ := res.RowsAffected()
	if n == 0 {
		errorf("tenant %q not found", id)
		os.Exit(1)
	}
	successf("tenant %s deleted with all data", id)
}

// ─── users ───────────────────────────────────────────────────────────────────

func cmdUsersList() {
	db, close := openDB()
	defer close()

	rows, err := db.Query(`SELECT u.id, u.username, u.role, u.status, t.name as tenant, u.created_at
		FROM users u JOIN tenants t ON u.tenant_id = t.id ORDER BY u.created_at`)
	if err != nil {
		errorf("query users: %v", err)
		os.Exit(1)
	}
	defer rows.Close()

	var data [][]string
	for rows.Next() {
		var id, username, role, status, tenant string
		var created int64
		rows.Scan(&id, &username, &role, &status, &tenant, &created)
		data = append(data, []string{id, username, role, status, tenant, time.Unix(created, 0).Format("2006-01-02 15:04")})
	}
	printTable([]string{"ID", "Username", "Role", "Status", "Tenant", "Created"}, data)
}

func cmdUsersCreate(fs *flagSet) {
	tenantID := fs.String("tenant-id", "", "Tenant ID")
	username := fs.String("username", "", "Username")
	password := fs.String("password", "", "Password")
	role := fs.String("role", "developer", "Role: owner|admin|developer|viewer")
	fs.MustParse()

	if *tenantID == "" || *username == "" || *password == "" {
		errorf("tenant-id, username, and password are required")
		os.Exit(1)
	}

	db, close := openDB()
	defer close()

	var exists int
	db.QueryRow("SELECT COUNT(*) FROM tenants WHERE id = ?", *tenantID).Scan(&exists)
	if exists == 0 {
		errorf("tenant %q not found", *tenantID)
		os.Exit(1)
	}

	hash, salt := hashPassword(*password)
	id := fmt.Sprintf("user_%s", randHex(8))
	ts := time.Now().Unix()
	_, err := db.Exec("INSERT INTO users (id, tenant_id, username, password, password_salt, role, status, created_at) VALUES (?, ?, ?, ?, ?, ?, 'active', ?)",
		id, *tenantID, *username, hash, salt, *role, ts)
	if err != nil {
		errorf("create user: %v", err)
		os.Exit(1)
	}
	successf("user created: %s (%s)", *username, id)
}

func cmdUsersDelete(id string) {
	if id == "" {
		errorf("user ID required")
		os.Exit(1)
	}
	db, close := openDB()
	defer close()
	res, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		errorf("delete user: %v", err)
		os.Exit(1)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		errorf("user %q not found", id)
		os.Exit(1)
	}
	successf("user %s deleted", id)
}

// ─── api keys ────────────────────────────────────────────────────────────────

func cmdKeysCreate(fs *flagSet) {
	tenantID := fs.String("tenant-id", "", "Tenant ID")
	name := fs.String("name", "", "Key name")
	scopes := fs.String("scopes", "", "Scopes (comma-separated, e.g. gpt-*,claude-*)")
	fs.MustParse()

	if *tenantID == "" || *name == "" {
		errorf("tenant-id and name are required")
		os.Exit(1)
	}

	db, close := openDB()
	defer close()

	plainKey := generateKey()
	keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(plainKey)))
	id := fmt.Sprintf("key_%s", randHex(8))
	ts := time.Now().Unix()

	_, err := db.Exec("INSERT INTO api_keys (id, tenant_id, name, key_hash, status, created_at) VALUES (?, ?, ?, ?, 'active', ?)",
		id, *tenantID, *name, keyHash, ts)
	if err != nil {
		errorf("create key: %v", err)
		os.Exit(1)
	}

	if *scopes != "" {
		for _, s := range strings.Split(*scopes, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				db.Exec("INSERT INTO api_key_scopes (key_id, scope) VALUES (?, ?)", id, s)
			}
		}
	}

	if useJSON {
		fmt.Println(toJSON(map[string]interface{}{"id": id, "key": plainKey, "name": *name}))
	} else {
		fmt.Println()
		fmt.Println(bold + green + "API Key Created" + reset)
		fmt.Printf("  Name: %s\n", *name)
		fmt.Printf("  ID:   %s\n", id)
		fmt.Printf("  Key:  %s%s%s\n", bold, plainKey, reset)
		fmt.Println()
		warnf("store this key securely — it will not be shown again")
	}
}

func cmdKeysList(fs *flagSet) {
	tenantID := fs.String("tenant-id", "", "Tenant ID (required)")
	fs.MustParse()

	if *tenantID == "" {
		errorf("--tenant-id is required")
		os.Exit(1)
	}

	db, close := openDB()
	defer close()

	rows, err := db.Query(`SELECT k.id, k.name, k.status, k.last_used_at, k.created_at
		FROM api_keys k WHERE k.tenant_id = ? ORDER BY k.created_at`, *tenantID)
	if err != nil {
		errorf("query keys: %v", err)
		os.Exit(1)
	}
	defer rows.Close()

	var data [][]string
	for rows.Next() {
		var id, name, status string
		var lastUsed, created sql.NullInt64
		rows.Scan(&id, &name, &status, &lastUsed, &created)
		lu := ""
		if lastUsed.Valid {
			lu = time.Unix(lastUsed.Int64, 0).Format("2006-01-02 15:04")
		}
		cr := ""
		if created.Valid {
			cr = time.Unix(created.Int64, 0).Format("2006-01-02 15:04")
		}
		data = append(data, []string{id, name, status, lu, cr})
	}
	printTable([]string{"ID", "Name", "Status", "Last Used", "Created"}, data)
}

func cmdKeysDelete(id string) {
	if id == "" {
		errorf("key ID required")
		os.Exit(1)
	}
	db, close := openDB()
	defer close()

	tx, _ := db.Begin()
	tx.Exec("DELETE FROM api_key_scopes WHERE key_id = ?", id)
	res, err := tx.Exec("DELETE FROM api_keys WHERE id = ?", id)
	if err != nil {
		tx.Rollback()
		errorf("delete key: %v", err)
		os.Exit(1)
	}
	tx.Commit()
	n, _ := res.RowsAffected()
	if n == 0 {
		errorf("key %q not found", id)
		os.Exit(1)
	}
	successf("key %s deleted", id)
}

// ─── tiers ───────────────────────────────────────────────────────────────────

func cmdTiersList() {
	db, close := openDB()
	defer close()

	rows, err := db.Query("SELECT id, name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents FROM tiers ORDER BY monthly_price_cents")
	if err != nil {
		errorf("query tiers: %v", err)
		os.Exit(1)
	}
	defer rows.Close()

	var data [][]string
	for rows.Next() {
		var id, name, desc string
		var maxKeys, maxModels, rate, tokens, price int
		rows.Scan(&id, &name, &desc, &maxKeys, &maxModels, &rate, &tokens, &price)
		mk := strconv.Itoa(maxKeys)
		if maxKeys < 0 {
			mk = "unlimited"
		}
		data = append(data, []string{id, name, desc, mk, strconv.Itoa(rate), strconv.Itoa(tokens), fmt.Sprintf("$%d/mo", price/100)})
	}
	printTable([]string{"ID", "Name", "Description", "Max Keys", "Rate/min", "Daily Tokens", "Price"}, data)
}

// ─── backup ──────────────────────────────────────────────────────────────────

func cmdBackup(fs *flagSet) {
	output := fs.String("output", "", "Output directory")
	fs.MustParse()

	src := dbPath()
	if _, err := os.Stat(src); os.IsNotExist(err) {
		errorf("database file not found: %s", src)
		os.Exit(1)
	}

	dir := *output
	if dir == "" {
		dir = "backups"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		errorf("mkdir: %v", err)
		os.Exit(1)
	}

	dest := filepath.Join(dir, fmt.Sprintf("switchblade_%s.db", time.Now().Format("20060102_150405")))

	// Use SQLite backup via VACUUM INTO
	db, close := openDB()
	defer close()
	_, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", filepath.ToSlash(dest)))
	if err != nil {
		// Fallback: copy file
		data, err := os.ReadFile(src)
		if err != nil {
			errorf("backup: %v", err)
			os.Exit(1)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			errorf("backup: %v", err)
			os.Exit(1)
		}
	}
	successf("backup saved to %s", dest)
}

// ─── stats ───────────────────────────────────────────────────────────────────

func cmdStats() {
	db, close := openDB()
	defer close()

	var tenants, users, keys, reqs, totalTokens int
	db.QueryRow("SELECT COUNT(*) FROM tenants").Scan(&tenants)
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&users)
	db.QueryRow("SELECT COUNT(*) FROM api_keys").Scan(&keys)
	db.QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&reqs)
	db.QueryRow("SELECT COALESCE(SUM(total_tokens), 0) FROM request_logs").Scan(&totalTokens)

	if useJSON {
		fmt.Println(toJSON(map[string]interface{}{
			"tenants":      tenants,
			"users":        users,
			"api_keys":     keys,
			"requests":     reqs,
			"total_tokens": totalTokens,
		}))
		return
	}
	fmt.Println(bold + "Platform Statistics" + reset)
	fmt.Printf("  Tenants:     %d\n", tenants)
	fmt.Printf("  Users:       %d\n", users)
	fmt.Printf("  API Keys:    %d\n", keys)
	fmt.Printf("  Requests:    %d\n", reqs)
	fmt.Printf("  Total Tokens: %d\n", totalTokens)
}

// ─── seed ─────────────────────────────────────────────────────────────────────

func cmdSeed() {
	db, close := openDB()
	defer close()

	// Ensure schema + tiers
	if _, err := db.Exec(schemaV2); err != nil {
		errorf("schema: %v", err)
		os.Exit(1)
	}
	if _, err := db.Exec(seedData); err != nil {
		errorf("seed tiers: %v", err)
		os.Exit(1)
	}

	// Demo tenant
	tenantID := "tenant_demo"
	var exists int
	db.QueryRow("SELECT COUNT(*) FROM tenants WHERE id = ?", tenantID).Scan(&exists)
	if exists == 0 {
		ts := time.Now().Unix()
		var tierID string
		db.QueryRow("SELECT id FROM tiers WHERE name = 'free'").Scan(&tierID)
		if tierID == "" {
			tierID = "tier_free"
		}
		_, err := db.Exec("INSERT INTO tenants (id, name, email, tier_id, status, created_at, updated_at) VALUES (?, 'Demo Org', 'demo@switchblade.local', ?, 'active', ?, ?)",
			tenantID, tierID, ts, ts)
		if err != nil {
			errorf("seed tenant: %v", err)
			os.Exit(1)
		}
		infof("created demo tenant: %s", tenantID)
	}

	// Demo user
	userID := "user_demo"
	db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&exists)
	if exists == 0 {
		hash, salt := hashPassword("demo123")
		ts := time.Now().Unix()
		_, err := db.Exec("INSERT INTO users (id, tenant_id, username, password, password_salt, role, status, created_at) VALUES (?, ?, 'demo', ?, ?, 'owner', 'active', ?)",
			userID, tenantID, hash, salt, ts)
		if err != nil {
			errorf("seed user: %v", err)
			os.Exit(1)
		}
		infof("created demo user: demo / demo123")
	}

	// Demo API key
	keyID := "key_demo"
	db.QueryRow("SELECT COUNT(*) FROM api_keys WHERE id = ?", keyID).Scan(&exists)
	if exists == 0 {
		plainKey := generateKey()
		keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(plainKey)))
		ts := time.Now().Unix()
		_, err := db.Exec("INSERT INTO api_keys (id, tenant_id, name, key_hash, status, created_at) VALUES (?, ?, 'demo-key', ?, 'active', ?)",
			keyID, tenantID, keyHash, ts)
		if err != nil {
			errorf("seed key: %v", err)
			os.Exit(1)
		}
		db.Exec("INSERT OR IGNORE INTO api_key_scopes (key_id, scope) VALUES (?, '*')", keyID)
		infof("created demo key: %s", plainKey)
	}

	successf("seed complete — tenant: %s, user: demo/demo123", tenantID)
}

// ─── dev ─────────────────────────────────────────────────────────────────────

func cmdDev() {
	fmt.Println(bold + cyan + "Switchblade Dev Mode" + reset)
	fmt.Println()

	// Step 1: Build Go binary
	infof("building Go binary...")
	binName := "switchblade"
	if _, err := os.Stat("switchblade.exe"); err == nil {
		binName = "switchblade.exe"
	}
	buildCmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", binName, "./cmd/switchblade")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		errorf("build failed: %v", err)
		os.Exit(1)
	}
	successf("Go binary built")

	// Step 2: Seed database
	infof("initializing database...")
	db, close := openDB()
	defer close()
	if _, err := db.Exec(schemaV2); err != nil {
		errorf("init tables: %v", err)
		os.Exit(1)
	}
	if _, err := db.Exec(seedData); err != nil {
		errorf("seed tiers: %v", err)
		os.Exit(1)
	}
	successf("database ready")
	close()

	// Step 3: Start Vite dev server in background
	var viteCmd *exec.Cmd

	infof("starting Vite dev server...")
	if _, err := os.Stat("web/package.json"); err == nil {
		viteCmd = exec.Command("npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "5173")
		viteCmd.Dir = "web"
		viteCmd.Stdout = os.Stdout
		viteCmd.Stderr = os.Stderr
		if err := viteCmd.Start(); err != nil {
			warnf("failed to start Vite: %v (dashboard will use static files)", err)
			viteCmd = nil
		} else {
			successf("Vite running on http://localhost:5173")
		}
	} else {
		warnf("web/ not found, skipping Vite")
	}

	// Step 4: Start Go server
	infof("starting Switchblade server...")
	// Resolve binary path properly for all platforms
	binPath := binName
	if _, err := os.Stat(binName); err != nil {
		// Try with .\ prefix on Windows
		binPath = "." + string(filepath.Separator) + binName
		if _, err := os.Stat(binPath); err != nil {
			// Try full path
			wd, _ := os.Getwd()
			binPath = filepath.Join(wd, binName)
		}
	}
	serverCmd := exec.Command("cmd", "/C", binPath)
	serverCmd.Stdout = os.Stdout
	serverCmd.Stderr = os.Stderr
	if err := serverCmd.Start(); err != nil {
		errorf("server failed: %v", err)
		os.Exit(1)
	}
	successf("Switchblade running on :2005 (proxy) :2006 (dashboard)")

	fmt.Println()
	fmt.Println(bold + "Dev URLs:" + reset)
	fmt.Printf("  Proxy:     http://localhost:2005\n")
	fmt.Printf("  Dashboard: http://localhost:2006\n")
	if viteCmd != nil {
		fmt.Printf("  Vite HMR:  http://localhost:5173\n")
	}
	fmt.Println()
	infof("Ctrl+C to stop all services")

	// Wait for interrupt
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	fmt.Println()
	warnf("shutting down...")

	if viteCmd != nil && viteCmd.Process != nil {
		viteCmd.Process.Kill()
		viteCmd.Wait()
	}
	if serverCmd.Process != nil {
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}
	successf("all services stopped")
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ─── flagSet (minimal stdlib wrapper) ────────────────────────────────────────

type flagSet struct {
	fs   *FlagSet
	args []string
	cmd  string
}

type FlagSet struct {
	flags map[string]*string
	pos   []string
}

func newFlagSet(args []string) *flagSet {
	return &flagSet{
		fs:   &FlagSet{flags: make(map[string]*string)},
		args: args,
	}
}

func (f *flagSet) String(name, value, usage string) *string {
	ptr := new(string)
	*ptr = value
	f.fs.flags["--"+name] = ptr
	return ptr
}

func (f *flagSet) MustParse() {
	i := 0
	for i < len(f.args) {
		arg := f.args[i]
		if ptr, ok := f.fs.flags[arg]; ok {
			i++
			if i < len(f.args) {
				*ptr = f.args[i]
			} else {
				errorf("%s requires a value", arg)
				os.Exit(1)
			}
		} else if arg == "--json" {
			useJSON = true
		} else if arg == "--help" || arg == "-h" {
			printHelp(f.cmd)
			os.Exit(0)
		} else if !strings.HasPrefix(arg, "--") {
			f.fs.pos = append(f.fs.pos, arg)
		} else {
			warnf("unknown flag: %s", arg)
		}
		i++
	}
}

func (f *flagSet) Args() []string { return f.fs.pos }

// ─── help ────────────────────────────────────────────────────────────────────

func printHelp(cmd string) {
	helps := map[string]string{
		"": `switchblade — Multi-tenant platform management and server CLI

USAGE:
  switchblade <command> [flags]

COMMANDS:
  start                       Start the Switchblade server (default)
  init                        Initialize database, create tables, seed tiers
  status                      Show server status, DB version, tenant count

  tenants list                List all tenants
  tenants create              Create a new tenant
    --name NAME               Tenant name
    --email EMAIL             Tenant email
    --tier TIER               Tier: free|pro|enterprise (default: free)

  tenants suspend <id>        Suspend a tenant
  tenants activate <id>       Activate a suspended tenant
  tenants delete <id>         Delete tenant and all their data

  users list                  List all users
  users create                Create a new user
    --tenant-id TID           Tenant ID
    --username USERNAME       Username
    --password PASSWORD       Password
    --role ROLE               Role: owner|admin|developer|viewer (default: developer)

  users delete <id>           Delete a user

  keys create                 Create an API key
    --tenant-id TID           Tenant ID
    --name NAME               Key name
    --scopes "gpt-*,claude-*" Scopes (comma-separated)

  keys list                   List keys for a tenant
    --tenant-id TID           Tenant ID (required)

  keys delete <id>            Delete an API key

  tiers list                  List all tiers

  backup                      Run database backup
    --output DIR              Output directory (default: backups)

  stats                       Show platform stats (tenants, users, requests, tokens)

  restore                     Decompress and restore a database backup
    --file PATH               Path to backup file (.db or .db.gz) (required)

GLOBAL FLAGS:
  --json                      Output in JSON format
  --help, -h                  Show this help

EXAMPLES:
  switchblade
  switchblade start
  switchblade tenants create --name "Acme Corp" --email admin@acme.io --tier pro
  switchblade keys create --tenant-id tenant_xxx --name "prod-key" --scopes "gpt-*,claude-*"
  switchblade tenants list --json
  switchblade backup --output /tmp/backups
`,
		"start":   "Start the Switchblade server.\n\nUsage: switchblade start",
		"init":    "Initialize database, create tables, seed default tiers.\n\nUsage: switchblade init",
		"status":  "Show server status including DB version and counts.\n\nUsage: switchblade status [--json]",
		"tenants": "Manage tenants.\n\nUsage: switchblade tenants <list|create|suspend|activate|delete> [flags]",
		"users":   "Manage users.\n\nUsage: switchblade users <list|create|delete> [flags]",
		"keys":    "Manage API keys.\n\nUsage: switchblade keys <create|list|delete> [flags]",
		"tiers":   "Manage tiers.\n\nUsage: switchblade tiers <list>",
		"backup":  "Run database backup.\n\nUsage: switchblade backup [--output DIR]",
		"stats":   "Show platform statistics.\n\nUsage: switchblade stats [--json]",
		"seed":    "Seed demo data (tenant + user + API key).\n\nUsage: switchblade seed",
		"dev":     "Start dev mode (build + seed + Vite + server).\n\nUsage: switchblade dev",
		"version": "Show version info.\n\nUsage: switchblade version",
		"restore": "Decompress and restore database backup.\n\nUsage: switchblade restore --file=<path>",
		"help":    "Show this help.\n\nUsage: switchblade help [command]",
	}
	if h, ok := helps[cmd]; ok {
		fmt.Println(h)
	} else {
		fmt.Println(helps[""])
	}
}

// ─── CLI Entrypoint ──────────────────────────────────────────────────────────

func runCLI() {
	if len(os.Args) < 2 {
		printHelp("")
		os.Exit(0)
	}

	// Handle global flags anywhere
	for _, a := range os.Args[1:] {
		if a == "--json" {
			useJSON = true
			break
		}
	}

	cmd := os.Args[1]

	// Check for --help as first arg
	if cmd == "--help" || cmd == "-h" {
		printHelp("")
		os.Exit(0)
	}

	switch cmd {
	case "init":
		cmdInit()
	case "start":
		cmdStart()
	case "status":
		cmdStatus()

	case "tenants":
		if len(os.Args) < 3 {
			printHelp("tenants")
			os.Exit(1)
		}
		sub := os.Args[2]
		rest := os.Args[3:]
		fs := newFlagSet(rest)
		fs.cmd = "tenants " + sub
		switch sub {
		case "list":
			cmdTenantsList()
		case "create":
			cmdTenantsCreate(fs)
		case "suspend":
			fs.MustParse()
			if len(fs.Args()) < 1 {
				errorf("tenant ID required")
				os.Exit(1)
			}
			cmdTenantsSuspend(fs.Args()[0])
		case "activate":
			fs.MustParse()
			if len(fs.Args()) < 1 {
				errorf("tenant ID required")
				os.Exit(1)
			}
			cmdTenantsActivate(fs.Args()[0])
		case "delete":
			fs.MustParse()
			if len(fs.Args()) < 1 {
				errorf("tenant ID required")
				os.Exit(1)
			}
			cmdTenantsDelete(fs.Args()[0])
		default:
			errorf("unknown subcommand: tenants %s", sub)
			os.Exit(1)
		}

	case "users":
		if len(os.Args) < 3 {
			printHelp("users")
			os.Exit(1)
		}
		sub := os.Args[2]
		rest := os.Args[3:]
		fs := newFlagSet(rest)
		fs.cmd = "users " + sub
		switch sub {
		case "list":
			cmdUsersList()
		case "create":
			cmdUsersCreate(fs)
		case "delete":
			fs.MustParse()
			if len(fs.Args()) < 1 {
				errorf("user ID required")
				os.Exit(1)
			}
			cmdUsersDelete(fs.Args()[0])
		default:
			errorf("unknown subcommand: users %s", sub)
			os.Exit(1)
		}

	case "keys":
		if len(os.Args) < 3 {
			printHelp("keys")
			os.Exit(1)
		}
		sub := os.Args[2]
		rest := os.Args[3:]
		fs := newFlagSet(rest)
		fs.cmd = "keys " + sub
		switch sub {
		case "create":
			cmdKeysCreate(fs)
		case "list":
			cmdKeysList(fs)
		case "delete":
			fs.MustParse()
			if len(fs.Args()) < 1 {
				errorf("key ID required")
				os.Exit(1)
			}
			cmdKeysDelete(fs.Args()[0])
		default:
			errorf("unknown subcommand: keys %s", sub)
			os.Exit(1)
		}

	case "tiers":
		if len(os.Args) >= 3 && os.Args[2] == "list" {
			cmdTiersList()
		} else if len(os.Args) >= 3 && os.Args[2] == "--help" {
			printHelp("tiers")
		} else {
			cmdTiersList()
		}

	case "backup":
		fs := newFlagSet(os.Args[2:])
		fs.cmd = "backup"
		cmdBackup(fs)

	case "stats":
		cmdStats()

	case "seed":
		cmdSeed()

	case "dev":
		cmdDev()

	case "version":
		fmt.Println(bold + cyan + "switchblade" + reset + " " + version.Version)
		fmt.Printf("  commit: %s\n", version.Commit)
		fmt.Printf("  date:   %s\n", version.Date)
		fmt.Printf("  go:     %s\n", version.GoVer)

	case "restore":
		// CLI restore command: switchblade restore --file=<path>
		cfg := config.Load()
		var backupFile string
		for _, arg := range os.Args[2:] {
			if len(arg) > 7 && arg[:7] == "--file=" {
				backupFile = arg[7:]
			}
		}
		if backupFile == "" {
			fmt.Fprintln(os.Stderr, "usage: switchblade restore --file=<backup.db.gz>")
			os.Exit(1)
		}
		if err := db.Restore(backupFile, cfg.DatabasePath); err != nil {
			fmt.Fprintf(os.Stderr, "restore failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("restored %s → %s\n", backupFile, cfg.DatabasePath)

	case "help":
		sub := ""
		if len(os.Args) >= 3 {
			sub = os.Args[2]
		}
		printHelp(sub)

	default:
		errorf("unknown command: %s", cmd)
		printHelp("")
		os.Exit(1)
	}
}
