package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"switchblade/internal/db"
)

// HandleTierQuotaStatus returns quota stats per account tier (subscription/cheap/free)
func HandleTierQuotaStatus(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(`
			SELECT 
				tier,
				COUNT(*) as total_accounts,
				SUM(CASE WHEN status = 'active' AND enabled = 1 THEN 1 ELSE 0 END) as active_accounts,
				SUM(quota_limit) as total_quota,
				SUM(quota_remaining) as remaining_quota,
				SUM(quota_limit - quota_remaining) as used_quota
			FROM accounts
			WHERE tier IN ('subscription', 'cheap', 'free')
			GROUP BY tier
			ORDER BY 
				CASE tier 
					WHEN 'subscription' THEN 1 
					WHEN 'cheap' THEN 2 
					WHEN 'free' THEN 3 
				END
		`)
		if err != nil {
			slog.Error("[api] tier quota status failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		type tierStats struct {
			Tier           string  `json:"tier"`
			TotalAccounts  int     `json:"total_accounts"`
			ActiveAccounts int     `json:"active_accounts"`
			TotalQuota     float64 `json:"total_quota"`
			RemainingQuota float64 `json:"remaining_quota"`
			UsedQuota      float64 `json:"used_quota"`
		}

		var stats []tierStats
		for rows.Next() {
			var s tierStats
			if err := rows.Scan(&s.Tier, &s.TotalAccounts, &s.ActiveAccounts, &s.TotalQuota, &s.RemainingQuota, &s.UsedQuota); err != nil {
				slog.Error("[api] tier quota status scan failed", "err", err)
				continue
			}
			stats = append(stats, s)
		}

		// Ensure all tiers are present even if empty
		tierMap := make(map[string]*tierStats)
		for i := range stats {
			tierMap[stats[i].Tier] = &stats[i]
		}

		result := make([]tierStats, 0, 3)
		for _, tier := range []string{"subscription", "cheap", "free"} {
			if s, ok := tierMap[tier]; ok {
				result = append(result, *s)
			} else {
				result = append(result, tierStats{Tier: tier})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// HandleServicesStats returns usage stats per service kind
func HandleServicesStats(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get stats from request_logs table
		rows, err := database.Query(`
			SELECT 
				COALESCE(service_kind, 'chat') as service_kind,
				COUNT(*) as request_count,
				SUM(prompt_tokens) as total_prompt_tokens,
				SUM(completion_tokens) as total_completion_tokens,
				SUM(total_tokens) as total_tokens
			FROM request_logs
			WHERE created_at > strftime('%s', 'now', '-30 days')
			GROUP BY service_kind
			ORDER BY request_count DESC
		`)
		if err != nil {
			slog.Error("[api] services stats failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		type serviceStats struct {
			ServiceKind           string `json:"service_kind"`
			RequestCount          int    `json:"request_count"`
			TotalPromptTokens     int    `json:"total_prompt_tokens"`
			TotalCompletionTokens int    `json:"total_completion_tokens"`
			TotalTokens           int    `json:"total_tokens"`
		}

		stats := make([]serviceStats, 0)
		for rows.Next() {
			var s serviceStats
			if err := rows.Scan(&s.ServiceKind, &s.RequestCount, &s.TotalPromptTokens, &s.TotalCompletionTokens, &s.TotalTokens); err != nil {
				slog.Error("[api] services stats scan failed", "err", err)
				continue
			}
			stats = append(stats, s)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}
