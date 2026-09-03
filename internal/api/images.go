package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
)

// MountImagesAPI registers image studio endpoints.
func MountImagesAPI(r chi.Router, database *db.DB, storageDir string) {
	r.Get("/api/images", listImages(database))
	r.Get("/api/images/stats", imageStats(database))
	r.Get("/api/images/{id}", serveImage(database))
	r.Delete("/api/images/{id}", deleteImage(database))
}

func listImages(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, prompt, file_type, local_path, file_size, created_at FROM image_studio_results ORDER BY id DESC LIMIT 500`)
		if err != nil {
			slog.Error("[api] list images failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var images []map[string]any
		for rows.Next() {
			var id, createdAt int64
			var fileType string
			var prompt, localPath *string
			var fileSize *int64
			if err := rows.Scan(&id, &prompt, &fileType, &localPath, &fileSize, &createdAt); err != nil {
				continue
			}
			images = append(images, map[string]any{
				"id": id, "provider": "", "prompt": prompt,
				"file_type": fileType, "local_path": localPath,
				"file_size": fileSize, "created_at": createdAt,
			})
		}
		jsonOK(w, images)
	}
}

func imageStats(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row := database.QueryRow(`SELECT COUNT(*), COALESCE(SUM(file_size), 0) FROM image_studio_results`)
		var count, totalSize int64
		if err := row.Scan(&count, &totalSize); err != nil {
			slog.Error("[api] image stats failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]any{"count": count, "total_size_bytes": totalSize})
	}
}

func serveImage(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		row := database.QueryRow(`SELECT local_path, file_type FROM image_studio_results WHERE id = ?`, id)
		var localPath *string
		var fileType string
		if err := row.Scan(&localPath, &fileType); err != nil {
			jsonError(w, http.StatusNotFound, "image not found")
			return
		}
		if localPath == nil || *localPath == "" {
			jsonError(w, http.StatusNotFound, "no file on disk")
			return
		}
		data, err := os.ReadFile(*localPath)
		if err != nil {
			jsonError(w, http.StatusNotFound, "file not readable")
			return
		}
		mime := fileType
		if mime == "" {
			mime = "application/octet-stream"
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			slog.Warn("[images] write response", "err", err)
		}
	}
}

func deleteImage(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		row := database.QueryRow(`SELECT local_path FROM image_studio_results WHERE id = ?`, id)
		var localPath *string
		if err := row.Scan(&localPath); err != nil {
			jsonError(w, http.StatusNotFound, "image not found")
			return
		}
		if _, err := database.Exec(`DELETE FROM image_studio_results WHERE id = ?`, id); err != nil {
			slog.Error("[api] delete image failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if localPath != nil && *localPath != "" {
			if err := os.Remove(*localPath); err != nil {
				slog.Info(fmt.Sprintf("[images] remove %s: %v", *localPath, err))
			}
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}
