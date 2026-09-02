// Package proxy provides load-balanced account selection for Switchblade.
package proxy

import (
	"database/sql"
	"errors"
	"math/rand"
	"sync"
	"time"

	"switchblade/internal/db"
	"switchblade/internal/providers"
)

// ErrNoAccounts is returned by Pick when no active accounts are available.
var ErrNoAccounts = errors.New("no active accounts available")

const cacheTTL = 3 * time.Second

// SharedTenant owns the accounts every tenant is allowed to draw on.
// accounts.tenant_id defaults to it, so an install that predates multi-tenancy
// keeps a single pool that all traffic shares.
const SharedTenant = "_system"

// inFlightTracker counts open leases per account id.
//
// It is shared by every pool a manager hands out: the shared '_system' rows
// appear in each tenant's pool, so one account is often held by several pools at
// once and a per-pool counter would report it idle while another tenant is still
// streaming from it.
type inFlightTracker struct {
	mu     sync.Mutex
	counts map[int64]int
}

func newInFlightTracker() *inFlightTracker {
	return &inFlightTracker{counts: make(map[int64]int)}
}

func (t *inFlightTracker) acquire(id int64) {
	t.mu.Lock()
	t.counts[id]++
	t.mu.Unlock()
}

func (t *inFlightTracker) release(id int64) {
	t.mu.Lock()
	if t.counts[id] > 0 {
		t.counts[id]--
	}
	if t.counts[id] == 0 {
		delete(t.counts, id)
	}
	t.mu.Unlock()
}

func (t *inFlightTracker) count(id int64) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counts[id]
}

type poolKey struct{ tenant, provider string }

// PoolSource resolves the pool serving provider on behalf of tenant.
// *PoolManager is the production implementation; the indirection keeps the
// router and fallback executor constructible without a database in tests.
type PoolSource interface {
	For(tenant, provider string) *AccountPool
}

// PoolManager hands out account pools keyed by (tenant, provider).
//
// Pools are created on demand and are never shared between tenants. Each one
// caches its own tenant's accounts plus the shared pool and nothing else, so a
// request cannot be served with another tenant's credentials — which would spend
// their quota, land in their last_used_at, and risk their account being banned
// upstream for traffic they never sent.
type PoolManager struct {
	mu       sync.Mutex
	db       *db.DB
	strategy string
	inFlight *inFlightTracker
	pools    map[poolKey]*AccountPool
}

// NewPoolManager creates a manager whose pools all use the given LB strategy.
func NewPoolManager(database *db.DB, strategy string) *PoolManager {
	return &PoolManager{
		db:       database,
		strategy: strategy,
		inFlight: newInFlightTracker(),
		pools:    make(map[poolKey]*AccountPool),
	}
}

// For returns the pool serving provider on behalf of tenant.
//
// An empty tenant — an unauthenticated route, or a legacy global-key request
// that carries no identity — resolves to the shared pool, which is the only
// inventory such a caller is entitled to.
func (m *PoolManager) For(tenant, provider string) *AccountPool {
	if tenant == "" {
		tenant = SharedTenant
	}
	k := poolKey{tenant: tenant, provider: provider}

	m.mu.Lock()
	defer m.mu.Unlock()

	if p, ok := m.pools[k]; ok {
		return p
	}
	p := &AccountPool{
		db:       m.db,
		strategy: m.strategy,
		tenant:   tenant,
		inFlight: m.inFlight,
	}
	m.pools[k] = p
	return p
}

// Shared returns the pool of accounts common to every tenant.
func (m *PoolManager) Shared(provider string) *AccountPool {
	return m.For(SharedTenant, provider)
}

// AccountPool holds a cached slice of active accounts for one (tenant, provider)
// pair and dispatches Pick() calls using the configured load-balancing strategy.
type AccountPool struct {
	mu       sync.Mutex
	db       *db.DB
	strategy string // round_robin | least_connections | random | weighted_quota
	tenant   string // immutable after construction

	accounts []*providers.Account
	provider string // provider the cache was filled for
	cachedAt time.Time
	rrIndex  int // round_robin cursor
	inFlight *inFlightTracker
}

// NewAccountPool creates a pool over the shared accounts with the given LB
// strategy. Tenant-scoped pools come from a PoolManager.
func NewAccountPool(database *db.DB, strategy string) *AccountPool {
	return &AccountPool{
		db:       database,
		strategy: strategy,
		tenant:   SharedTenant,
		inFlight: newInFlightTracker(),
	}
}

// Tenant reports which tenant this pool serves.
func (p *AccountPool) Tenant() string { return p.tenant }

// Reload fetches active accounts for provider from DB into the in-memory cache.
// The pool's own tenant and the shared pool are the only rows the query can see.
func (p *AccountPool) Reload(provider string) error {
	const q = `SELECT id, provider, email, status, enabled, tokens, quota_limit, quota_remaining, last_used_at, tier
	           FROM accounts
	           WHERE provider = ? AND tenant_id IN (?, ?) AND status = 'active' AND enabled = 1`

	rows, err := p.db.Query(q, provider, p.tenant, SharedTenant)
	if err != nil {
		return err
	}
	defer rows.Close()

	var accounts []*providers.Account
	for rows.Next() {
		a := &providers.Account{}
		var enabled int
		var lastUsedAt sql.NullInt64
		var tokens sql.NullString
		if err := rows.Scan(
			&a.ID, &a.Provider, &a.Email, &a.Status,
			&enabled, &tokens, &a.QuotaLimit, &a.QuotaRemaining, &lastUsedAt,
			&a.Tier,
		); err != nil {
			return err
		}
		a.Enabled = enabled == 1
		a.Tokens = tokens.String
		if lastUsedAt.Valid {
			a.LastUsedAt = lastUsedAt.Int64
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	p.mu.Lock()
	p.accounts = accounts
	p.provider = provider
	p.cachedAt = time.Now()
	p.mu.Unlock()
	return nil
}

// stale reports whether the cache must be refilled before the next pick. The
// provider is part of the cache identity, so a pool asked for a provider other
// than the one it last loaded must not answer from the rows it is holding.
func (p *AccountPool) stale(provider string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.provider != provider || time.Since(p.cachedAt) > cacheTTL
}

// Pick selects an account using the configured LB strategy.
// Reloads from DB if the cache is stale (> 3s).
func (p *AccountPool) Pick(provider string) (*providers.Account, error) {
	if p.stale(provider) {
		if err := p.Reload(provider); err != nil {
			return nil, err
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.accounts) == 0 {
		return nil, ErrNoAccounts
	}
	return p.pick(), nil
}

// ErrTierExhausted is returned when no accounts remain in the requested tier or lower.
var ErrTierExhausted = errors.New("all tiers exhausted")

// PickWithTier selects an account from the preferred tier first, falling back to lower tiers.
// preferredTier must be one of: "subscription", "cheap", "free".
// Returns ErrTierExhausted if no accounts are available in any tier.
func (p *AccountPool) PickWithTier(provider string, preferredTier string) (*providers.Account, string, error) {
	if err := providers.ValidateTier(preferredTier); err != nil {
		return nil, "", err
	}

	if p.stale(provider) {
		if err := p.Reload(provider); err != nil {
			return nil, "", err
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.accounts) == 0 {
		return nil, "", ErrNoAccounts
	}

	// Tier priority order: subscription > cheap > free
	tierOrder := []string{
		providers.TierSubscription,
		providers.TierCheap,
		providers.TierFree,
	}

	// Find starting index for preferred tier
	startIdx := 0
	for i, t := range tierOrder {
		if t == preferredTier {
			startIdx = i
			break
		}
	}

	// Try each tier from preferred downward
	for i := startIdx; i < len(tierOrder); i++ {
		tier := tierOrder[i]
		if acc := p.pickFromTier(tier); acc != nil {
			return acc, tier, nil
		}
	}

	return nil, "", ErrTierExhausted
}

// pickFromTier selects an account from the specified tier using the configured LB strategy.
// Must be called with p.mu held.
// Skips accounts with quota_remaining <= 0 (exhausted).
func (p *AccountPool) pickFromTier(tier string) *providers.Account {
	var tierAccounts []*providers.Account
	for _, a := range p.accounts {
		if a.Tier == tier && a.QuotaRemaining > 0 {
			tierAccounts = append(tierAccounts, a)
		}
	}
	if len(tierAccounts) == 0 {
		return nil
	}

	// Save original accounts and swap with tier-filtered list
	origAccounts := p.accounts
	p.accounts = tierAccounts
	defer func() { p.accounts = origAccounts }()

	return p.pick()
}

// pick applies the configured strategy to whatever p.accounts currently holds.
// Must be called with p.mu held and a non-empty account list.
func (p *AccountPool) pick() *providers.Account {
	switch p.strategy {
	case "least_connections":
		return p.pickLeastConnections()
	case "random":
		return p.pickRandom()
	case "weighted_quota":
		return p.pickWeightedQuota()
	default: // round_robin
		return p.pickRoundRobin()
	}
}

func (p *AccountPool) pickRoundRobin() *providers.Account {
	acc := p.accounts[p.rrIndex%len(p.accounts)]
	p.rrIndex++
	return acc
}

func (p *AccountPool) pickLeastConnections() *providers.Account {
	best := p.accounts[0]
	bestCount := p.inFlight.count(best.ID)
	for _, a := range p.accounts[1:] {
		if c := p.inFlight.count(a.ID); c < bestCount {
			best = a
			bestCount = c
		}
	}
	return best
}

func (p *AccountPool) pickRandom() *providers.Account {
	return p.accounts[rand.Intn(len(p.accounts))]
}

func (p *AccountPool) pickWeightedQuota() *providers.Account {
	var total float64
	for _, a := range p.accounts {
		if a.QuotaRemaining > 0 {
			total += a.QuotaRemaining
		}
	}
	if total <= 0 {
		// ponytail: fall back to round_robin when all quotas are zero
		return p.pickRoundRobin()
	}
	r := rand.Float64() * total
	var cumulative float64
	for _, a := range p.accounts {
		if a.QuotaRemaining > 0 {
			cumulative += a.QuotaRemaining
			if r <= cumulative {
				return a
			}
		}
	}
	return p.accounts[len(p.accounts)-1]
}

// MarkUsed increments the in-flight counter and updates last_used_at for account id.
func (p *AccountPool) MarkUsed(id int64) {
	p.inFlight.acquire(id)

	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now().Unix()
	for _, a := range p.accounts {
		if a.ID == id {
			a.LastUsedAt = now
			break
		}
	}
}

// MarkDone decrements the in-flight counter for account id.
// If err is non-nil, the account is evicted and the cache is staled so the
// next Pick triggers a fresh Reload.
func (p *AccountPool) MarkDone(id int64, err error) {
	p.inFlight.release(id)
	if err == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for i, a := range p.accounts {
		if a.ID == id {
			p.accounts = append(p.accounts[:i], p.accounts[i+1:]...)
			break
		}
	}
	p.cachedAt = time.Time{} // stale → next Pick reloads
}
