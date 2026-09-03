package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
	"switchblade/internal/mitm"
)

// MITMAPI holds the MITM bridge API handlers.
type MITMAPI struct {
	proxy *mitm.Proxy
	dns   *mitm.DNSHijacker
	ca    *mitm.CA
	db    *db.DB
}

// NewMITMAPI creates the MITM API handler.
func NewMITMAPI(p *mitm.Proxy, dns *mitm.DNSHijacker, ca *mitm.CA, database *db.DB) *MITMAPI {
	return &MITMAPI{
		proxy: p,
		dns:   dns,
		ca:    ca,
		db:    database,
	}
}

// RegisterRoutes adds MITM routes to the router. Must be called within an auth-protected group.
func (m *MITMAPI) RegisterRoutes(r chi.Router) {
	r.Get("/mitm/status", m.handleStatus)
	r.Post("/mitm/start", m.handleStart)
	r.Post("/mitm/stop", m.handleStop)
	r.Get("/mitm/sessions", m.handleSessions)
	r.Get("/mitm/cert", m.handleCert)
	r.Post("/mitm/dns/install", m.handleDNSInstall)
	r.Post("/mitm/dns/uninstall", m.handleDNSUninstall)
}

func (m *MITMAPI) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"running":    m.proxy.IsRunning(),
		"dns_hijack": m.dns.IsInstalled(),
	})
}

func (m *MITMAPI) handleStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := m.proxy.Start(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (m *MITMAPI) handleStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := m.proxy.Stop(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func (m *MITMAPI) handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rows, err := m.db.Query(
		`SELECT id, ide, provider, account_id, method, path, status_code, bytes_sent, created_at
		 FROM mitm_sessions ORDER BY id DESC LIMIT 100`,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var sessions []map[string]any
	for rows.Next() {
		var id, accountID, statusCode, bytesSent, createdAt sql.NullInt64
		var ide, provider, method, path string
		if err := rows.Scan(&id, &ide, &provider, &accountID, &method, &path, &statusCode, &bytesSent, &createdAt); err != nil {
			slog.Warn("[mitm/api] scan session", "err", err)
			continue
		}
		sessions = append(sessions, map[string]any{
			"id":          id.Int64,
			"ide":         ide,
			"provider":    provider,
			"account_id":  accountID.Int64,
			"method":      method,
			"path":        path,
			"status_code": statusCode.Int64,
			"bytes_sent":  bytesSent.Int64,
			"created_at":  createdAt.Int64,
		})
	}

	json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
}

func (m *MITMAPI) handleCert(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", "attachment; filename=switchblade-ca.crt")
	w.Write(m.ca.CertPEM)
}

func (m *MITMAPI) handleDNSInstall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Collect all domains from registered handlers
	// For now, use a default set — in production this would come from the handler registry
	domains := []string{
		"api2.cursor.sh",
		"cursor.sh",
		"api.github.com",
		"copilot-proxy.githubusercontent.com",
		"api.kiro.dev",
		"kiro.dev",
		"api.antigravity.dev",
		"antigravity.dev",
	}

	if err := m.dns.Install(domains); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "installed", "domains": strings.Join(domains, ", ")})
}

func (m *MITMAPI) handleDNSUninstall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := m.dns.Uninstall(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "uninstalled"})
}
