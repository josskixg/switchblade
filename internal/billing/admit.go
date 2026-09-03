package billing

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Verdict is the outcome of an admission check, evaluated before a request is
// proxied upstream.
type Verdict struct {
	Allow  bool
	Code   string // ok_allowance|ok_credit|unmetered|tenant_inactive|allowance_exhausted|no_credit
	Reason string
}

// Admit decides whether a tenant may make another billable request.
//
// Order matters: the subscription's included allowance is spent first, and only
// once it is exhausted does prepaid credit apply. A tenant with neither a
// subscription nor a balance row is treated as unmetered — existing
// single-tenant deployments predate billing and must not start 402-ing after an
// upgrade. Configure a plan or a balance to bring a tenant under metering.
func (m *Meter) Admit(tenantID string) Verdict {
	if tenantID == "" {
		// Legacy global API_KEY path — no tenant to bill.
		return Verdict{Allow: true, Code: "unmetered"}
	}

	var (
		tenantStatus              string
		included, used, periodEnd int64
		overage                   int
		balance                   int64
		hasSub, hasBalance        int
		subStatus                 string
	)
	err := m.db.QueryRow(`
		SELECT t.status,
		       COALESCE(s.included_nano, 0), COALESCE(s.used_nano, 0),
		       COALESCE(s.overage_enabled, 1), COALESCE(s.period_end, 0),
		       COALESCE(s.status, ''), COALESCE(b.balance_nano, 0),
		       CASE WHEN s.tenant_id IS NULL THEN 0 ELSE 1 END,
		       CASE WHEN b.tenant_id IS NULL THEN 0 ELSE 1 END
		FROM tenants t
		LEFT JOIN tenant_subscriptions s ON s.tenant_id = t.id
		LEFT JOIN tenant_balance       b ON b.tenant_id = t.id
		WHERE t.id = ?`, tenantID,
	).Scan(&tenantStatus, &included, &used, &overage, &periodEnd,
		&subStatus, &balance, &hasSub, &hasBalance)

	if err == sql.ErrNoRows {
		return Verdict{Allow: false, Code: "tenant_inactive", Reason: "unknown tenant"}
	}
	if err != nil {
		// Fail open on infrastructure trouble: dropping paid traffic because the
		// billing DB hiccuped costs more than the odd unbilled request. The error
		// is logged so it cannot pass unnoticed.
		slog.Info(fmt.Sprintf("[billing] admit %s: %v", tenantID, err))
		return Verdict{Allow: true, Code: "unmetered", Reason: "billing lookup failed"}
	}

	if tenantStatus != "active" {
		return Verdict{Allow: false, Code: "tenant_inactive", Reason: "tenant is not active"}
	}

	if hasSub == 0 && hasBalance == 0 {
		return Verdict{Allow: true, Code: "unmetered"}
	}

	now := time.Now().Unix()
	if hasSub == 1 && subStatus == "active" && now < periodEnd && used < included {
		return Verdict{Allow: true, Code: "ok_allowance"}
	}

	overageAllowed := hasSub == 0 || overage == 1
	if overageAllowed && balance > 0 {
		return Verdict{Allow: true, Code: "ok_credit"}
	}

	if hasSub == 1 && !overageAllowed {
		return Verdict{Allow: false, Code: "allowance_exhausted",
			Reason: "plan allowance exhausted and overage is disabled"}
	}
	return Verdict{Allow: false, Code: "no_credit",
		Reason: "plan allowance exhausted and credit balance is empty"}
}

// Balance returns a tenant's prepaid credit.
func Balance(db *sql.DB, tenantID string) (Nano, error) {
	var b int64
	err := db.QueryRow(`SELECT balance_nano FROM tenant_balance WHERE tenant_id = ?`, tenantID).Scan(&b)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return Nano(b), err
}

// TopUp credits a tenant's balance and records it in the ledger. kind is
// normally "topup"; use "adjustment" or "refund" for manual corrections.
func TopUp(db *sql.DB, tenantID, kind string, amount Nano, note string) (Nano, error) {
	if amount == 0 {
		return Balance(db, tenantID)
	}
	now := time.Now().Unix()

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO tenant_balance (tenant_id, balance_nano, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(tenant_id) DO UPDATE SET balance_nano = balance_nano + ?, updated_at = ?`,
		tenantID, int64(amount), now, int64(amount), now,
	); err != nil {
		return 0, err
	}

	after := balanceOf(tx, tenantID)
	if err := appendLedger(tx, tenantID, kind, amount, after, "", "", note, now); err != nil {
		return 0, err
	}
	return after, tx.Commit()
}
