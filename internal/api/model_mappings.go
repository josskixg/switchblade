package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
)

// MountModelMappingsAPI mounts model mapping REST CRUD routes.
func MountModelMappingsAPI(r chi.Router, database *db.DB) {
	r.Get("/api/model-mappings", listModelMappings(database))
	r.Post("/api/model-mappings", RequireJSONHandler(createModelMapping(database)))
	r.Put("/api/model-mappings/{id}", RequireJSONHandler(updateModelMapping(database)))
	r.Delete("/api/model-mappings/{id}", deleteModelMapping(database))
}

func listModelMappings(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, source_pattern, match_type, target_model, enabled, priority, label, created_at, updated_at 
			 FROM model_mappings ORDER BY priority ASC, id DESC`)
		if err != nil {
			log.Printf("[api] list model mappings failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var mappings []map[string]any
		for rows.Next() {
			var id, enabled, priority, createdAt, updatedAt int64
			var sourcePattern, matchType, targetModel, label string
			if err := rows.Scan(&id, &sourcePattern, &matchType, &targetModel, &enabled, &priority, &label, &createdAt, &updatedAt); err != nil {
				continue
			}
			mappings = append(mappings, map[string]any{
				"id":             id,
				"source_pattern": sourcePattern,
				"match_type":     matchType,
				"target_model":   targetModel,
				"enabled":        enabled == 1,
				"priority":       priority,
				"label":          label,
				"created_at":     createdAt,
				"updated_at":     updatedAt,
			})
		}
		jsonOK(w, mappings)
	}
}

func createModelMapping(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SourcePattern string `json:"source_pattern"`
			MatchType     string `json:"match_type"`
			TargetModel   string `json:"target_model"`
			Priority      int    `json:"priority"`
			Label         string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SourcePattern == "" || body.TargetModel == "" {
			jsonError(w, http.StatusBadRequest, "source_pattern and target_model required")
			return
		}
		if body.MatchType == "" {
			body.MatchType = "contains"
		}
		_, err := database.Exec(
			`INSERT INTO model_mappings (source_pattern, match_type, target_model, enabled, priority, label, created_at, updated_at) 
			 VALUES (?, ?, ?, 1, ?, ?, ?, ?)`,
			body.SourcePattern, body.MatchType, body.TargetModel, body.Priority, body.Label, time.Now().Unix(), time.Now().Unix(),
		)
		if err != nil {
			log.Printf("[api] create model mapping failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]string{"status": "created"})
	}
}

func updateModelMapping(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			SourcePattern string `json:"source_pattern"`
			MatchType     string `json:"match_type"`
			TargetModel   string `json:"target_model"`
			Enabled       *bool  `json:"enabled"`
			Priority      *int   `json:"priority"`
			Label         string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Query existing values to preserve
		var currentEnabled, currentPriority int
		var currentSource, currentMatch, currentTarget, currentLabel string
		err := database.QueryRow(`SELECT source_pattern, match_type, target_model, enabled, priority, label FROM model_mappings WHERE id = ?`, id).Scan(
			&currentSource, &currentMatch, &currentTarget, &currentEnabled, &currentPriority, &currentLabel,
		)
		if err != nil {
			jsonError(w, http.StatusNotFound, "model mapping not found")
			return
		}

		sourcePattern := body.SourcePattern
		if sourcePattern == "" {
			sourcePattern = currentSource
		}
		matchType := body.MatchType
		if matchType == "" {
			matchType = currentMatch
		}
		targetModel := body.TargetModel
		if targetModel == "" {
			targetModel = currentTarget
		}
		enabled := currentEnabled
		if body.Enabled != nil {
			if *body.Enabled {
				enabled = 1
			} else {
				enabled = 0
			}
		}
		priority := currentPriority
		if body.Priority != nil {
			priority = *body.Priority
		}
		label := body.Label
		if label == "" {
			label = currentLabel
		}

		_, err = database.Exec(
			`UPDATE model_mappings SET source_pattern = ?, match_type = ?, target_model = ?, enabled = ?, priority = ?, label = ?, updated_at = ? WHERE id = ?`,
			sourcePattern, matchType, targetModel, enabled, priority, label, time.Now().Unix(), id,
		)
		if err != nil {
			log.Printf("[api] update model mapping failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "updated"})
	}
}

func deleteModelMapping(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := database.Exec(`DELETE FROM model_mappings WHERE id = ?`, id); err != nil {
			log.Printf("[api] delete model mapping failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}
