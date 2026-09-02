package billing

import (
	"database/sql"
	"testing"
	"time"

	"switchblade/internal/db"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	raw, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { raw.Close() })
	if err := raw.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.ApplyVersionedMigrations(raw.DB); err != nil {
		t.Fatalf("versioned migrate: %v", err)
	}
	if _, err := raw.DB.Exec(
		`INSERT OR IGNORE INTO tenants (id, name, email, status) VALUES ('t1', 'Test', 't1@example.com', 'active')`,
	); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return raw.DB
}

func balance(t *testing.T, database *sql.DB, tenant string) Nano {
	t.Helper()
	b, err := Balance(database, tenant)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return b
}

// TestPriceCost pins the arithmetic that turns tokens into money. Getting the
// scale wrong here is exactly what made cost_cents useless.
func TestPriceCost(t *testing.T) {
	// $0.15 per 1M input tokens, $0.60 per 1M output, no margin.
	p := Price{InputNanoPerMTok: 150_000_000, OutputNanoPerMTok: 600_000_000}

	got := p.Cost(Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000})
	want := Nano(750_000_000) // $0.75
	if got != want {
		t.Errorf("cost = %d (%.4f USD), want %d", got, got.USD(), want)
	}

	// A small request must not round to zero — the whole point of nanodollars.
	small := p.Cost(Usage{PromptTokens: 1000})
	if small == 0 {
		t.Fatal("1000-token prompt priced at zero — sub-cent precision lost")
	}
	if small != 150_000 {
		t.Errorf("small cost = %d, want 150000", small)
	}
}

func TestPriceCost_MarginApplied(t *testing.T) {
	p := Price{InputNanoPerMTok: 1_000_000_000, MarginBPS: 2000} // +20%
	got := p.Cost(Usage{PromptTokens: 1_000_000})
	if want := Nano(1_200_000_000); got != want {
		t.Errorf("cost with 20%% margin = %d, want %d", got, want)
	}
}

// TestPricingLookup_PrefixFallback covers dated model snapshots billing at the
// base model's rate instead of falling through unpriced.
func TestPricingLookup_PrefixFallback(t *testing.T) {
	database := testDB(t)
	cache := NewPricingCache(database)

	p, ok := cache.Lookup("gpt-4o-mini-2024-07-18")
	if !ok {
		t.Fatal("versioned model did not match any rate card")
	}
	if p.Model != "gpt-4o-mini" {
		t.Errorf("matched %q, want gpt-4o-mini (longest prefix, not gpt-4o)", p.Model)
	}

	if _, ok := cache.Lookup("some-unknown-model"); ok {
		t.Error("unknown model unexpectedly matched a rate card")
	}
}

func TestMeterPersist_ChargesAgainstBalance(t *testing.T) {
	database := testDB(t)
	m := NewMeter(database, 16)

	if _, err := TopUp(database, "t1", "topup", USD(10), "initial"); err != nil {
		t.Fatalf("topup: %v", err)
	}

	// 1M in + 1M out on gpt-4o-mini = $0.75 list, +20% margin = $0.90.
	err := m.persist(Event{
		TenantID: "t1", Model: "gpt-4o-mini", Provider: "openai", Status: "success",
		RequestID: "req_test", Usage: Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000},
	})
	if err != nil {
		t.Fatalf("persist: %v", err)
	}

	if got, want := balance(t, database, "t1"), USD(10)-USD(0.90); got != want {
		t.Errorf("balance = %d (%.4f USD), want %d", got, got.USD(), want)
	}

	var rows int
	database.QueryRow(`SELECT COUNT(*) FROM usage_records WHERE tenant_id = 't1'`).Scan(&rows)
	if rows != 1 {
		t.Errorf("usage_records rows = %d, want 1", rows)
	}
	database.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE tenant_id = 't1'`).Scan(&rows)
	if rows != 1 {
		t.Errorf("request_logs rows = %d, want 1", rows)
	}

	var costNano int64
	database.QueryRow(`SELECT cost_nano FROM usage_records WHERE tenant_id = 't1'`).Scan(&costNano)
	if costNano != int64(USD(0.90)) {
		t.Errorf("cost_nano = %d, want %d", costNano, int64(USD(0.90)))
	}
}

func TestMeterPersist_FailedAndCachedAreNotCharged(t *testing.T) {
	database := testDB(t)
	m := NewMeter(database, 16)
	if _, err := TopUp(database, "t1", "topup", USD(5), ""); err != nil {
		t.Fatalf("topup: %v", err)
	}
	before := balance(t, database, "t1")

	usage := Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000}
	if err := m.persist(Event{TenantID: "t1", Model: "gpt-4o-mini", Status: "error", Usage: usage}); err != nil {
		t.Fatalf("persist error event: %v", err)
	}
	if err := m.persist(Event{TenantID: "t1", Model: "gpt-4o-mini", Status: "success", Cached: true, Usage: usage}); err != nil {
		t.Fatalf("persist cached event: %v", err)
	}

	if after := balance(t, database, "t1"); after != before {
		t.Errorf("balance moved from %d to %d — failed/cached requests must be free", before, after)
	}
	// Both are still recorded for reporting.
	var rows int
	database.QueryRow(`SELECT COUNT(*) FROM usage_records WHERE tenant_id = 't1'`).Scan(&rows)
	if rows != 2 {
		t.Errorf("usage_records rows = %d, want 2", rows)
	}
}

// TestMeterPersist_EstimatedFlagStored guards the audit split between real
// metering and byte-count guesses: a record billed from an estimate must be
// queryable as such, or an operator can never reconcile invoices against
// provider bills.
func TestMeterPersist_EstimatedFlagStored(t *testing.T) {
	database := testDB(t)
	m := NewMeter(database, 16)

	if err := m.persist(Event{
		TenantID: "t1", Model: "gpt-4o-mini", Provider: "openai", Status: "success",
		RequestID: "req_est",
		Usage:     Usage{PromptTokens: 100, CompletionTokens: 200, TotalTokens: 300, Estimated: true},
	}); err != nil {
		t.Fatalf("persist estimated event: %v", err)
	}
	if err := m.persist(Event{
		TenantID: "t1", Model: "gpt-4o-mini", Provider: "openai", Status: "success",
		RequestID: "req_real",
		Usage:     Usage{PromptTokens: 100, CompletionTokens: 200, TotalTokens: 300},
	}); err != nil {
		t.Fatalf("persist metered event: %v", err)
	}

	var estimated int
	if err := database.QueryRow(
		`SELECT estimated FROM usage_records WHERE request_id = 'req_est'`,
	).Scan(&estimated); err != nil {
		t.Fatalf("read estimated flag: %v", err)
	}
	if estimated != 1 {
		t.Error("estimated usage stored as real metering")
	}
	if err := database.QueryRow(
		`SELECT estimated FROM usage_records WHERE request_id = 'req_real'`,
	).Scan(&estimated); err != nil {
		t.Fatalf("read estimated flag: %v", err)
	}
	if estimated != 0 {
		t.Error("provider-reported usage marked as an estimate")
	}
}

// TestCharge_AllowanceThenOverage is the core of the hybrid plan: the included
// allowance is spent first, and only the remainder touches prepaid credit.
func TestCharge_AllowanceThenOverage(t *testing.T) {
	database := testDB(t)
	m := NewMeter(database, 16)

	now := time.Now().Unix()
	_, err := database.Exec(`
		INSERT INTO tenant_subscriptions
			(tenant_id, tier_id, status, period_start, period_end, included_nano, used_nano, overage_enabled, created_at, updated_at)
		VALUES ('t1', 'tier_pro', 'active', ?, ?, ?, 0, 1, ?, ?)`,
		now, now+86400, int64(USD(0.50)), now, now)
	if err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := TopUp(database, "t1", "topup", USD(10), ""); err != nil {
		t.Fatalf("topup: %v", err)
	}

	// $0.90 of usage against a $0.50 allowance → $0.40 of overage.
	if err := m.persist(Event{
		TenantID: "t1", Model: "gpt-4o-mini", Status: "success",
		Usage: Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000},
	}); err != nil {
		t.Fatalf("persist: %v", err)
	}

	var used int64
	database.QueryRow(`SELECT used_nano FROM tenant_subscriptions WHERE tenant_id = 't1'`).Scan(&used)
	if used != int64(USD(0.50)) {
		t.Errorf("subscription used = %d, want %d (allowance fully consumed)", used, int64(USD(0.50)))
	}

	if got, want := balance(t, database, "t1"), USD(10)-USD(0.40); got != want {
		t.Errorf("balance = %.4f USD, want %.4f — overage should be $0.40", got.USD(), want.USD())
	}

	// Both legs must appear in the ledger so an invoice can be rebuilt.
	var subLegs, usageLegs int
	database.QueryRow(`SELECT COUNT(*) FROM credit_ledger WHERE tenant_id='t1' AND kind='subscription'`).Scan(&subLegs)
	database.QueryRow(`SELECT COUNT(*) FROM credit_ledger WHERE tenant_id='t1' AND kind='usage'`).Scan(&usageLegs)
	if subLegs != 1 || usageLegs != 1 {
		t.Errorf("ledger legs: subscription=%d usage=%d, want 1 and 1", subLegs, usageLegs)
	}
}

func TestAdmit(t *testing.T) {
	database := testDB(t)
	m := NewMeter(database, 16)
	now := time.Now().Unix()

	t.Run("no tenant is unmetered", func(t *testing.T) {
		if v := m.Admit(""); !v.Allow || v.Code != "unmetered" {
			t.Errorf("got %+v, want allow/unmetered", v)
		}
	})

	t.Run("tenant without billing config is unmetered", func(t *testing.T) {
		// Existing single-tenant deployments predate billing; they must not
		// start failing with 402 after an upgrade.
		if v := m.Admit("t1"); !v.Allow || v.Code != "unmetered" {
			t.Errorf("got %+v, want allow/unmetered", v)
		}
	})

	t.Run("unknown tenant is rejected", func(t *testing.T) {
		if v := m.Admit("nope"); v.Allow {
			t.Errorf("got %+v, want deny", v)
		}
	})

	t.Run("allowance remaining", func(t *testing.T) {
		database.Exec(`
			INSERT INTO tenant_subscriptions
				(tenant_id, tier_id, status, period_start, period_end, included_nano, used_nano, overage_enabled, created_at, updated_at)
			VALUES ('t1','tier_pro','active',?,?,?,0,1,?,?)`,
			now, now+86400, int64(USD(1)), now, now)
		if v := m.Admit("t1"); !v.Allow || v.Code != "ok_allowance" {
			t.Errorf("got %+v, want allow/ok_allowance", v)
		}
	})

	t.Run("allowance spent falls through to credit", func(t *testing.T) {
		database.Exec(`UPDATE tenant_subscriptions SET used_nano = included_nano WHERE tenant_id = 't1'`)
		if v := m.Admit("t1"); v.Allow {
			t.Errorf("got %+v, want deny — allowance spent and no credit", v)
		}
		if _, err := TopUp(database, "t1", "topup", USD(5), ""); err != nil {
			t.Fatalf("topup: %v", err)
		}
		if v := m.Admit("t1"); !v.Allow || v.Code != "ok_credit" {
			t.Errorf("got %+v, want allow/ok_credit", v)
		}
	})

	t.Run("overage disabled blocks even with credit", func(t *testing.T) {
		database.Exec(`UPDATE tenant_subscriptions SET overage_enabled = 0 WHERE tenant_id = 't1'`)
		v := m.Admit("t1")
		if v.Allow || v.Code != "allowance_exhausted" {
			t.Errorf("got %+v, want deny/allowance_exhausted", v)
		}
	})

	t.Run("inactive tenant is rejected", func(t *testing.T) {
		database.Exec(`UPDATE tenants SET status = 'suspended' WHERE id = 't1'`)
		v := m.Admit("t1")
		if v.Allow || v.Code != "tenant_inactive" {
			t.Errorf("got %+v, want deny/tenant_inactive", v)
		}
	})
}

// TestPersist_UnmeteredTenantStaysUnmetered guards the promise Admit documents:
// a deployment that predates billing must not start 402-ing after an upgrade.
//
// The regression this catches: charge() used to INSERT a tenant_balance row
// unconditionally, so a tenant with neither a plan nor credit went overdrawn on
// its very first billable request. Admit then saw a balance row at or below zero
// and returned no_credit — a permanent 402 on every request thereafter. The
// original unmetered test only called Admit on a virgin tenant, the one state
// where the bug cannot show.
func TestPersist_UnmeteredTenantStaysUnmetered(t *testing.T) {
	database := testDB(t)
	m := NewMeter(database, 16)

	if v := m.Admit("t1"); !v.Allow || v.Code != "unmetered" {
		t.Fatalf("before any traffic: %+v, want allow/unmetered", v)
	}

	for i := 0; i < 3; i++ {
		if err := m.persist(Event{
			TenantID: "t1", Model: "gpt-4o-mini", Provider: "openai", Status: "success",
			Usage: Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000},
		}); err != nil {
			t.Fatalf("persist %d: %v", i, err)
		}
		if v := m.Admit("t1"); !v.Allow {
			t.Fatalf("after %d billable request(s): %+v — unmetered tenant got blocked", i+1, v)
		}
	}

	// No plan and no credit means nothing to draw on, so no balance row should
	// have been conjured and no ledger entry written.
	var balanceRows, ledgerRows int
	database.QueryRow(`SELECT COUNT(*) FROM tenant_balance WHERE tenant_id = 't1'`).Scan(&balanceRows)
	database.QueryRow(`SELECT COUNT(*) FROM credit_ledger WHERE tenant_id = 't1'`).Scan(&ledgerRows)
	if balanceRows != 0 {
		t.Errorf("tenant_balance rows = %d, want 0 — an unmetered tenant must not be given a balance", balanceRows)
	}
	if ledgerRows != 0 {
		t.Errorf("credit_ledger rows = %d, want 0", ledgerRows)
	}

	// Usage is still recorded, with its would-be cost, so the operator can see
	// what the traffic is worth before putting the tenant on a plan.
	var records int
	var costNano int64
	database.QueryRow(`SELECT COUNT(*), COALESCE(SUM(cost_nano), 0) FROM usage_records WHERE tenant_id = 't1'`).Scan(&records, &costNano)
	if records != 3 {
		t.Errorf("usage_records = %d, want 3", records)
	}
	if costNano != int64(USD(0.90))*3 {
		t.Errorf("recorded cost = %d, want %d", costNano, int64(USD(0.90))*3)
	}
}

// TestPersist_SubscriberOverageStillGoesNegative pins the opposite case: a
// tenant that IS under metering must still be debited past zero, so the overdraft
// is visible and Admit blocks the next request.
func TestPersist_SubscriberOverageStillGoesNegative(t *testing.T) {
	database := testDB(t)
	m := NewMeter(database, 16)

	now := time.Now().Unix()
	if _, err := database.Exec(`
		INSERT INTO tenant_subscriptions
			(tenant_id, tier_id, status, period_start, period_end, included_nano, used_nano, overage_enabled, created_at, updated_at)
		VALUES ('t1','tier_pro','active',?,?,?,0,1,?,?)`,
		now, now+86400, int64(USD(0.10)), now, now); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	if err := m.persist(Event{
		TenantID: "t1", Model: "gpt-4o-mini", Status: "success",
		Usage: Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000},
	}); err != nil {
		t.Fatalf("persist: %v", err)
	}

	// $0.90 of usage against a $0.10 allowance and no credit → -$0.80.
	if got, want := balance(t, database, "t1"), -USD(0.80); got != want {
		t.Errorf("balance = %.4f USD, want %.4f", got.USD(), want.USD())
	}
	if v := m.Admit("t1"); v.Allow {
		t.Errorf("overdrawn subscriber was admitted: %+v", v)
	}
}

func TestTopUp_RecordsRunningBalance(t *testing.T) {
	database := testDB(t)

	if _, err := TopUp(database, "t1", "topup", USD(3), "first"); err != nil {
		t.Fatalf("topup: %v", err)
	}
	after, err := TopUp(database, "t1", "topup", USD(2), "second")
	if err != nil {
		t.Fatalf("topup: %v", err)
	}
	if after != USD(5) {
		t.Errorf("balance = %.2f, want 5.00", after.USD())
	}

	var lastAfter int64
	database.QueryRow(
		`SELECT balance_after_nano FROM credit_ledger WHERE tenant_id='t1' ORDER BY id DESC LIMIT 1`,
	).Scan(&lastAfter)
	if lastAfter != int64(USD(5)) {
		t.Errorf("ledger balance_after = %d, want %d", lastAfter, int64(USD(5)))
	}
}
