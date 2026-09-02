package billing

import (
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Usage is the token accounting for a single request.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// Estimated marks counts approximated from byte volume because the
	// upstream ended without reporting usage. The request is still priced and
	// charged, but the flag is persisted so an operator can separate real
	// metering from guesses when reconciling against provider bills.
	Estimated bool
}

// Event is one billable request, handed to the Meter after the response has
// been written to the client. Recording is asynchronous so metering never sits
// in the latency path of a proxied request.
type Event struct {
	TenantID    string
	APIKeyID    int64
	RequestID   string
	Provider    string
	Model       string
	ServiceKind string // chat|embeddings|tts|stt|image
	Usage       Usage
	LatencyMS   int64
	Status      string // success|error
	ErrMessage  string
	// Cached marks a response served from the proxy cache. It is recorded with
	// its token counts for reporting but never charged — no upstream call was
	// made, so there is no cost to pass on.
	Cached        bool
	AccountID     int64
	AccountEmail  string
	FallbackChain []string
}

// Meter prices events, writes the audit trail, and charges the tenant.
type Meter struct {
	db      *sql.DB
	pricing *PricingCache

	events chan Event
	done   chan struct{}

	closeOnce sync.Once
	dropped   atomic.Int64
}

// NewMeter creates a meter with a buffered intake queue. Start must be called
// before Record.
func NewMeter(db *sql.DB, buffer int) *Meter {
	if buffer <= 0 {
		buffer = 1024
	}
	return &Meter{
		db:      db,
		pricing: NewPricingCache(db),
		events:  make(chan Event, buffer),
		done:    make(chan struct{}),
	}
}

// Pricing exposes the rate card cache for admin handlers.
func (m *Meter) Pricing() *PricingCache { return m.pricing }

// Start launches the background writer.
func (m *Meter) Start() { go m.run() }

// Record queues an event. It never blocks: if the queue is saturated the event
// is dropped and counted, because stalling a proxy response to write a billing
// row is the worse failure. Dropped() surfaces the counter for alerting.
func (m *Meter) Record(ev Event) {
	select {
	case m.events <- ev:
	default:
		m.dropped.Add(1)
	}
}

// Dropped returns how many events were discarded due to a full queue.
func (m *Meter) Dropped() int64 { return m.dropped.Load() }

// Close drains the queue and waits for the writer to finish.
func (m *Meter) Close() {
	m.closeOnce.Do(func() {
		close(m.events)
		<-m.done
	})
}

func (m *Meter) run() {
	defer close(m.done)
	for ev := range m.events {
		if err := m.persist(ev); err != nil {
			log.Printf("[billing] persist request %s: %v", ev.RequestID, err)
		}
	}
}

// persist writes the usage record, the audit log, and the charge in one
// transaction so a tenant is never debited without a matching usage row.
func (m *Meter) persist(ev Event) error {
	cost, priced := m.pricing.Cost(ev.Model, ev.Usage)
	if !priced && ev.Usage.TotalTokens > 0 {
		log.Printf("[billing] no rate card for model %q — recorded at zero cost", ev.Model)
	}
	// Only successful, non-cached calls are charged: a failed request should not
	// cost money, and a cache hit never reached a provider.
	if ev.Status != "success" || ev.Cached {
		cost = 0
	}

	now := time.Now().Unix()

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if ev.TenantID != "" {
		if _, err := tx.Exec(`
			INSERT INTO usage_records
				(tenant_id, api_key_id, model, prompt_tokens, completion_tokens, total_tokens,
				 cost_cents, cost_nano, provider, service_kind, status, latency_ms, request_id, estimated, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ev.TenantID, nullableID(ev.APIKeyID), ev.Model,
			ev.Usage.PromptTokens, ev.Usage.CompletionTokens, ev.Usage.TotalTokens,
			int64(cost), ev.Provider, kindOrChat(ev.ServiceKind), ev.Status, ev.LatencyMS,
			ev.RequestID, boolInt(ev.Usage.Estimated), now,
		); err != nil {
			return err
		}
	}

	var chain any
	if len(ev.FallbackChain) > 0 {
		b, _ := json.Marshal(ev.FallbackChain)
		chain = string(b)
	}
	if _, err := tx.Exec(`
		INSERT INTO request_logs
			(tenant_id, account_id, provider, model, service_kind, prompt_tokens, completion_tokens,
			 total_tokens, status, duration_ms, error_message, account_email, fallback_chain, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tenantOrSystem(ev.TenantID), nullableID(ev.AccountID), ev.Provider, ev.Model,
		kindOrChat(ev.ServiceKind), ev.Usage.PromptTokens, ev.Usage.CompletionTokens,
		ev.Usage.TotalTokens, ev.Status, ev.LatencyMS, nullableStr(ev.ErrMessage),
		nullableStr(ev.AccountEmail), chain, now,
	); err != nil {
		return err
	}

	if cost > 0 && ev.TenantID != "" {
		if err := charge(tx, ev.TenantID, cost, ev.Model, ev.RequestID, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// charge applies the hybrid billing model: consume the subscription's included
// allowance first, then bill whatever is left over against prepaid credit. Both
// legs are recorded in credit_ledger so an invoice can be rebuilt from history.
func charge(tx *sql.Tx, tenantID string, cost Nano, model, requestID string, now int64) error {
	remaining := cost

	var included, used, periodEnd int64
	var overage int
	err := tx.QueryRow(`
		SELECT included_nano, used_nano, overage_enabled, period_end
		FROM tenant_subscriptions
		WHERE tenant_id = ? AND status = 'active'`, tenantID,
	).Scan(&included, &used, &overage, &periodEnd)

	hasSub := err == nil
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if hasSub && now < periodEnd {
		if avail := included - used; avail > 0 {
			take := remaining
			if int64(take) > avail {
				take = Nano(avail)
			}
			if _, err := tx.Exec(`
				UPDATE tenant_subscriptions SET used_nano = used_nano + ?, updated_at = ?
				WHERE tenant_id = ?`, int64(take), now, tenantID); err != nil {
				return err
			}
			if err := appendLedger(tx, tenantID, "subscription", -take, balanceOf(tx, tenantID), requestID, model, "included allowance", now); err != nil {
				return err
			}
			remaining -= take
		}
	}

	if remaining <= 0 {
		return nil
	}

	// A tenant with neither a plan nor a balance row is unmetered — see Admit,
	// which uses exactly this test. Creating a balance row for one here would
	// flip it to overdrawn on its first billable request and 402 it from then on,
	// which is precisely the upgrade-day outage Admit promises not to cause.
	// The usage row is still written, so the traffic's worth stays visible.
	var hasBalance int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM tenant_balance WHERE tenant_id = ?`, tenantID,
	).Scan(&hasBalance); err != nil {
		return err
	}
	if !hasSub && hasBalance == 0 {
		return nil
	}

	// Overage — debit prepaid credit. The balance is allowed to go negative so a
	// burst mid-request is still recorded truthfully; Admit stops the next one.
	if _, err := tx.Exec(`
		INSERT INTO tenant_balance (tenant_id, balance_nano, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(tenant_id) DO UPDATE SET balance_nano = balance_nano - ?, updated_at = ?`,
		tenantID, -int64(remaining), now, int64(remaining), now,
	); err != nil {
		return err
	}

	return appendLedger(tx, tenantID, "usage", -remaining, balanceOf(tx, tenantID), requestID, model, "overage", now)
}

func appendLedger(tx *sql.Tx, tenantID, kind string, amount, balanceAfter Nano, requestID, model, note string, now int64) error {
	_, err := tx.Exec(`
		INSERT INTO credit_ledger
			(tenant_id, kind, amount_nano, balance_after_nano, request_id, model, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tenantID, kind, int64(amount), int64(balanceAfter),
		nullableStr(requestID), nullableStr(model), note, now)
	return err
}

func balanceOf(tx *sql.Tx, tenantID string) Nano {
	var b int64
	_ = tx.QueryRow(`SELECT balance_nano FROM tenant_balance WHERE tenant_id = ?`, tenantID).Scan(&b)
	return Nano(b)
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func kindOrChat(k string) string {
	if k == "" {
		return "chat"
	}
	return k
}

func tenantOrSystem(id string) string {
	if id == "" {
		return "_system"
	}
	return id
}
