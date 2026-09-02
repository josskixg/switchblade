package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
)

// MountReplayAPI registers replay queue endpoints backed by request_logs.replay_status.
func MountReplayAPI(r chi.Router, database *db.DB) {
	r.Get("/api/replay", listReplay(database))
	r.Post("/api/replay", RequireJSONHandler(enqueueReplay(database)))
	r.Delete("/api/replay/{id}", deleteReplay(database))
	r.Get("/api/replay/stats", replayStats(database))
}

func listReplay(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, provider, model, replay_status, created_at FROM request_logs WHERE replay_status IS NOT NULL ORDER BY id DESC LIMIT 500`)
		if err != nil {
			log.Printf("[api] list replay failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var items []map[string]any
		for rows.Next() {
			var id, createdAt int64
			var provider, replayStatus string
			var model *string
			if err := rows.Scan(&id, &provider, &model, &replayStatus, &createdAt); err != nil {
				continue
			}
			items = append(items, map[string]any{
				"id": id, "provider": provider, "model": model,
				"replay_status": replayStatus, "created_at": createdAt,
			})
		}
		jsonOK(w, items)
	}
}

func enqueueReplay(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Provider == "" {
			jsonError(w, http.StatusBadRequest, "provider required")
			return
		}
		now := time.Now().Unix()
		res, err := database.Exec(
			`INSERT INTO request_logs (provider, model, status, replay_status, created_at) VALUES (?, ?, 'pending', 'pending', ?)`,
			body.Provider, body.Model, now,
		)
		if err != nil {
			log.Printf("[api] enqueue replay failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		id, _ := res.LastInsertId()
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]any{"id": id})
	}
}

func deleteReplay(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := database.Exec(`UPDATE request_logs SET replay_status = NULL WHERE id = ?`, id); err != nil {
			log.Printf("[api] delete replay failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "removed"})
	}
}

func replayStats(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT replay_status, COUNT(*) FROM request_logs WHERE replay_status IS NOT NULL GROUP BY replay_status`)
		if err != nil {
			log.Printf("[api] replay stats failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		stats := map[string]int64{}
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err != nil {
				continue
			}
			stats[status] = count
		}
		jsonOK(w, stats)
	}
}
