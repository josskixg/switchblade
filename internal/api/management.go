// Package api — management API handlers for accounts, stats, keys, filters.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/config"
	"switchblade/internal/crypto"
	"switchblade/internal/db"
)

// MountManagementAPI attaches all /api/* routes to the given router.
func MountManagementAPI(r chi.Router, database *db.DB) {
	// Accounts
	r.Get("/api/accounts", listAccounts(database))
	r.Post("/api/accounts", RequireJSONHandler(createAccount(database)))
	r.Put("/api/accounts/{id}", RequireJSONHandler(updateAccount(database)))
	r.Delete("/api/accounts/{id}", deleteAccount(database))

	// Stats
	r.Get("/api/stats", getStats(database))
	r.Get("/api/logs", getLogs(database))

	// Settings (key-value)
	r.Get("/api/settings", getSettings(database))
	r.Post("/api/settings", RequireJSONHandler(setSetting(database)))

	// Filter rules
	r.Get("/api/filters", listFilters(database))
	r.Post("/api/filters", RequireJSONHandler(createFilter(database)))
	r.Delete("/api/filters/{id}", deleteFilter(database))
}

// --- Accounts ---

func listAccounts(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := r.URL.Query().Get("provider")
		var rows *sql.Rows
		var err error
		if provider != "" {
			rows, err = database.Query(
				`SELECT id, provider, email, status, enabled, quota_limit, quota_remaining, last_used_at, created_at FROM accounts WHERE provider = ? ORDER BY id DESC`,
				provider)
		} else {
			rows, err = database.Query(
				`SELECT id, provider, email, status, enabled, quota_limit, quota_remaining, last_used_at, created_at FROM accounts ORDER BY id DESC`)
		}
		if err != nil {
			log.Printf("[api] list accounts failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var accounts []map[string]any
		for rows.Next() {
			var id, lastUsedAt, createdAt int64
			var enabled int
			var quotaLimit, quotaRemaining float64
			var prov, email, status string
			if err := rows.Scan(&id, &prov, &email, &status, &enabled, &quotaLimit, &quotaRemaining, &lastUsedAt, &createdAt); err != nil {
				continue
			}
			accounts = append(accounts, map[string]any{
				"id": id, "provider": prov, "email": email, "status": status,
				"enabled": enabled == 1, "quota_limit": quotaLimit,
				"quota_remaining": quotaRemaining, "last_used_at": lastUsedAt, "created_at": createdAt,
			})
		}
		jsonOK(w, accounts)
	}
}

func createAccount(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Provider string  `json:"provider"`
			Email    string  `json:"email"`
			Password string  `json:"password"`
			Tokens   string  `json:"tokens"`
			Quota    float64 `json:"quota_limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		now := time.Now().Unix()
		encryptedPassword := crypto.Encrypt(body.Password, config.C.EncryptionKey)
		res, err := database.Exec(
			`INSERT INTO accounts (provider, email, password, status, enabled, tokens, quota_limit, quota_remaining, created_at, updated_at)
			 VALUES (?, ?, ?, 'pending', 1, ?, ?, ?, ?, ?)`,
			body.Provider, body.Email, encryptedPassword, body.Tokens, body.Quota, body.Quota, now, now,
		)
		if err != nil {
			log.Printf("[api] create account failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		id, _ := res.LastInsertId()
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]any{"id": id})
	}
}

func updateAccount(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		// ponytail: only allow safe fields to be updated
		if status, ok := body["status"].(string); ok {
			database.Exec(`UPDATE accounts SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().Unix(), id)
		}
		if enabled, ok := body["enabled"].(bool); ok {
			e := 0
			if enabled {
				e = 1
			}
			database.Exec(`UPDATE accounts SET enabled = ?, updated_at = ? WHERE id = ?`, e, time.Now().Unix(), id)
		}
		jsonOK(w, map[string]string{"status": "updated"})
	}
}

func deleteAccount(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := database.Exec(`DELETE FROM accounts WHERE id = ?`, id); err != nil {
			log.Printf("[api] delete account failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}

// --- Stats ---

func getStats(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		since := r.URL.Query().Get("since")
		if since == "" {
			since = time.Now().AddDate(0, 0, -7).UTC().Format("2006-01-02")
		}
		stats, err := database.GetUsageStats(since)
		if err != nil {
			log.Printf("[api] get stats failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, stats)
	}
}

func getLogs(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		logs, err := database.GetRecentLogs(limit)
		if err != nil {
			log.Printf("[api] get logs failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, logs)
	}
}

// --- Settings ---

func getSettings(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(`SELECT key, value FROM settings ORDER BY key`)
		if err != nil {
			log.Printf("[api] get settings failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()
		result := make(map[string]string)
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				result[k] = v
			}
		}
		jsonOK(w, result)
	}
}

func setSetting(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		_, err := database.Exec(
			`INSERT INTO settings (key, tenant_id, value) VALUES (?, '_system', ?) ON CONFLICT(key, tenant_id) DO UPDATE SET value = excluded.value`,
			body.Key, body.Value,
		)
		if err != nil {
			log.Printf("[api] set setting failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

// --- Filters ---

func listFilters(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT rule_id, pattern, replacement, is_active, is_regex, sort_order FROM filter_rules ORDER BY sort_order`)
		if err != nil {
			log.Printf("[api] list filters failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()
		var rules []map[string]any
		for rows.Next() {
			var ruleID, pattern, replacement string
			var isActive, isRegex, sortOrder int
			if err := rows.Scan(&ruleID, &pattern, &replacement, &isActive, &isRegex, &sortOrder); err != nil {
				continue
			}
			rules = append(rules, map[string]any{
				"rule_id": ruleID, "pattern": pattern, "replacement": replacement,
				"is_active": isActive == 1, "is_regex": isRegex == 1, "sort_order": sortOrder,
			})
		}
		jsonOK(w, rules)
	}
}

func createFilter(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RuleID      string `json:"rule_id"`
			Pattern     string `json:"pattern"`
			Replacement string `json:"replacement"`
			IsRegex     bool   `json:"is_regex"`
			SortOrder   int    `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		isRegex := 0
		if body.IsRegex {
			isRegex = 1
		}
		var exists int
		database.QueryRow(`SELECT COUNT(*) FROM filter_rules WHERE rule_id = ?`, body.RuleID).Scan(&exists)
		if exists > 0 {
			jsonError(w, http.StatusConflict, "filter rule_id already exists")
			return
		}
		_, err := database.Exec(
			`INSERT INTO filter_rules (rule_id, pattern, replacement, is_active, is_regex, sort_order) VALUES (?, ?, ?, 1, ?, ?)`,
			body.RuleID, body.Pattern, body.Replacement, isRegex, body.SortOrder,
		)
		if err != nil {
			log.Printf("[api] create filter failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]string{"status": "created"})
	}
}

func deleteFilter(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := database.Exec(`DELETE FROM filter_rules WHERE rule_id = ?`, id); err != nil {
			log.Printf("[api] delete filter failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}

// --- helpers ---

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
