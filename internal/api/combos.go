// Package api — model combo (fallback chain) management endpoints.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
)

// MountCombosAPI attaches model combo CRUD routes.
func MountCombosAPI(r chi.Router, database *db.DB) {
	r.Get("/api/model-combos", listCombos(database))
	r.Post("/api/model-combos", RequireJSONHandler(createCombo(database)))
	r.Put("/api/model-combos/{id}", RequireJSONHandler(updateCombo(database)))
	r.Delete("/api/model-combos/{id}", deleteCombo(database))
}

func listCombos(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, name, label, models_json, enabled, created_at FROM model_combos ORDER BY id DESC`)
		if err != nil {
			slog.Error("[api] list combos failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var combos []map[string]any
		for rows.Next() {
			var id, enabled, createdAt int64
			var name, label, modelsJSON string
			if err := rows.Scan(&id, &name, &label, &modelsJSON, &enabled, &createdAt); err != nil {
				continue
			}
			combos = append(combos, map[string]any{
				"id": id, "name": name, "label": label, "models_json": modelsJSON,
				"enabled": enabled == 1, "created_at": createdAt,
			})
		}
		jsonOK(w, combos)
	}
}

func createCombo(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name       string `json:"name"`
			Label      string `json:"label"`
			ModelsJSON string `json:"models_json"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			jsonError(w, http.StatusBadRequest, "name required")
			return
		}
		if body.ModelsJSON == "" {
			body.ModelsJSON = "[]"
		}
		_, err := database.Exec(
			`INSERT INTO model_combos (name, label, models_json, enabled, created_at) VALUES (?, ?, ?, 1, ?)`,
			body.Name, body.Label, body.ModelsJSON, time.Now().Unix(),
		)
		if err != nil {
			slog.Error("[api] create combo failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]string{"status": "created"})
	}
}

func updateCombo(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Name       string `json:"name"`
			Label      string `json:"label"`
			ModelsJSON string `json:"models_json"`
			Enabled    *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		enabled := 1
		if body.Enabled != nil && !*body.Enabled {
			enabled = 0
		}
		_, err := database.Exec(
			`UPDATE model_combos SET name = ?, label = ?, models_json = ?, enabled = ?, updated_at = ? WHERE id = ?`,
			body.Name, body.Label, body.ModelsJSON, enabled, time.Now().Unix(), id,
		)
		if err != nil {
			slog.Error("[api] update combo failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "updated"})
	}
}

func deleteCombo(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := database.Exec(`DELETE FROM model_combos WHERE id = ?`, id); err != nil {
			slog.Error("[api] delete combo failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}
