package api

import (
	"context"
	"log"
	"os"
	"time"

	"switchblade/internal/db"
)

// StartImageCleanup starts a background goroutine that deletes image files and
// DB rows older than retentionDays, running once at startup then every 24h.
func StartImageCleanup(ctx context.Context, database *db.DB, storageDir string, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	go func() {
		runImageCleanup(database, retentionDays)
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("[image/cleanup] stopped")
				return
			case <-t.C:
				runImageCleanup(database, retentionDays)
			}
		}
	}()
}

func runImageCleanup(database *db.DB, retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	rows, err := database.Query(
		`SELECT id, local_path FROM image_studio_results WHERE created_at < ?`, cutoff)
	if err != nil {
		log.Printf("[image/cleanup] query error: %v", err)
		return
	}
	defer rows.Close()

	var ids []int64
	var paths []string
	for rows.Next() {
		var id int64
		var localPath *string
		if err := rows.Scan(&id, &localPath); err != nil {
			continue
		}
		ids = append(ids, id)
		if localPath != nil && *localPath != "" {
			paths = append(paths, *localPath)
		}
	}
	rows.Close()

	cleaned := 0
	for i, id := range ids {
		if i < len(paths) && paths[i] != "" {
			if err := os.Remove(paths[i]); err != nil {
				log.Printf("[cleanup] remove %s: %v", paths[i], err)
			}
		}
		if _, err := database.Exec(`DELETE FROM image_studio_results WHERE id = ?`, id); err != nil {
			log.Printf("[cleanup] db delete: %v", err)
		}
		cleaned++
	}
	if cleaned > 0 {
		log.Printf("[image/cleanup] removed %d image(s) older than %d days", cleaned, retentionDays)
	}
}
