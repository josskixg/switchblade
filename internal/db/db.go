// Package db provides SQLite database access for Switchblade.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema_v2.sql
var schemaSQL string

// DB wraps a sql.DB with migration support.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
// Parent directories are created if they don't exist.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	// modernc.org/sqlite only understands `_pragma=` DSN params — the
	// mattn/go-sqlite3 style (`_journal_mode=WAL`, `_foreign_keys=on`) is
	// silently ignored, which left WAL, busy_timeout, and FK enforcement off.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	raw.SetMaxOpenConns(1) // SQLite = single writer

	if err := raw.Ping(); err != nil {
		raw.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &DB{DB: raw}, nil
}

// Migrate runs the embedded schema_v2.sql (idempotent CREATE IF NOT EXISTS).
func (db *DB) Migrate() error {
	_, err := db.Exec(schemaSQL)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	slog.Info("[DB] schema applied")
	return nil
}
