package auth

import (
	"context"
	"fmt"
	"log/slog"
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
		slog.Error("[auth/warmup] query error", "err", err)
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
			slog.Error("[auth/warmup] scan error", "err", err)
			continue
		}
		accs = append(accs, a)
	}
	if err := rows.Err(); err != nil {
		slog.Error("[auth/warmup] rows error", "err", err)
	}

	slog.Info(fmt.Sprintf("[auth/warmup] checking %d active accounts", len(accs)))

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
		slog.Info(fmt.Sprintf("[auth/warmup] no provider registered for %q (account %d)", acc.Provider, acc.ID))
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if p.Healthy(checkCtx, acc) {
		slog.Info("[auth/warmup] account healthy", "account_id", acc.ID, "email", acc.Email)
		return
	}

	slog.Error("[auth/warmup] account unhealthy, marking error", "account_id", acc.ID, "email", acc.Email)
	_, err := w.db.ExecContext(ctx,
		`UPDATE accounts SET status = 'error', updated_at = ? WHERE id = ?`,
		time.Now().Unix(), acc.ID,
	)
	if err != nil {
		slog.Error("[auth/warmup] update account failed", "account_id", acc.ID, "err", err)
	}
}
