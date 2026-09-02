// Package storage — local image/video file storage for generation results.
package storage

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"switchblade/internal/ssrf"
)

// ImageStore manages local image storage under a base directory.
type ImageStore struct {
	BaseDir string // e.g. "data/images"
}

// NewImageStore creates a store rooted at baseDir.
func NewImageStore(baseDir string) *ImageStore {
	return &ImageStore{BaseDir: baseDir}
}

// Save downloads a URL and stores it at BaseDir/{chatID}/{imageID}.{ext}.
// Returns the local path on success.
func (s *ImageStore) Save(chatID, imageID, ext, url string) (string, error) {
	dir := filepath.Join(s.BaseDir, chatID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	dest := filepath.Join(dir, imageID+"."+ext)

	if err := ssrf.ValidateURL(url); err != nil {
		return "", fmt.Errorf("ssrf blocked: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("create: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return dest, nil
}

// SaveBytes stores raw bytes at BaseDir/{chatID}/{imageID}.{ext}.
func (s *ImageStore) SaveBytes(chatID, imageID, ext string, data []byte) (string, error) {
	dir := filepath.Join(s.BaseDir, chatID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	dest := filepath.Join(dir, imageID+"."+ext)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return dest, nil
}

// Serve writes a stored file to the given http.ResponseWriter.
func (s *ImageStore) Serve(w http.ResponseWriter, chatID, imageID, ext string) error {
	path := filepath.Join(s.BaseDir, chatID, imageID+"."+ext)
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	switch ext {
	case "png":
		w.Header().Set("Content-Type", "image/png")
	case "jpg", "jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case "webp":
		w.Header().Set("Content-Type", "image/webp")
	case "mp4":
		w.Header().Set("Content-Type", "video/mp4")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	_, err = io.Copy(w, f)
	return err
}

// Delete removes an image file from disk.
func (s *ImageStore) Delete(chatID, imageID, ext string) error {
	path := filepath.Join(s.BaseDir, chatID, imageID+"."+ext)
	return os.Remove(path)
}

// Stats returns total file count and total bytes under BaseDir.
func (s *ImageStore) Stats() (count int, totalBytes int64, err error) {
	err = filepath.Walk(s.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		count++
		totalBytes += info.Size()
		return nil
	})
	return
}

// Cleanup removes files older than retentionDays. 0 = keep forever.
func (s *ImageStore) Cleanup(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	return filepath.Walk(s.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if info.ModTime().Before(cutoff) {
			return os.Remove(path)
		}
		return nil
	})
}
