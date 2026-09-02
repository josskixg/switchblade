package db

import (
	"database/sql"
	"fmt"

	"switchblade/internal/providers"
)

// GetAccountByID loads a single account by primary key.
func (db *DB) GetAccountByID(id int64) (*providers.Account, error) {
	row := db.QueryRow(`
		SELECT id, provider, email, password, status, enabled,
		       tokens, quota_limit, quota_remaining, quota_reset_at,
		       last_used_at, last_login_at, error_message, metadata,
		       created_at, updated_at
		FROM accounts WHERE id = ?`, id)
	return scanAccount(row)
}

// UpdateAccountStatus sets status and error_message for the given account.
func (db *DB) UpdateAccountStatus(id int64, status, errMsg string) error {
	_, err := db.Exec(
		`UPDATE accounts SET status = ?, error_message = ?, updated_at = strftime('%s','now') WHERE id = ?`,
		status, errMsg, id,
	)
	return err
}

// UpdateAccountTokens replaces the JSON token blob for the given account.
func (db *DB) UpdateAccountTokens(id int64, tokens string) error {
	_, err := db.Exec(
		`UPDATE accounts SET tokens = ?, updated_at = strftime('%s','now') WHERE id = ?`,
		tokens, id,
	)
	return err
}

// ListActiveAccounts returns enabled=1, status='active' accounts.
// Pass provider="" to return all providers.
func (db *DB) ListActiveAccounts(provider string) ([]*providers.Account, error) {
	query := `
		SELECT id, provider, email, password, status, enabled,
		       tokens, quota_limit, quota_remaining, quota_reset_at,
		       last_used_at, last_login_at, error_message, metadata,
		       created_at, updated_at
		FROM accounts
		WHERE enabled = 1 AND status = 'active'`
	args := []any{}
	if provider != "" {
		query += " AND provider = ?"
		args = append(args, provider)
	}
	query += " ORDER BY id"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListActiveAccounts: %w", err)
	}
	defer rows.Close()

	var out []*providers.Account
	for rows.Next() {
		acc, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, acc)
	}
	return out, rows.Err()
}

// scanner abstracts *sql.Row and *sql.Rows for scanAccount.
type scanner interface {
	Scan(dest ...any) error
}

func scanAccount(s scanner) (*providers.Account, error) {
	var a providers.Account
	var enabled int
	var tokens, errorMsg, metadata sql.NullString
	var quotaResetAt, lastUsedAt, lastLoginAt, updatedAt sql.NullInt64

	err := s.Scan(
		&a.ID, &a.Provider, &a.Email, &a.Password, &a.Status, &enabled,
		&tokens, &a.QuotaLimit, &a.QuotaRemaining, &quotaResetAt,
		&lastUsedAt, &lastLoginAt, &errorMsg, &metadata,
		&a.CreatedAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanAccount: %w", err)
	}
	a.Enabled = enabled == 1
	a.Tokens = tokens.String
	a.ErrorMessage = errorMsg.String
	a.Metadata = metadata.String
	a.QuotaResetAt = quotaResetAt.Int64
	a.LastUsedAt = lastUsedAt.Int64
	a.LastLoginAt = lastLoginAt.Int64
	a.UpdatedAt = updatedAt.Int64
	return &a, nil
}
