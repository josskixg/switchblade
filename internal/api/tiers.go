// Package api — CRUD handlers for subscription tiers.
package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
)

// MountTierAPI attaches /api/tiers/* and /api/tenants/{id}/tier routes.
func MountTierAPI(r chi.Router, database *db.DB) {
	r.Get("/api/tiers", handleListTiers(database))
	r.Get("/api/tiers/defaults", handleGetTierDefaults(database))
	r.Get("/api/tiers/{id}", handleGetTier(database))
	r.Post("/api/tiers", RequireJSONHandler(handleCreateTier(database)))
	r.Put("/api/tiers/{id}", RequireJSONHandler(handleUpdateTier(database)))
	r.Delete("/api/tiers/{id}", handleDeleteTier(database))

	r.Put("/api/tenants/{id}/tier", RequireJSONHandler(handleAssignTenantTier(database)))
}

func handleListTiers(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents, created_at FROM tiers ORDER BY monthly_price_cents`)
		if err != nil {
			slog.Error("[api] list tiers failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var tiers []map[string]any
		for rows.Next() {
			var id, name string
			var descPtr *string
			var maxKeys, maxModels, rateLimit, dailyLimit, priceCents, createdAt int64
			if err := rows.Scan(&id, &name, &descPtr, &maxKeys, &maxModels, &rateLimit, &dailyLimit, &priceCents, &createdAt); err != nil {
				continue
			}
			description := ""
			if descPtr != nil {
				description = *descPtr
			}
			tiers = append(tiers, map[string]any{
				"id":                    id,
				"name":                  name,
				"description":           description,
				"max_api_keys":          maxKeys,
				"max_models":            maxModels,
				"rate_limit_per_minute": rateLimit,
				"daily_token_limit":     dailyLimit,
				"monthly_price_cents":   priceCents,
				"created_at":            createdAt,
			})
		}
		jsonOK(w, tiers)
	}
}

func handleGetTier(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var name string
		var descPtr *string
		var maxKeys, maxModels, rateLimit, dailyLimit, priceCents, createdAt int64
		err := database.QueryRow(
			`SELECT id, name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents, created_at FROM tiers WHERE id = ?`,
			id,
		).Scan(&id, &name, &descPtr, &maxKeys, &maxModels, &rateLimit, &dailyLimit, &priceCents, &createdAt)
		if err != nil {
			if err == sql.ErrNoRows {
				jsonError(w, http.StatusNotFound, "tier not found")
			} else {
				slog.Error("[api] get tier failed", "err", err)
				jsonError(w, http.StatusInternalServerError, "internal server error")
			}
			return
		}

		description := ""
		if descPtr != nil {
			description = *descPtr
		}
		jsonOK(w, map[string]any{
			"id":                    id,
			"name":                  name,
			"description":           description,
			"max_api_keys":          maxKeys,
			"max_models":            maxModels,
			"rate_limit_per_minute": rateLimit,
			"daily_token_limit":     dailyLimit,
			"monthly_price_cents":   priceCents,
			"created_at":            createdAt,
		})
	}
}

func handleGetTierDefaults(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT name, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit FROM tiers ORDER BY monthly_price_cents`)
		if err != nil {
			slog.Error("[api] get tier defaults failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		defaults := map[string]map[string]any{}
		for rows.Next() {
			var name string
			var maxKeys, maxModels, rateLimit, dailyLimit int64
			if err := rows.Scan(&name, &maxKeys, &maxModels, &rateLimit, &dailyLimit); err == nil {
				defaults[name] = map[string]any{
					"max_api_keys":          maxKeys,
					"max_models":            maxModels,
					"rate_limit_per_minute": rateLimit,
					"daily_token_limit":     dailyLimit,
				}
			}
		}
		jsonOK(w, defaults)
	}
}

func handleCreateTier(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name              string `json:"name"`
			Description       string `json:"description"`
			MaxAPIKeys        int64  `json:"max_api_keys"`
			MaxModels         int64  `json:"max_models"`
			RateLimitPerMin   int64  `json:"rate_limit_per_minute"`
			DailyTokenLimit   int64  `json:"daily_token_limit"`
			MonthlyPriceCents int64  `json:"monthly_price_cents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Name == "" {
			jsonError(w, http.StatusBadRequest, "name required")
			return
		}

		_, err := database.Exec(
			`INSERT INTO tiers (name, description, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit, monthly_price_cents, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			body.Name, body.Description, body.MaxAPIKeys, body.MaxModels,
			body.RateLimitPerMin, body.DailyTokenLimit, body.MonthlyPriceCents, time.Now().Unix(),
		)
		if err != nil {
			slog.Error("[api] create tier failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]string{"status": "created"})
	}
}

func handleUpdateTier(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Name              string `json:"name"`
			Description       string `json:"description"`
			MaxAPIKeys        int64  `json:"max_api_keys"`
			MaxModels         int64  `json:"max_models"`
			RateLimitPerMin   int64  `json:"rate_limit_per_minute"`
			DailyTokenLimit   int64  `json:"daily_token_limit"`
			MonthlyPriceCents int64  `json:"monthly_price_cents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		_, err := database.Exec(
			`UPDATE tiers SET name = ?, description = ?, max_api_keys = ?, max_models = ?,
			 rate_limit_per_minute = ?, daily_token_limit = ?, monthly_price_cents = ?, updated_at = ?
			 WHERE id = ?`,
			body.Name, body.Description, body.MaxAPIKeys, body.MaxModels,
			body.RateLimitPerMin, body.DailyTokenLimit, body.MonthlyPriceCents, time.Now().Unix(), id,
		)
		if err != nil {
			slog.Error("[api] update tier failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "updated"})
	}
}

func handleDeleteTier(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := database.Exec(`DELETE FROM tiers WHERE id = ?`, id); err != nil {
			slog.Error("[api] delete tier failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}

func handleAssignTenantTier(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "id")
		var body struct {
			TierID string `json:"tier_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Verify tenant exists.
		var tierName string
		err := database.QueryRow(`SELECT COALESCE(t.name, 'free') FROM tenants tn LEFT JOIN tiers t ON tn.tier_id = t.id WHERE tn.id = ?`, tenantID).Scan(&tierName)
		if err != nil {
			if err == sql.ErrNoRows {
				jsonError(w, http.StatusNotFound, "tenant not found")
			} else {
				slog.Error("[api] assign tier: tenant lookup failed", "err", err)
				jsonError(w, http.StatusInternalServerError, "internal server error")
			}
			return
		}

		// Verify tier exists and get limits.
		var tierNameNew string
		var maxKeys, maxModels, rateLimit, dailyLimit int64
		if err := database.QueryRow(
			`SELECT name, max_api_keys, max_models, rate_limit_per_minute, daily_token_limit FROM tiers WHERE id = ?`,
			body.TierID,
		).Scan(&tierNameNew, &maxKeys, &maxModels, &rateLimit, &dailyLimit); err != nil {
			jsonError(w, http.StatusNotFound, "tier not found")
			return
		}

		limits := map[string]any{"tier": tierNameNew}
		if tierNameNew != "enterprise" {
			limits["max_api_keys"] = maxKeys
			limits["max_models"] = maxModels
			limits["rate_limit_per_minute"] = rateLimit
			limits["daily_token_limit"] = dailyLimit
		} else {
			limits["max_api_keys"] = -1
			limits["max_models"] = -1
			limits["rate_limit_per_minute"] = rateLimit
			limits["daily_token_limit"] = -1
		}

		_, err = database.Exec(
			`UPDATE tenants SET tier_id = ?, updated_at = ? WHERE id = ?`,
			body.TierID, time.Now().Unix(), tenantID,
		)
		if err != nil {
			slog.Error("[api] assign tier failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, limits)
	}
}
