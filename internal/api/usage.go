// Package api provides HTTP handlers for the Switchblade API.
package api

import (
	"database/sql"
	"net/http"
	"time"
)

// RecordUsage inserts a usage record for token/cost tracking.
func RecordUsage(db *sql.DB, tenantID, keyID, model string, promptTokens, completionTokens, totalTokens, costCents int) error {
	now := time.Now().Unix()
	_, err := db.Exec(`
		INSERT INTO usage_records (tenant_id, api_key_id, model, prompt_tokens, completion_tokens, total_tokens, cost_cents, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'success', ?)
	`, tenantID, keyID, model, promptTokens, completionTokens, totalTokens, costCents, now)
	return err
}

// CheckQuota returns true if tenant is within daily AND monthly limits.
// Queries tenants.daily_token_limit + monthly limit (daily * 30 as default).
func CheckQuota(db *sql.DB, tenantID string) (bool, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()

	// Get tier limits
	var dailyLimit, monthlyLimit sql.NullInt64
	err := db.QueryRow(`
		SELECT t.daily_token_limit, t.daily_token_limit * 30
		FROM tenants tn
		LEFT JOIN tiers t ON tn.tier_id = t.id
		WHERE tn.id = ?
	`, tenantID).Scan(&dailyLimit, &monthlyLimit)
	if err != nil {
		// No tier = no limit
		return true, nil
	}

	// Daily usage
	var dailyUsed int64
	err = db.QueryRow(`
		SELECT COALESCE(SUM(total_tokens), 0)
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ?
	`, tenantID, todayStart).Scan(&dailyUsed)
	if err != nil {
		return true, nil
	}
	if dailyLimit.Valid && dailyUsed > dailyLimit.Int64 {
		return false, nil
	}

	// Monthly usage
	var monthlyUsed int64
	err = db.QueryRow(`
		SELECT COALESCE(SUM(total_tokens), 0)
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ?
	`, tenantID, monthStart).Scan(&monthlyUsed)
	if err != nil {
		return true, nil
	}
	if monthlyLimit.Valid && monthlyUsed > monthlyLimit.Int64 {
		return false, nil
	}

	return true, nil
}

// GetUsageStats returns usage statistics for a tenant.
// period: "daily" or "monthly" (both returned always).
func GetUsageStats(db *sql.DB, tenantID string, period string) (map[string]interface{}, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()

	stats := make(map[string]interface{})

	// Today
	var todayTokens, todayRequests, todayCost int64
	err := db.QueryRow(`
		SELECT COALESCE(SUM(total_tokens), 0), COUNT(*), COALESCE(SUM(cost_cents), 0)
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ?
	`, tenantID, todayStart).Scan(&todayTokens, &todayRequests, &todayCost)
	if err != nil {
		return nil, err
	}

	// Month
	var monthTokens, monthRequests, monthCost int64
	err = db.QueryRow(`
		SELECT COALESCE(SUM(total_tokens), 0), COUNT(*), COALESCE(SUM(cost_cents), 0)
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ?
	`, tenantID, monthStart).Scan(&monthTokens, &monthRequests, &monthCost)
	if err != nil {
		return nil, err
	}

	// Limits
	var dailyLimit, monthlyLimit sql.NullInt64
	db.QueryRow(`
		SELECT t.daily_token_limit, t.daily_token_limit * 30
		FROM tenants tn
		LEFT JOIN tiers t ON tn.tier_id = t.id
		WHERE tn.id = ?
	`, tenantID).Scan(&dailyLimit, &monthlyLimit)

	stats["today_tokens"] = todayTokens
	stats["month_tokens"] = monthTokens
	stats["today_requests"] = todayRequests
	stats["month_requests"] = monthRequests
	stats["today_cost_cents"] = todayCost
	stats["month_cost_cents"] = monthCost

	if dailyLimit.Valid {
		remaining := dailyLimit.Int64 - todayTokens
		if remaining < 0 {
			remaining = 0
		}
		stats["remaining_daily_quota"] = remaining
		stats["daily_limit"] = dailyLimit.Int64
	} else {
		stats["remaining_daily_quota"] = -1 // unlimited
		stats["daily_limit"] = -1
	}

	if monthlyLimit.Valid {
		remaining := monthlyLimit.Int64 - monthTokens
		if remaining < 0 {
			remaining = 0
		}
		stats["remaining_monthly_quota"] = remaining
		stats["monthly_limit"] = monthlyLimit.Int64
	} else {
		stats["remaining_monthly_quota"] = -1
		stats["monthly_limit"] = -1
	}

	return stats, nil
}

// HandleUsageStats handles GET /api/usage?tenant_id=xxx&period=daily
func HandleUsageStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			jsonError(w, http.StatusBadRequest, "tenant_id is required")
			return
		}

		period := r.URL.Query().Get("period")
		if period == "" {
			period = "daily"
		}

		stats, err := GetUsageStats(db, tenantID, period)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to get usage stats")
			return
		}

		jsonOK(w, stats)
	}
}

// HandleListUsageRecords handles GET /api/usage/records?tenant_id=xxx&limit=100
func HandleListUsageRecords(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			jsonError(w, http.StatusBadRequest, "tenant_id is required")
			return
		}

		limit := r.URL.Query().Get("limit")
		if limit == "" {
			limit = "100"
		}

		rows, err := db.Query(`
			SELECT id, api_key_id, model, prompt_tokens, completion_tokens, total_tokens, cost_cents, status, created_at
			FROM usage_records
			WHERE tenant_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		`, tenantID, limit)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		type record struct {
			ID               int64  `json:"id"`
			APIKeyID         *int64 `json:"api_key_id"`
			Model            string `json:"model"`
			PromptTokens     int64  `json:"prompt_tokens"`
			CompletionTokens int64  `json:"completion_tokens"`
			TotalTokens      int64  `json:"total_tokens"`
			CostCents        int64  `json:"cost_cents"`
			Status           string `json:"status"`
			CreatedAt        int64  `json:"created_at"`
		}

		var records []record
		for rows.Next() {
			var r record
			var kid sql.NullInt64
			if err := rows.Scan(&r.ID, &kid, &r.Model, &r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &r.CostCents, &r.Status, &r.CreatedAt); err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to scan record")
				return
			}
			if kid.Valid {
				r.APIKeyID = &kid.Int64
			}
			records = append(records, r)
		}

		if records == nil {
			records = []record{}
		}

		jsonOK(w, map[string]interface{}{"records": records})
	}
}
