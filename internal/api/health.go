package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

var startTime = time.Now()

// HandleHealthCheck returns basic service status and uptime.
func HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime).Round(time.Second)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"uptime": uptime.String(),
	})
}

// HandleReadiness checks database connectivity. Returns 503 if db is down.
func HandleReadiness(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := db.Ping(); err != nil {
			log.Printf("[api] health readiness check failed: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "not_ready",
				"error":  "database unavailable",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	}
}

// HandleLiveness always returns alive — process is running.
func HandleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}
