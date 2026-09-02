// Package db — backup, restore, and log rotation.
package db

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Backup writes a gzip-compressed copy of the SQLite file to dir.
// Returns the path of the created backup file.
func Backup(dbPath, dir string, retention int) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("backup mkdir: %w", err)
	}

	// Checkpoint WAL to main DB file before copying so backup is consistent.
	// Opens the DB just for the pragma — caller's connection may not have it.
	checkpointDB, err := Open(dbPath)
	if err == nil {
		_, _ = checkpointDB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		checkpointDB.Close()
	}

	stamp := time.Now().Format("20060102-150405")
	dest := filepath.Join(dir, fmt.Sprintf("switchblade-%s.db.gz", stamp))

	src, err := os.Open(dbPath)
	if err != nil {
		return "", fmt.Errorf("backup open src: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("backup create dst: %w", err)
	}
	defer dst.Close()

	gz := gzip.NewWriter(dst)
	if _, err := io.Copy(gz, src); err != nil {
		return "", fmt.Errorf("backup copy: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("backup gzip close: %w", err)
	}

	if retention > 0 {
		pruneBackups(dir, retention)
	}
	return dest, nil
}

// Restore decompresses a .db.gz backup file or copies a raw .db file onto destPath (overwrites).
func Restore(backupPath, destPath string) error {
	src, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("restore open: %w", err)
	}
	defer src.Close()

	var reader io.Reader = src
	if strings.HasSuffix(strings.ToLower(backupPath), ".gz") {
		gz, err := gzip.NewReader(src)
		if err != nil {
			return fmt.Errorf("restore gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	dst, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("restore create: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, reader); err != nil {
		return fmt.Errorf("restore copy: %w", err)
	}
	return nil
}

// StartAutoBackup runs a background goroutine that calls Backup every intervalMinutes.
func StartAutoBackup(ctx context.Context, dbPath, dir string, intervalMinutes, retention int) {
	if intervalMinutes <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("[backup] auto backup stopped")
				return
			case <-ticker.C:
				if path, err := Backup(dbPath, dir, retention); err != nil {
					log.Printf("[backup] error: %v", err)
				} else {
					log.Printf("[backup] wrote %s", path)
				}
			}
		}
	}()
}

// PruneRequestLogs deletes request_logs older than retentionDays.
func (db *DB) PruneRequestLogs(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	res, err := db.Exec(`DELETE FROM request_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[db] pruned %d request_logs older than %d days", n, retentionDays)
	}
	return nil
}

// PruneUsageSummary deletes usage_summary rows with a bucket timestamp older than retentionDays.
func (db *DB) PruneUsageSummary(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Format("2006-01-02")
	// bucket is stored as "YYYY-MM-DD-HH" text — compare lexicographically via date string
	res, err := db.Exec(`DELETE FROM usage_summary WHERE bucket < ?`, cutoff)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[db] pruned %d usage_summary rows older than %d days", n, retentionDays)
	}
	return nil
}

// pruneBackups deletes the oldest .db.gz files in dir, keeping the N most recent.
func pruneBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".db.gz") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files) // lexicographic = chronological (timestamp in name)
	for len(files) > keep {
		if err := os.Remove(files[0]); err != nil {
			log.Printf("[backup] prune remove %s: %v", files[0], err)
		}
		files = files[1:]
	}
}
