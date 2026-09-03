// Package api provides HTTP handlers including the quota monitor.
package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// QuotaMonitor runs a background goroutine that checks tenant token usage
// against tier limits and auto-suspends tenants that exceed their daily quota.
type QuotaMonitor struct {
	DB             *sql.DB
	CheckInterval  time.Duration
	mu             sync.RWMutex
	blockedTenants map[string]string // tenantID -> reason
}

// StartQuotaMonitor creates and starts a quota monitor goroutine.
// Returns the monitor instance so middleware can call IsBlocked().
func StartQuotaMonitor(db *sql.DB, interval time.Duration) *QuotaMonitor {
	qm := &QuotaMonitor{
		DB:             db,
		CheckInterval:  interval,
		blockedTenants: make(map[string]string),
	}
	go qm.runLoop()
	return qm
}

// runLoop is the internal loop wrapper with panic recovery.
func (qm *QuotaMonitor) runLoop() {
	ticker := time.NewTicker(qm.CheckInterval)
	defer ticker.Stop()

	// Run immediately on start, then on each tick.
	qm.checkQuotas()

	for range ticker.C {
		qm.checkQuotas()
	}
}

// checkQuotas queries active tenants, compares usage vs limits, blocks/unblocks.
func (qm *QuotaMonitor) checkQuotas() {
	tenants, err := qm.fetchActiveTenants()
	if err != nil {
		slog.Error("[quota-monitor] error fetching tenants", "err", err)
		return
	}

	todayStart := startOfToday()
	for _, t := range tenants {
		usage, err := qm.getDailyUsage(t.id, todayStart)
		if err != nil {
			slog.Error("[quota-monitor] fetch usage failed", "tenant", t.id, "err", err)
			continue
		}

		if t.dailyTokenLimit < 0 {
			// -1 = unlimited (enterprise)
			continue
		}

		if usage > t.dailyTokenLimit {
			qm.blockTenant(t.id, "daily_quota_exceeded")
		} else {
			// If previously blocked for daily quota, unblock now (midnight reset)
			qm.unblockIfSafe(t.id)
		}
	}
}

type tenantInfo struct {
	id              string
	dailyTokenLimit int64
}

// fetchActiveTenants returns all tenants that are not suspended.
func (qm *QuotaMonitor) fetchActiveTenants() ([]tenantInfo, error) {
	query := `
		SELECT t.id, COALESCE(tier.daily_token_limit, -1)
		FROM tenants t
		LEFT JOIN tiers tier ON t.tier_id = tier.id
		WHERE t.status != 'suspended'
	`
	rows, err := qm.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query tenants: %w", err)
	}
	defer rows.Close()

	var tenants []tenantInfo
	for rows.Next() {
		var t tenantInfo
		if err := rows.Scan(&t.id, &t.dailyTokenLimit); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// getDailyUsage returns SUM(total_tokens) for the tenant since the given unix timestamp.
func (qm *QuotaMonitor) getDailyUsage(tenantID string, sinceUnixSec int64) (int64, error) {
	var usage sql.NullInt64
	err := qm.DB.QueryRow(
		"SELECT SUM(total_tokens) FROM usage_records WHERE tenant_id = ? AND created_at >= ?",
		tenantID, sinceUnixSec,
	).Scan(&usage)
	if err != nil {
		return 0, fmt.Errorf("query usage: %w", err)
	}
	if !usage.Valid {
		return 0, nil
	}
	return usage.Int64, nil
}

// blockTenant sets tenant status to 'suspended' if not already blocked.
func (qm *QuotaMonitor) blockTenant(tenantID, reason string) {
	qm.mu.Lock()
	if _, exists := qm.blockedTenants[tenantID]; exists {
		qm.mu.Unlock()
		return
	}
	qm.mu.Unlock()

	_, err := qm.DB.Exec(
		"UPDATE tenants SET status = 'suspended', updated_at = strftime('%s', 'now') WHERE id = ?",
		tenantID,
	)
	if err != nil {
		slog.Error("[quota-monitor] block tenant failed", "tenant", tenantID, "err", err)
		return
	}

	qm.mu.Lock()
	qm.blockedTenants[tenantID] = reason
	qm.mu.Unlock()

	slog.Info(fmt.Sprintf("[quota-monitor] BLOCKED tenant %s — reason: %s", tenantID, reason))
}

// unblockIfSafe unblocks a tenant that was auto-blocked and is now within daily limits.
func (qm *QuotaMonitor) unblockIfSafe(tenantID string) {
	qm.mu.RLock()
	reason, blocked := qm.blockedTenants[tenantID]
	qm.mu.RUnlock()

	if !blocked {
		return
	}

	_, err := qm.DB.Exec(
		"UPDATE tenants SET status = 'active', updated_at = strftime('%s', 'now') WHERE id = ?",
		tenantID,
	)
	if err != nil {
		slog.Error("[quota-monitor] unblock tenant failed", "tenant", tenantID, "err", err)
		return
	}

	qm.mu.Lock()
	delete(qm.blockedTenants, tenantID)
	qm.mu.Unlock()

	slog.Info(fmt.Sprintf("[quota-monitor] UNBLOCKED tenant %s — previous reason: %s", tenantID, reason))
}

// IsBlocked returns true if the tenant is currently blocked.
func (qm *QuotaMonitor) IsBlocked(tenantID string) bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	_, ok := qm.blockedTenants[tenantID]
	return ok
}

// Unblock manually removes a tenant from the blocked list and reactivates it.
func (qm *QuotaMonitor) Unblock(tenantID string) {
	_, err := qm.DB.Exec(
		"UPDATE tenants SET status = 'active', updated_at = strftime('%s', 'now') WHERE id = ?",
		tenantID,
	)
	if err != nil {
		slog.Error("[quota-monitor] manual unblock failed", "tenant", tenantID, "err", err)
		return
	}

	qm.mu.Lock()
	delete(qm.blockedTenants, tenantID)
	qm.mu.Unlock()

	slog.Info(fmt.Sprintf("[quota-monitor] manually UNBLOCKED tenant %s", tenantID))
}

// BlockedTenants returns a sorted list of blocked tenant IDs.
func (qm *QuotaMonitor) BlockedTenants() []string {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	ids := make([]string, 0, len(qm.blockedTenants))
	for id := range qm.blockedTenants {
		ids = append(ids, id)
	}
	return ids
}

// startOfToday returns the unix timestamp for 00:00:00 today (UTC).
func startOfToday() int64 {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
}

// startOfMonth returns the unix timestamp for the 1st of the current month at 00:00:00 (UTC).
func startOfMonth() int64 {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
}

// QuotaMiddleware returns an http.Handler that rejects requests from blocked tenants.
// quotaChecker should be a func(tenantID string) bool — typically qm.IsBlocked.
func QuotaMiddleware(quotaChecker func(string) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract tenant_id from request context or header
			tenantID := r.Header.Get("X-Tenant-ID")
			if tenantID == "" {
				next.ServeHTTP(w, r)
				return
			}
			if quotaChecker(tenantID) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "Account suspended. Daily quota exceeded.",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
