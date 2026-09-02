package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"switchblade/internal/db"
	"switchblade/internal/reqctx"
)

const legacyTestKey = "legacy-global-key"

// authTestDB returns a fully migrated database, so the middleware runs against
// the same schema production does rather than a hand-rolled subset.
func authTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func insertTenant(t *testing.T, database *db.DB, id, status string) {
	t.Helper()
	if _, err := database.Exec(
		"INSERT INTO tenants (id, name, email, status) VALUES (?, ?, ?, ?)",
		id, id, id+"@test.com", status,
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
}

// insertKey stores a v2 key for tenant and returns its raw value.
func insertKey(t *testing.T, database *db.DB, tenantID string, enabled int, scopes ...string) string {
	t.Helper()
	value := "sk_live_" + tenantID
	sum := sha256.Sum256([]byte(value))
	res, err := database.Exec(
		"INSERT INTO api_keys (tenant_id, name, key_hash, created_at, enabled) VALUES (?, 'test', ?, 0, ?)",
		tenantID, hex.EncodeToString(sum[:]), enabled,
	)
	if err != nil {
		t.Fatalf("insert key: %v", err)
	}
	keyID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	for _, s := range scopes {
		if _, err := database.Exec(
			"INSERT INTO api_key_scopes (api_key_id, model_pattern) VALUES (?, ?)", keyID, s,
		); err != nil {
			t.Fatalf("insert scope: %v", err)
		}
	}
	return value
}

// serveAuth runs one request through AuthKeyV2OrLegacy and reports whether the
// protected handler was reached.
func serveAuth(database *sql.DB, req *http.Request) (*httptest.ResponseRecorder, bool, string) {
	var reached bool
	var tenant string
	h := AuthKeyV2OrLegacy(database, legacyTestKey)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		reached = true
		tenant = reqctx.Tenant(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, reached, tenant
}

func chatRequest(key, contentType, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("x-api-key", key)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

func TestAuthKeyV2_ValidKeyInjectsTenant(t *testing.T) {
	database := authTestDB(t)
	insertTenant(t, database, "tenant_a", "active")
	key := insertKey(t, database, "tenant_a", 1)

	rec, reached, tenant := serveAuth(database.DB, chatRequest(key, "application/json", `{"model":"gpt-4o"}`))
	if !reached {
		t.Fatalf("valid key rejected: %d %s", rec.Code, rec.Body.String())
	}
	if tenant != "tenant_a" {
		t.Fatalf("expected tenant_a in context, got %q", tenant)
	}
}

// A key nobody has heard of is not an infrastructure failure — the legacy global
// key must still get its turn.
func TestAuthKeyV2_UnknownKeyFallsThroughToLegacy(t *testing.T) {
	database := authTestDB(t)

	rec, reached, _ := serveAuth(database.DB, chatRequest(legacyTestKey, "application/json", `{}`))
	if !reached {
		t.Fatalf("legacy key rejected: %d %s", rec.Code, rec.Body.String())
	}

	rec, reached, _ = serveAuth(database.DB, chatRequest("sk_live_nope", "application/json", `{}`))
	if reached {
		t.Fatal("unknown key reached the handler")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// A lookup that cannot run tells us nothing about the key, so the request must
// not be handed to the legacy comparison — that would admit it with no tenant,
// no scopes and no quota.
func TestAuthKeyV2_DatabaseErrorFailsClosed(t *testing.T) {
	database := authTestDB(t)
	insertTenant(t, database, "tenant_a", "active")
	key := insertKey(t, database, "tenant_a", 1)
	database.Close()

	for name, presented := range map[string]string{
		"v2 key":     key,
		"legacy key": legacyTestKey,
	} {
		rec, reached, _ := serveAuth(database.DB, chatRequest(presented, "application/json", `{}`))
		if reached {
			t.Fatalf("%s reached the handler during a database outage", name)
		}
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503, got %d (%s)", name, rec.Code, rec.Body.String())
		}
	}
}

// A key pointing at a nonexistent tenant cannot exist: the FK on
// api_keys.tenant_id rejects the insert, so the middleware's missing-tenant
// branch is unreachable through real data. That branch stays as defense in
// depth and is exercised by the suspended-tenant case below — missing and
// suspended tenants both land in the same "tenant is not active" 403.
func TestAuthKeyV2_UnknownTenantKeyCannotExist(t *testing.T) {
	database := authTestDB(t)

	sum := sha256.Sum256([]byte("sk_live_tenant_ghost"))
	_, err := database.Exec(
		"INSERT INTO api_keys (tenant_id, name, key_hash, created_at, enabled) VALUES ('tenant_ghost', 'test', ?, 0, 1)",
		hex.EncodeToString(sum[:]),
	)
	if err == nil {
		t.Fatal("api key for unknown tenant inserted; FK enforcement is broken")
	}
}

func TestAuthKeyV2_SuspendedTenantIsForbidden(t *testing.T) {
	database := authTestDB(t)
	insertTenant(t, database, "tenant_a", "suspended")
	key := insertKey(t, database, "tenant_a", 1)

	rec, reached, _ := serveAuth(database.DB, chatRequest(key, "application/json", `{}`))
	if reached {
		t.Fatal("suspended tenant reached the handler")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestAuthKeyV2_DisabledKeyIsForbidden(t *testing.T) {
	database := authTestDB(t)
	insertTenant(t, database, "tenant_a", "active")
	key := insertKey(t, database, "tenant_a", 0)

	rec, reached, _ := serveAuth(database.DB, chatRequest(key, "application/json", `{}`))
	if reached {
		t.Fatal("disabled key reached the handler")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

// The scope check keys off the media type, so the charset parameter every SDK
// attaches must not become a way around it.
func TestAuthKeyV2_ScopeCheckSurvivesContentTypeParameters(t *testing.T) {
	database := authTestDB(t)
	insertTenant(t, database, "tenant_a", "active")
	key := insertKey(t, database, "tenant_a", 1, "gpt-*")

	for _, ct := range []string{"application/json", "application/json; charset=utf-8", "Application/JSON"} {
		rec, reached, _ := serveAuth(database.DB, chatRequest(key, ct, `{"model":"claude-3-opus"}`))
		if reached {
			t.Fatalf("out-of-scope model admitted with Content-Type %q", ct)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("Content-Type %q: expected 403, got %d", ct, rec.Code)
		}
	}

	if _, reached, _ := serveAuth(database.DB, chatRequest(key, "application/json; charset=utf-8", `{"model":"gpt-4o"}`)); !reached {
		t.Fatal("in-scope model rejected")
	}
}
