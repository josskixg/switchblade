// Package api — relay node management endpoints.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
)

// MountRelayMgmtAPI attaches relay node CRUD routes.
func MountRelayMgmtAPI(r chi.Router, database *db.DB) {
	// ponytail: create table here so schema.sql stays untouched
	database.Exec(`CREATE TABLE IF NOT EXISTS relay_nodes (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		url        TEXT    NOT NULL,
		name       TEXT    NOT NULL DEFAULT '',
		secret     TEXT    NOT NULL DEFAULT '',
		active     INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL
	)`)

	r.Get("/api/relay/nodes", listRelayNodes(database))
	r.Post("/api/relay/nodes", RequireJSONHandler(createRelayNode(database)))
	r.Delete("/api/relay/nodes/{id}", deleteRelayNode(database))
	r.Get("/api/relay/stats", getRelayStats(database))
}

func listRelayNodes(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, url, name, secret, active, created_at FROM relay_nodes ORDER BY id DESC`)
		if err != nil {
			slog.Error("[api] list relay nodes failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var nodes []map[string]any
		for rows.Next() {
			var id, active, createdAt int64
			var url, name, secret string
			if err := rows.Scan(&id, &url, &name, &secret, &active, &createdAt); err != nil {
				continue
			}
			nodes = append(nodes, map[string]any{
				"id": id, "url": url, "name": name, "secret": secret,
				"active": active == 1, "created_at": createdAt,
			})
		}
		jsonOK(w, nodes)
	}
}

func createRelayNode(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL    string `json:"url"`
			Name   string `json:"name"`
			Secret string `json:"secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
			jsonError(w, http.StatusBadRequest, "url required")
			return
		}
		if err := ValidateBaseURL(body.URL); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		_, err := database.Exec(
			`INSERT INTO relay_nodes (url, name, secret, created_at) VALUES (?, ?, ?, ?)`,
			body.URL, body.Name, body.Secret, time.Now().Unix(),
		)
		if err != nil {
			slog.Error("[api] create relay node failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]string{"status": "created"})
	}
}

func deleteRelayNode(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := database.Exec(`DELETE FROM relay_nodes WHERE id = ?`, id); err != nil {
			slog.Error("[api] delete relay node failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}

func getRelayStats(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var active, inactive int
		database.QueryRow(`SELECT COUNT(*) FROM relay_nodes WHERE active = 1`).Scan(&active)
		database.QueryRow(`SELECT COUNT(*) FROM relay_nodes WHERE active = 0`).Scan(&inactive)
		jsonOK(w, map[string]any{"active": active, "inactive": inactive})
	}
}
