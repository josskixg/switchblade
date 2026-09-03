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

// MountAdminAPI registers owner-only tenant administration routes.
func MountAdminAPI(r chi.Router, database *db.DB) {
	r.Group(func(r chi.Router) {
		r.Use(requireOwner)
		r.Get("/api/tenants", listTenants(database))
		r.Put("/api/tenants/{id}", RequireJSONHandler(updateTenant(database)))
		r.Delete("/api/tenants/{id}", deleteTenant(database))
		r.Get("/api/admin/stats", adminStats(database))
	})
}

func requireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if role, _ := r.Context().Value(ctxRole).(string); role != "owner" {
			jsonError(w, http.StatusForbidden, "owner access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func listTenants(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(`
			SELECT t.id, t.name, t.email, t.status, t.created_at, COALESCE(ti.name, 'free'),
			       (SELECT COUNT(*) FROM request_logs rl WHERE rl.tenant_id = t.id AND rl.created_at >= ?) AS requests_24h
			FROM tenants t
			LEFT JOIN tiers ti ON ti.id = t.tier_id
			ORDER BY t.created_at DESC`, time.Now().Add(-24*time.Hour).Unix())
		if err != nil {
			slog.Error("[api] list tenants failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		tenants := make([]map[string]any, 0)
		for rows.Next() {
			var id, name, email, status, tier string
			var createdAt, requests24h int64
			if err := rows.Scan(&id, &name, &email, &status, &createdAt, &tier, &requests24h); err != nil {
				continue
			}
			tenants = append(tenants, map[string]any{
				"id": id, "name": name, "email": email, "status": status,
				"tier": tier, "requests_24h": requests24h,
				"created_at": time.Unix(createdAt, 0).UTC().Format(time.RFC3339),
			})
		}
		jsonOK(w, tenants)
	}
}

func updateTenant(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Status != "active" && body.Status != "suspended") {
			jsonError(w, http.StatusBadRequest, "status must be active or suspended")
			return
		}
		result, err := database.Exec(`UPDATE tenants SET status = ?, updated_at = ? WHERE id = ?`, body.Status, time.Now().Unix(), chi.URLParam(r, "id"))
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			jsonError(w, http.StatusNotFound, "tenant not found")
			return
		}
		jsonOK(w, map[string]string{"status": body.Status})
	}
}

func deleteTenant(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if current, _ := r.Context().Value(ctxTenantID).(string); id == current {
			jsonError(w, http.StatusBadRequest, "cannot delete current tenant")
			return
		}
		tx, err := database.Begin()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer tx.Rollback()
		for _, query := range []string{
			`DELETE FROM api_key_scopes WHERE api_key_id IN (SELECT id FROM api_keys WHERE tenant_id = ?)`,
			`DELETE FROM usage_records WHERE tenant_id = ?`,
			`DELETE FROM api_keys WHERE tenant_id = ?`,
			`DELETE FROM users WHERE tenant_id = ?`,
		} {
			if _, err := tx.Exec(query, id); err != nil {
				jsonError(w, http.StatusInternalServerError, "internal server error")
				return
			}
		}
		result, err := tx.Exec(`DELETE FROM tenants WHERE id = ?`, id)
		if err != nil {
			jsonError(w, http.StatusConflict, "tenant has related data")
			return
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			jsonError(w, http.StatusNotFound, "tenant not found")
			return
		}
		if err := tx.Commit(); err != nil {
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}

func adminStats(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var activeTenants, requests24h, tokens24h, revenue int64
		if err := database.QueryRow(`SELECT COUNT(*) FROM tenants WHERE status = 'active'`).Scan(&activeTenants); err != nil {
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		since := time.Now().Add(-24 * time.Hour).Unix()
		if err := database.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM usage_records WHERE created_at >= ?`, since).Scan(&requests24h, &tokens24h); err != nil {
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if err := database.QueryRow(`SELECT COALESCE(SUM(t.monthly_price_cents), 0) FROM tenants tn JOIN tiers t ON t.id = tn.tier_id WHERE tn.status = 'active'`).Scan(&revenue); err != nil && err != sql.ErrNoRows {
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]int64{
			"active_tenants": activeTenants, "total_requests_24h": requests24h,
			"tokens_used_24h": tokens24h, "revenue_estimate_cents": revenue,
		})
	}
}
