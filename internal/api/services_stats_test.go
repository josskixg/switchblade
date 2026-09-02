package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"switchblade/internal/db"
)

func TestHandleServicesStats(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	// Create tables
	_, err = database.Exec(`
		CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
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

	// Insert test data with different service kinds
	now := time.Now().Unix()
	_, err = database.Exec(`
		INSERT INTO request_logs (provider, model, service_kind, status, prompt_tokens, completion_tokens, total_tokens, created_at) VALUES
		('openai', 'gpt-4', 'chat', 'success', 100, 200, 300, ?),
		('openai', 'gpt-4', 'chat', 'success', 150, 250, 400, ?),
		('openai', 'text-embedding-3-small', 'embeddings', 'success', 50, 0, 50, ?),
		('openai', 'tts-1', 'tts', 'success', 20, 0, 20, ?),
		('anthropic', 'claude-3', 'chat', 'success', 200, 300, 500, ?)
	`, now, now, now, now, now)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	handler := HandleServicesStats(database)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/stats", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var stats []struct {
		ServiceKind           string `json:"service_kind"`
		RequestCount          int    `json:"request_count"`
		TotalPromptTokens     int    `json:"total_prompt_tokens"`
		TotalCompletionTokens int    `json:"total_completion_tokens"`
		TotalTokens           int    `json:"total_tokens"`
	}

	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(stats) != 3 {
		t.Errorf("expected 3 service kinds, got %d", len(stats))
	}

	// Find chat service
	var chatStats *struct {
		ServiceKind           string `json:"service_kind"`
		RequestCount          int    `json:"request_count"`
		TotalPromptTokens     int    `json:"total_prompt_tokens"`
		TotalCompletionTokens int    `json:"total_completion_tokens"`
		TotalTokens           int    `json:"total_tokens"`
	}
	for i := range stats {
		if stats[i].ServiceKind == "chat" {
			chatStats = &stats[i]
			break
		}
	}

	if chatStats == nil {
		t.Fatal("chat service not found in response")
	}

	if chatStats.RequestCount != 3 {
		t.Errorf("expected 3 chat requests, got %d", chatStats.RequestCount)
	}

	expectedPrompt := 450 // 100 + 150 + 200
	if chatStats.TotalPromptTokens != expectedPrompt {
		t.Errorf("expected %d prompt tokens, got %d", expectedPrompt, chatStats.TotalPromptTokens)
	}

	expectedCompletion := 750 // 200 + 250 + 300
	if chatStats.TotalCompletionTokens != expectedCompletion {
		t.Errorf("expected %d completion tokens, got %d", expectedCompletion, chatStats.TotalCompletionTokens)
	}

	expectedTotal := 1200 // 300 + 400 + 500
	if chatStats.TotalTokens != expectedTotal {
		t.Errorf("expected %d total tokens, got %d", expectedTotal, chatStats.TotalTokens)
	}
}

func TestHandleServicesStats_EmptyDB(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	_, err = database.Exec(`
		CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
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

	handler := HandleServicesStats(database)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/stats", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var stats []interface{}
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return empty array, not error
	if stats == nil {
		t.Error("expected empty array, got nil")
	}
}
