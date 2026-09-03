package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"switchblade/internal/db"
)

// MountKeysAPI registers API key management endpoints.
func MountKeysAPI(r chi.Router, database *db.DB) {
	r.Get("/api/keys", listKeys(database))
	r.Post("/api/keys", RequireJSONHandler(createKey(database)))
	r.Delete("/api/keys/{id}", deleteKey(database))
}

func listKeys(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Tenant-scoped listing: developers see only their tenant's keys.
		// Legacy global-key callers carry no tenant — they see the unscoped
		// legacy set, matching the behavior of the endpoints they were minted for.
		ctxTenant, _ := r.Context().Value(ctxTenantID).(string)
		ctxRole, _ := r.Context().Value(ctxRole).(string)

		query := `SELECT id, name, key_hash, created_at, last_used_at, enabled FROM api_keys`
		args := []any{}
		if ctxTenant != "" && ctxRole != "owner" && ctxRole != "admin" {
			query += ` WHERE tenant_id = ?`
			args = append(args, ctxTenant)
		}
		query += ` ORDER BY id`

		rows, err := database.Query(query, args...)
		if err != nil {
			slog.Error("[api] list keys failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		type keyRow struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			KeyHash    string `json:"key_hash"`
			CreatedAt  int64  `json:"created_at"`
			LastUsedAt int64  `json:"last_used_at"`
			Enabled    bool   `json:"enabled"`
		}
		var out []keyRow
		for rows.Next() {
			var k keyRow
			var enabled int
			if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.CreatedAt, &k.LastUsedAt, &enabled); err != nil {
				slog.Error("[api] key scan failed", "err", err)
				jsonError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			k.Enabled = enabled == 1
			out = append(out, k)
		}
		jsonOK(w, out)
	}
}

func createKey(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			jsonError(w, http.StatusBadRequest, "name required")
			return
		}

		// generate 32 random bytes → hex key
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			jsonError(w, http.StatusInternalServerError, "rng failure")
			return
		}
		key := hex.EncodeToString(raw)

		sum := sha256.Sum256([]byte(key))
		hash := hex.EncodeToString(sum[:])

		// Keys are tenant-scoped in schema_v2 — insert under the caller's tenant
		// (or '_system' for legacy unscoped callers) so tenant listing/filtering works.
		ctxTenant, _ := r.Context().Value(ctxTenantID).(string)
		tenantID := ctxTenant
		if tenantID == "" {
			tenantID = "_system"
		}

		now := time.Now().Unix()
		res, err := database.Exec(
			`INSERT INTO api_keys (tenant_id, name, key_hash, created_at, last_used_at, enabled) VALUES (?, ?, ?, ?, 0, 1)`,
			tenantID, body.Name, hash, now,
		)
		if err != nil {
			slog.Error("[api] create key failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		id, _ := res.LastInsertId()
		w.WriteHeader(http.StatusCreated)
		// return the plaintext key once — caller must store it
		jsonOK(w, map[string]any{"id": id, "key": key, "name": body.Name})
	}
}

func deleteKey(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// Tenant gate: developers may only delete their own tenant's keys.
		ctxTenant, _ := r.Context().Value(ctxTenantID).(string)
		ctxRole, _ := r.Context().Value(ctxRole).(string)
		if ctxTenant != "" && ctxRole != "owner" && ctxRole != "admin" {
			var keyTenant string
			err := database.QueryRow(`SELECT tenant_id FROM api_keys WHERE id = ?`, id).Scan(&keyTenant)
			if err == sql.ErrNoRows {
				jsonError(w, http.StatusNotFound, "key not found")
				return
			}
			if err != nil {
				slog.Error("[api] key tenant lookup failed", "err", err)
				jsonError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			if keyTenant != ctxTenant {
				jsonError(w, http.StatusForbidden, "cannot delete another tenant's key")
				return
			}
		}

		if _, err := database.Exec(`DELETE FROM api_keys WHERE id = ?`, id); err != nil {
			slog.Error("[api] delete key failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}
