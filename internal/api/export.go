package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"switchblade/internal/config"
	"switchblade/internal/crypto"
	"switchblade/internal/db"
)

// MountExportAPI registers export/import endpoints.
func MountExportAPI(r chi.Router, database *db.DB) {
	r.Get("/api/export/accounts", exportAccounts(database))
	r.Get("/api/export/logs", exportLogs(database))
	r.Post("/api/import/accounts", RequireJSONHandler(importAccounts(database)))
}

func exportAccounts(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, provider, email, status, enabled, quota_limit, quota_remaining,
			        last_used_at, created_at, metadata
			 FROM accounts ORDER BY id`,
		)
		if err != nil {
			slog.Error("[api] export accounts failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		type row struct {
			ID             int64   `json:"id"`
			Provider       string  `json:"provider"`
			Email          string  `json:"email"`
			Status         string  `json:"status"`
			Enabled        bool    `json:"enabled"`
			QuotaLimit     float64 `json:"quota_limit"`
			QuotaRemaining float64 `json:"quota_remaining"`
			LastUsedAt     int64   `json:"last_used_at"`
			CreatedAt      int64   `json:"created_at"`
			Metadata       string  `json:"metadata,omitempty"`
		}
		var out []row
		for rows.Next() {
			var a row
			var enabled int
			if err := rows.Scan(&a.ID, &a.Provider, &a.Email, &a.Status, &enabled,
				&a.QuotaLimit, &a.QuotaRemaining, &a.LastUsedAt, &a.CreatedAt, &a.Metadata); err != nil {
				slog.Error("[api] export accounts scan failed", "err", err)
				jsonError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			a.Enabled = enabled == 1
			out = append(out, a)
		}
		jsonOK(w, out)
	}
}

func exportLogs(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		since := r.URL.Query().Get("since")

		query := `SELECT id, provider, model, status, duration_ms, total_tokens, credits_used, created_at, error_message
		          FROM request_logs`
		args := []any{}
		if since != "" {
			// since is YYYY-MM-DD; convert to unix via strftime for comparison
			query += " WHERE created_at >= strftime('%s', ?)"
			args = append(args, since)
		}
		query += " ORDER BY id"

		rows, err := database.Query(query, args...)
		if err != nil {
			slog.Error("[api] export logs failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var out []map[string]any
		for rows.Next() {
			var id, totalTokens, createdAt int64
			var durationMs int64
			var creditsUsed float64
			var provider, status string
			var model, errMsg *string
			if err := rows.Scan(&id, &provider, &model, &status, &durationMs, &totalTokens, &creditsUsed, &createdAt, &errMsg); err != nil {
				slog.Error("[api] export logs scan failed", "err", err)
				jsonError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			entry := map[string]any{
				"id":           id,
				"provider":     provider,
				"status":       status,
				"duration_ms":  durationMs,
				"total_tokens": totalTokens,
				"credits_used": creditsUsed,
				"created_at":   createdAt,
			}
			if model != nil {
				entry["model"] = *model
			}
			if errMsg != nil {
				entry["error_message"] = *errMsg
			}
			out = append(out, entry)
		}
		jsonOK(w, out)
	}
}

func importAccounts(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type importRow struct {
			Provider string `json:"provider"`
			Email    string `json:"email"`
			Password string `json:"password"` // pre-encrypted or plain
			Status   string `json:"status"`
			Enabled  *bool  `json:"enabled"`
			Metadata string `json:"metadata"`
		}
		var body []importRow
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		now := time.Now().Unix()
		inserted, updated := 0, 0
		for _, a := range body {
			if a.Provider == "" || a.Email == "" {
				continue
			}
			enabled := 1
			if a.Enabled != nil && !*a.Enabled {
				enabled = 0
			}
			status := a.Status
			if status == "" {
				status = "pending"
			}
			// pre-encrypted or plain: keep if decryptable, else encrypt (ponytail: avoid double-encrypt)
			pw := a.Password
			if pw != "" && config.C != nil && config.C.EncryptionKey != "" {
				if crypto.Decrypt(pw, config.C.EncryptionKey) == "" {
					pw = crypto.Encrypt(pw, config.C.EncryptionKey)
				}
			}
			// upsert by (provider, email)
			res, err := database.Exec(`
				INSERT INTO accounts (provider, email, password, status, enabled, metadata, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(provider, email) DO UPDATE SET
					status   = excluded.status,
					enabled  = excluded.enabled,
					metadata = excluded.metadata,
					updated_at = ?`,
				a.Provider, a.Email, pw, status, enabled, a.Metadata, now, now,
			)
			if err != nil {
				slog.Error("[api] import accounts failed", "err", err)
				jsonError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			rows, _ := res.RowsAffected()
			if rows == 1 {
				inserted++
			} else {
				updated++
			}
		}
		jsonOK(w, map[string]any{"inserted": inserted, "updated": updated})
	}
}
