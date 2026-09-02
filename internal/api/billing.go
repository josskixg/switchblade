package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/billing"
	"switchblade/internal/reqctx"
)

// BillingGate stops a tenant whose plan allowance and prepaid credit are both
// spent. It runs after authentication, which is what puts the tenant id in the
// context, and before the proxy router.
//
// This is the enforcement half of metering. The pre-existing CheckQuota summed
// usage_records, a table nothing ever wrote to, so it returned zero and passed
// unconditionally — every tenant was effectively unlimited.
func BillingGate(meter *billing.Meter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v := meter.Admit(reqctx.Tenant(r.Context()))
			if v.Allow {
				next.ServeHTTP(w, r)
				return
			}

			status := http.StatusPaymentRequired
			if v.Code == "tenant_inactive" {
				status = http.StatusForbidden
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": v.Reason,
				"code":  v.Code,
			})
		})
	}
}

// MountBillingAPI attaches /api/billing/* routes.
func MountBillingAPI(r chi.Router, database *sql.DB) {
	r.Get("/api/billing/account", handleBillingAccount(database))
	r.Get("/api/billing/ledger", handleBillingLedger(database))
	r.Get("/api/billing/pricing", handleListPricing(database))
	r.Put("/api/billing/pricing/{model}", RequireJSONHandler(handleUpsertPricing(database)))
	r.Post("/api/billing/topup", RequireJSONHandler(handleTopUp(database)))
	r.Post("/api/billing/subscription", RequireJSONHandler(handleUpsertSubscription(database)))
}

// handleBillingAccount returns the caller's balance and current plan period —
// everything a customer-facing billing screen needs in one call.
func handleBillingAccount(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := billingTenant(r)
		if tenantID == "" {
			jsonError(w, http.StatusForbidden, "no tenant in context")
			return
		}

		balance, err := billing.Balance(database, tenantID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to read balance")
			return
		}

		out := map[string]any{
			"tenant_id":        tenantID,
			"balance_nano":     int64(balance),
			"balance_usd":      balance.USD(),
			"has_subscription": false,
		}

		var tierID, status string
		var periodStart, periodEnd, included, used int64
		var overage int
		err = database.QueryRow(`
			SELECT tier_id, status, period_start, period_end, included_nano, used_nano, overage_enabled
			FROM tenant_subscriptions WHERE tenant_id = ?`, tenantID,
		).Scan(&tierID, &status, &periodStart, &periodEnd, &included, &used, &overage)
		if err == nil {
			out["has_subscription"] = true
			out["subscription"] = map[string]any{
				"tier_id":         tierID,
				"status":          status,
				"period_start":    periodStart,
				"period_end":      periodEnd,
				"included_nano":   included,
				"included_usd":    billing.Nano(included).USD(),
				"used_nano":       used,
				"used_usd":        billing.Nano(used).USD(),
				"remaining_nano":  included - used,
				"overage_enabled": overage == 1,
			}
		} else if err != sql.ErrNoRows {
			jsonError(w, http.StatusInternalServerError, "failed to read subscription")
			return
		}

		jsonOK(w, out)
	}
}

func handleBillingLedger(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := billingTenant(r)
		if tenantID == "" {
			jsonError(w, http.StatusForbidden, "no tenant in context")
			return
		}

		rows, err := database.Query(`
			SELECT id, kind, amount_nano, balance_after_nano,
			       COALESCE(request_id, ''), COALESCE(model, ''), COALESCE(note, ''), created_at
			FROM credit_ledger WHERE tenant_id = ?
			ORDER BY created_at DESC, id DESC LIMIT 200`, tenantID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to read ledger")
			return
		}
		defer rows.Close()

		entries := make([]map[string]any, 0)
		for rows.Next() {
			var id, amount, after, createdAt int64
			var kind, requestID, model, note string
			if err := rows.Scan(&id, &kind, &amount, &after, &requestID, &model, &note, &createdAt); err != nil {
				continue
			}
			entries = append(entries, map[string]any{
				"id":                 id,
				"kind":               kind,
				"amount_nano":        amount,
				"amount_usd":         billing.Nano(amount).USD(),
				"balance_after_nano": after,
				"balance_after_usd":  billing.Nano(after).USD(),
				"request_id":         requestID,
				"model":              model,
				"note":               note,
				"created_at":         createdAt,
			})
		}
		jsonOK(w, entries)
	}
}

func handleListPricing(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(`
			SELECT model, input_nano_per_mtok, output_nano_per_mtok, margin_bps, enabled, updated_at
			FROM model_pricing ORDER BY model`)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to read pricing")
			return
		}
		defer rows.Close()

		out := make([]map[string]any, 0)
		for rows.Next() {
			var model string
			var in, outNano, margin, updatedAt int64
			var enabled int
			if err := rows.Scan(&model, &in, &outNano, &margin, &enabled, &updatedAt); err != nil {
				continue
			}
			out = append(out, map[string]any{
				"model":                model,
				"input_nano_per_mtok":  in,
				"output_nano_per_mtok": outNano,
				"input_usd_per_mtok":   billing.Nano(in).USD(),
				"output_usd_per_mtok":  billing.Nano(outNano).USD(),
				"margin_bps":           margin,
				"enabled":              enabled == 1,
				"updated_at":           updatedAt,
			})
		}
		jsonOK(w, out)
	}
}

func handleUpsertPricing(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if role := reqctx.RoleOf(r.Context()); role != "owner" && role != "admin" {
			jsonError(w, http.StatusForbidden, "only owners and admins can change pricing")
			return
		}
		model := chi.URLParam(r, "model")
		if model == "" {
			jsonError(w, http.StatusBadRequest, "model is required")
			return
		}

		var body struct {
			InputNanoPerMTok  *int64 `json:"input_nano_per_mtok"`
			OutputNanoPerMTok *int64 `json:"output_nano_per_mtok"`
			MarginBPS         *int64 `json:"margin_bps"`
			Enabled           *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if body.InputNanoPerMTok == nil || body.OutputNanoPerMTok == nil {
			jsonError(w, http.StatusBadRequest, "input_nano_per_mtok and output_nano_per_mtok are required")
			return
		}
		if *body.InputNanoPerMTok < 0 || *body.OutputNanoPerMTok < 0 {
			jsonError(w, http.StatusBadRequest, "prices cannot be negative")
			return
		}

		margin := int64(0)
		if body.MarginBPS != nil {
			margin = *body.MarginBPS
		}
		enabled := 1
		if body.Enabled != nil && !*body.Enabled {
			enabled = 0
		}

		if _, err := database.Exec(`
			INSERT INTO model_pricing (model, input_nano_per_mtok, output_nano_per_mtok, margin_bps, enabled, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(model) DO UPDATE SET
				input_nano_per_mtok = excluded.input_nano_per_mtok,
				output_nano_per_mtok = excluded.output_nano_per_mtok,
				margin_bps = excluded.margin_bps,
				enabled = excluded.enabled,
				updated_at = excluded.updated_at`,
			model, *body.InputNanoPerMTok, *body.OutputNanoPerMTok, margin, enabled, time.Now().Unix(),
		); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to save pricing")
			return
		}

		jsonOK(w, map[string]any{"model": model, "saved": true})
	}
}

func handleTopUp(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Creating credit is an owner action — it is literally minting money.
		if reqctx.RoleOf(r.Context()) != "owner" {
			jsonError(w, http.StatusForbidden, "only owners can top up balances")
			return
		}

		var body struct {
			TenantID   string  `json:"tenant_id"`
			AmountUSD  float64 `json:"amount_usd"`
			AmountNano int64   `json:"amount_nano"`
			Kind       string  `json:"kind"`
			Note       string  `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if body.TenantID == "" {
			jsonError(w, http.StatusBadRequest, "tenant_id is required")
			return
		}

		amount := billing.Nano(body.AmountNano)
		if amount == 0 {
			amount = billing.USD(body.AmountUSD)
		}
		if amount == 0 {
			jsonError(w, http.StatusBadRequest, "amount_usd or amount_nano is required")
			return
		}

		kind := body.Kind
		switch kind {
		case "":
			kind = "topup"
		case "topup", "refund", "adjustment":
		default:
			jsonError(w, http.StatusBadRequest, "kind must be topup, refund, or adjustment")
			return
		}

		after, err := billing.TopUp(database, body.TenantID, kind, amount, body.Note)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to apply top-up")
			return
		}

		jsonOK(w, map[string]any{
			"tenant_id":    body.TenantID,
			"amount_nano":  int64(amount),
			"balance_nano": int64(after),
			"balance_usd":  after.USD(),
		})
	}
}

// handleUpsertSubscription assigns a tenant to a plan for one billing period.
// included_usd is the value of the allowance the plan grants; usage is drawn
// from it before any credit is touched.
func handleUpsertSubscription(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if role := reqctx.RoleOf(r.Context()); role != "owner" && role != "admin" {
			jsonError(w, http.StatusForbidden, "only owners and admins can change subscriptions")
			return
		}

		var body struct {
			TenantID       string  `json:"tenant_id"`
			TierID         string  `json:"tier_id"`
			IncludedUSD    float64 `json:"included_usd"`
			PeriodDays     int     `json:"period_days"`
			OverageEnabled *bool   `json:"overage_enabled"`
			Status         string  `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if body.TenantID == "" || body.TierID == "" {
			jsonError(w, http.StatusBadRequest, "tenant_id and tier_id are required")
			return
		}
		if body.PeriodDays <= 0 {
			body.PeriodDays = 30
		}
		status := body.Status
		if status == "" {
			status = "active"
		}
		overage := 1
		if body.OverageEnabled != nil && !*body.OverageEnabled {
			overage = 0
		}

		now := time.Now().Unix()
		periodEnd := time.Now().AddDate(0, 0, body.PeriodDays).Unix()
		included := int64(billing.USD(body.IncludedUSD))

		// A new period resets used_nano — that is what starts the allowance over.
		if _, err := database.Exec(`
			INSERT INTO tenant_subscriptions
				(tenant_id, tier_id, status, period_start, period_end, included_nano, used_nano,
				 overage_enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
			ON CONFLICT(tenant_id) DO UPDATE SET
				tier_id = excluded.tier_id,
				status = excluded.status,
				period_start = excluded.period_start,
				period_end = excluded.period_end,
				included_nano = excluded.included_nano,
				used_nano = 0,
				overage_enabled = excluded.overage_enabled,
				updated_at = excluded.updated_at`,
			body.TenantID, body.TierID, status, now, periodEnd, included, overage, now, now,
		); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to save subscription")
			return
		}

		jsonOK(w, map[string]any{
			"tenant_id":     body.TenantID,
			"tier_id":       body.TierID,
			"status":        status,
			"period_end":    periodEnd,
			"included_nano": included,
			"included_usd":  body.IncludedUSD,
		})
	}
}

// billingTenant resolves which tenant a billing read applies to. Owners and
// admins may inspect another tenant via ?tenant_id=; everyone else is pinned to
// their own, so a customer cannot read a competitor's spend.
func billingTenant(r *http.Request) string {
	own := reqctx.Tenant(r.Context())
	role := reqctx.RoleOf(r.Context())
	if role == "owner" || role == "admin" {
		if q := r.URL.Query().Get("tenant_id"); q != "" {
			return q
		}
	}
	return own
}
