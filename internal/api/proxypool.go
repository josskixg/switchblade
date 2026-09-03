// Package api — proxy pool management endpoints.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
)

// MountProxyPoolAPI attaches proxy pool CRUD routes.
func MountProxyPoolAPI(r chi.Router, database *db.DB) {
	r.Get("/api/proxy-pool", listProxies(database))
	r.Post("/api/proxy-pool", RequireJSONHandler(addProxy(database)))
	r.Put("/api/proxy-pool/{id}", RequireJSONHandler(toggleProxy(database)))
	r.Delete("/api/proxy-pool/{id}", deleteProxy(database))
}

func listProxies(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, url, type, label, status, success_count, fail_count, created_at FROM proxy_pool ORDER BY id DESC`)
		if err != nil {
			slog.Error("[api] list proxies failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var proxies []map[string]any
		for rows.Next() {
			var id, successCount, failCount, createdAt int64
			var url, ptype, label, status string
			if err := rows.Scan(&id, &url, &ptype, &label, &status, &successCount, &failCount, &createdAt); err != nil {
				continue
			}
			proxies = append(proxies, map[string]any{
				"id": id, "url": url, "type": ptype, "label": label, "status": status,
				"success_count": successCount, "fail_count": failCount, "created_at": createdAt,
			})
		}
		jsonOK(w, proxies)
	}
}

func addProxy(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL   string `json:"url"`
			Type  string `json:"type"`
			Label string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
			jsonError(w, http.StatusBadRequest, "url required")
			return
		}
		if err := ValidateBaseURL(body.URL); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Type == "" {
			body.Type = "http"
		}
		_, err := database.Exec(
			`INSERT INTO proxy_pool (url, type, label, status, created_at) VALUES (?, ?, ?, 'active', ?)`,
			body.URL, body.Type, body.Label, time.Now().Unix(),
		)
		if err != nil {
			slog.Error("[api] add proxy failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]string{"status": "created"})
	}
}

func toggleProxy(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status == "" {
			// ponytail: toggle between active/disabled when no status provided
			database.Exec(
				`UPDATE proxy_pool SET status = CASE WHEN status = 'active' THEN 'disabled' ELSE 'active' END, updated_at = ? WHERE id = ?`,
				time.Now().Unix(), id,
			)
		} else {
			database.Exec(
				`UPDATE proxy_pool SET status = ?, updated_at = ? WHERE id = ?`,
				body.Status, time.Now().Unix(), id,
			)
		}
		jsonOK(w, map[string]string{"status": "updated"})
	}
}

func deleteProxy(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := database.Exec(`DELETE FROM proxy_pool WHERE id = ?`, id); err != nil {
			slog.Error("[api] delete proxy failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}
