package auth

import (
	"context"
	"log"
	"sync"
	"time"

	"switchblade/internal/db"
	"switchblade/internal/providers"
)

// WarmupQueue health-checks all enabled+active accounts concurrently.
type WarmupQueue struct {
	db          *db.DB
	registry    *providers.Registry
	concurrency int
}

// NewWarmupQueue creates a WarmupQueue.
func NewWarmupQueue(database *db.DB, registry *providers.Registry, concurrency int) *WarmupQueue {
	return &WarmupQueue{
		db:          database,
		registry:    registry,
		concurrency: concurrency,
	}
}

// RunOnce loads all enabled+active accounts, checks each via Provider.Healthy(),
// and marks unhealthy ones as 'error'.
func (w *WarmupQueue) RunOnce(ctx context.Context) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT id, provider, email, password, status, enabled, tokens,
		        quota_limit, quota_remaining, quota_reset_at,
		        last_used_at, last_login_at, error_message, metadata,
		        created_at, updated_at
		   FROM accounts
		  WHERE enabled = 1 AND status = 'active'`,
	)
	if err != nil {
		log.Printf("[auth/warmup] query error: %v", err)
		return
	}
	defer rows.Close()

	var accs []*providers.Account
	for rows.Next() {
		a := &providers.Account{}
		if err := rows.Scan(
			&a.ID, &a.Provider, &a.Email, &a.Password, &a.Status, &a.Enabled,
			&a.Tokens, &a.QuotaLimit, &a.QuotaRemaining, &a.QuotaResetAt,
			&a.LastUsedAt, &a.LastLoginAt, &a.ErrorMessage, &a.Metadata,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			log.Printf("[auth/warmup] scan error: %v", err)
			continue
		}
		accs = append(accs, a)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[auth/warmup] rows error: %v", err)
	}

	log.Printf("[auth/warmup] checking %d active accounts", len(accs))

	// ponytail: semaphore via buffered chan, no extra sync primitive needed
	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup

	for _, acc := range accs {
		acc := acc
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			w.check(ctx, acc)
		}()
	}
	wg.Wait()
}

func (w *WarmupQueue) check(ctx context.Context, acc *providers.Account) {
	// find the right provider by name
	var p providers.Provider
	for _, candidate := range w.registry.All() {
		if candidate.Name() == acc.Provider {
			p = candidate
			break
		}
	}
	if p == nil {
		log.Printf("[auth/warmup] no provider registered for %q (account %d)", acc.Provider, acc.ID)
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if p.Healthy(checkCtx, acc) {
		log.Printf("[auth/warmup] account %d (%s) healthy", acc.ID, acc.Email)
		return
	}

	log.Printf("[auth/warmup] account %d (%s) unhealthy, marking error", acc.ID, acc.Email)
	_, err := w.db.ExecContext(ctx,
		`UPDATE accounts SET status = 'error', updated_at = ? WHERE id = ?`,
		time.Now().Unix(), acc.ID,
	)
	if err != nil {
		log.Printf("[auth/warmup] failed to update account %d: %v", acc.ID, err)
	}
}
