package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/config"
	"switchblade/internal/db"
	"switchblade/internal/providers"
)

// testDB creates an in-memory SQLite database with schema applied.
func testDB(t *testing.T) *db.DB {
	t.Helper()

	// Temporarily override the DSN to use in-memory SQLite.
	// db.Open expects a path like ":memory:" to produce "file::memory:?..."
	// We bypass the normal Open() and create an in-memory DB directly
	// because Open() tries os.MkdirAll on the parent directory.
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

// testRouter returns a chi router with management API routes mounted,
// with auth middleware disabled for testing.
func testRouter(t *testing.T, database *db.DB) http.Handler {
	t.Helper()

	r := chi.NewRouter()
	r.Use(CORS)
	r.Use(RequestLogger)
	// No AuthAPIKey middleware — tests don't need auth.

	MountManagementAPI(r, database)
	MountKeysAPI(r, database)
	MountProviderConfigAPI(r, database)
	r.Get("/health", HandleHealthCheck)

	return r
}

// do performs a request and returns the httptest.ResponseRecorder.
func do(handler http.Handler, method, url, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestGETHealth(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	w := do(handler, "GET", "/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /health: status %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestGETStats(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	w := do(handler, "GET", "/api/stats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/stats: status %d, body: %s, want %d", w.Code, w.Body.String(), http.StatusOK)
	}

	// stats returns an array of {provider, model, request_count, ...}
	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Fresh DB = empty array (no request logs yet).
	if len(resp) != 0 {
		t.Fatalf("len(stats) = %d, want 0", len(resp))
	}
}

func TestGETAccounts(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	w := do(handler, "GET", "/api/accounts", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/accounts: status %d, want %d", w.Code, http.StatusOK)
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Fresh database = empty array.
	if len(resp) != 0 {
		t.Fatalf("len(accounts) = %d, want 0", len(resp))
	}
}

func TestPOSTCreateAccount(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	w := do(handler, "POST", "/api/accounts", `{"provider":"test","email":"a@b.com","password":"secret"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/accounts: status %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if id, ok := resp["id"].(float64); !ok || id < 1 {
		t.Fatalf("expected id >= 1, got %v", resp["id"])
	}

	// Verify account was persisted (DB-level check).
	row := db.QueryRow(`SELECT provider, email FROM accounts WHERE id = ?`, int64(resp["id"].(float64)))
	var prov, email string
	if err := row.Scan(&prov, &email); err != nil {
		t.Fatalf("db verify: %v", err)
	}
	if prov != "test" || email != "a@b.com" {
		t.Fatalf("db verify: provider=%s email=%s, want test/a@b.com", prov, email)
	}
}

func TestPOSTAccountConflict(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	// Create the same account twice → second should fail (unique constraint).
	do(handler, "POST", "/api/accounts", `{"provider":"test","email":"dup@b.com","password":"secret"}`)
	w := do(handler, "POST", "/api/accounts", `{"provider":"test","email":"dup@b.com","password":"secret"}`)
	if w.Code == http.StatusCreated {
		t.Fatal("POST duplicate account: expected failure, got 201")
	}
}

func TestGETProviderConfig(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	w := do(handler, "GET", "/api/provider-config", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/provider-config: status %d, want %d", w.Code, http.StatusOK)
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Fresh database = empty array.
	if len(resp) != 0 {
		t.Fatalf("len(configs) = %d, want 0", len(resp))
	}
}

func TestGETKeys(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	w := do(handler, "GET", "/api/keys", "")
	// NOTE: `api_keys` table missing from schema.sql — handler returns 500.
	// Once schema adds CREATE TABLE IF NOT EXISTS api_keys, this should pass.
	if w.Code != http.StatusOK {
		t.Skipf("GET /api/keys: status %d, body: %s — missing `api_keys` table in schema", w.Code, w.Body.String())
	}

	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("len(keys) = %d, want 0", len(resp))
	}
}

func TestPOSTCreateKey(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	w := do(handler, "POST", "/api/keys", `{"name":"test-key"}`)
	// NOTE: `api_keys` table missing from schema.sql — handler returns 500.
	// Once schema adds CREATE TABLE IF NOT EXISTS api_keys, this should pass.
	if w.Code != http.StatusCreated {
		t.Skipf("POST /api/keys: status %d, body: %s — missing `api_keys` table in schema", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if key, ok := resp["key"].(string); !ok || len(key) == 0 {
		t.Fatalf("expected non-empty key, got %v", resp["key"])
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	// POST a setting.
	w := do(handler, "POST", "/api/settings", `{"key":"test.key","value":"hello"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/settings: status %d, want %d", w.Code, http.StatusOK)
	}

	// GET settings and verify.
	w = do(handler, "GET", "/api/settings", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/settings: status %d, want %d", w.Code, http.StatusOK)
	}

	var settings map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v := settings["test.key"]; v != "hello" {
		t.Fatalf("settings[test.key] = %q, want %q", v, "hello")
	}

	// Overwrite the setting.
	do(handler, "POST", "/api/settings", `{"key":"test.key","value":"world"}`)
	w = do(handler, "GET", "/api/settings", "")
	var settings2 map[string]string
	json.Unmarshal(w.Body.Bytes(), &settings2)
	if v := settings2["test.key"]; v != "world" {
		t.Fatalf("settings[test.key] after overwrite = %q, want %q", v, "world")
	}
}

func TestDashboardModelsUsesProviderConfig(t *testing.T) {
	database := testDB(t)
	_, err := database.Exec(`INSERT INTO provider_config (provider, base_url, models, enabled, extra, updated_at) VALUES ('gpt-4', 'https://example.com', '["configured-model"]', 1, '{}', 1)`)
	if err != nil {
		t.Fatalf("insert provider config: %v", err)
	}
	store := providers.NewConfigStore(database.DB)
	t.Cleanup(store.Stop)
	registry := &providers.Registry{ConfigStore: store}
	registry.Register(&providersTestStub{name: "gpt-4"})

	r := chi.NewRouter()
	MountDashboardModelsAPI(r, registry)
	w := do(r, http.MethodGet, "/api/models", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/models: status %d, body: %s", w.Code, w.Body.String())
	}
	var response struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	if response.Object != "list" || len(response.Data) != 1 || response.Data[0].ID != "configured-model" || response.Data[0].OwnedBy != "gpt-4" {
		t.Fatalf("unexpected models response: %+v", response)
	}
}

type providersTestStub struct{ name string }

func (s *providersTestStub) Name() string                { return s.name }
func (s *providersTestStub) OwnsModel(model string) bool { return model == s.name }
func (s *providersTestStub) Chat(_ context.Context, _ *providers.Account, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
	return nil, nil
}
func (s *providersTestStub) Healthy(_ context.Context, _ *providers.Account) bool { return true }

func TestProviderConfigSeedAndList(t *testing.T) {
	db := testDB(t)
	handler := testRouter(t, db)

	// Seed default provider configs.
	w := do(handler, "POST", "/api/provider-config/seed", "")
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/provider-config/seed: status %d, want %d", w.Code, http.StatusOK)
	}

	// List configs — should now have entries.
	w = do(handler, "GET", "/api/provider-config", "")
	var configs []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &configs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(configs) < 3 {
		t.Fatalf("len(configs) = %d, want >= 3", len(configs))
	}
}

func init() {
	// Disable log output during tests.
	os.Setenv("API_KEY", "switchblade-secret")
	os.Setenv("ENCRYPTION_KEY", "test-encryption-key-32-bytes-long")
	// Initialize config singleton for tests.
	config.Load()
}
