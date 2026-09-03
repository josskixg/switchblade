// Package api provides HTTP handlers for the Switchblade API.
package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/reqctx"
)

// Context keys for v2 auth — aliases of the canonical reqctx keys.
const (
	ContextTenantID = reqctx.TenantID
	ContextRole     = reqctx.Role
	ContextKeyID    = reqctx.KeyID
	ContextScopes   = reqctx.Scopes
)

// GenerateKeyV2 creates a new API key with "sk_live_" prefix + 32 hex chars.
// Returns (keyValue, keyHash).
func GenerateKeyV2() (string, string) {
	b := make([]byte, 16)
	rand.Read(b)
	key := "sk_live_" + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(key))
	return key, hex.EncodeToString(hash[:])
}

// MatchesScope checks if a scope pattern matches a model name.
// "gpt-*" matches "gpt-4", "gpt-3.5-turbo"; "*" matches everything.
func MatchesScope(scopePattern string, modelName string) bool {
	if scopePattern == "*" {
		return true
	}
	if strings.HasSuffix(scopePattern, "*") {
		prefix := strings.TrimSuffix(scopePattern, "*")
		return strings.HasPrefix(modelName, prefix)
	}
	return scopePattern == modelName
}

// CanAccessModel checks if any scope pattern matches the given model.
func CanAccessModel(scopes []string, modelName string) bool {
	for _, scope := range scopes {
		if MatchesScope(scope, modelName) {
			return true
		}
	}
	return false
}

// loadKeyScopes retrieves scope patterns for an API key.
func loadKeyScopes(db *sql.DB, keyID int64) ([]string, error) {
	rows, err := db.Query("SELECT model_pattern FROM api_key_scopes WHERE api_key_id = ?", keyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scopes []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		scopes = append(scopes, s)
	}
	return scopes, rows.Err()
}

// extractModelFromBody reads JSON body to extract the "model" field,
// then returns a restored body reader so downstream handlers can read it.
func extractModelFromBody(body io.ReadCloser) (model string, restored io.ReadCloser, err error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return "", nil, err
	}
	defer body.Close()

	var req struct {
		Model string `json:"model"`
	}
	json.Unmarshal(data, &req)

	return req.Model, io.NopCloser(strings.NewReader(string(data))), nil
}

// AuthKeyV2 is middleware that validates API keys v2, checks scopes against
// the requested model, verifies tenant status, and injects tenant_id/role/scopes
// into context.
func AuthKeyV2(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyValue := r.Header.Get("x-api-key")
			if keyValue == "" {
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(auth, "Bearer ") {
					keyValue = strings.TrimPrefix(auth, "Bearer ")
				}
			}
			if keyValue == "" {
				jsonError(w, http.StatusUnauthorized, "missing API key")
				return
			}

			keyHash := sha256.Sum256([]byte(keyValue))
			hashHex := hex.EncodeToString(keyHash[:])

			var keyID int64
			var tenantID string

			err := db.QueryRow(`
				SELECT id, tenant_id
				FROM api_keys
				WHERE key_hash = ? AND enabled = 1
			`, hashHex).Scan(&keyID, &tenantID)

			if err == sql.ErrNoRows {
				jsonError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "database error")
				return
			}

			// Check tenant status
			var tenantStatus string
			err = db.QueryRow("SELECT status FROM tenants WHERE id = ?", tenantID).Scan(&tenantStatus)
			if err != nil || tenantStatus != "active" {
				jsonError(w, http.StatusForbidden, "tenant is not active")
				return
			}

			// Load scopes
			scopes, err := loadKeyScopes(db, keyID)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to load key scopes")
				return
			}

			// Extract model from request and check scope
			var modelName string
			if r.Header.Get("Content-Type") == "application/json" && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
				modelName, r.Body, err = extractModelFromBody(r.Body)
				if err != nil {
					jsonError(w, http.StatusBadRequest, "invalid request body")
					return
				}
			} else {
				modelName = r.URL.Query().Get("model")
			}

			// If no model specified, skip scope check (allow)
			if modelName != "" && len(scopes) > 0 {
				if !CanAccessModel(scopes, modelName) {
					jsonError(w, http.StatusForbidden, fmt.Sprintf("API key does not have access to model: %s", modelName))
					return
				}
			}

			// Check quota
			withinQuota, err := CheckQuota(db, tenantID)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to check quota")
				return
			}
			if !withinQuota {
				jsonError(w, http.StatusTooManyRequests, "Daily/Monthly token quota exceeded")
				return
			}

			// Update last_used
			db.Exec("UPDATE api_keys SET last_used_at = ? WHERE id = ?", time.Now().Unix(), keyID)

			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextTenantID, tenantID)
			ctx = context.WithValue(ctx, ContextRole, "developer")
			ctx = context.WithValue(ctx, ContextKeyID, keyID)
			ctx = context.WithValue(ctx, ContextScopes, scopes)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// HandleCreateKeyV2 handles POST /api/keys/v2
// Request: {name, tenant_id?, scopes: ["gpt-*", "claude-*"], expires_at?}
// tenant_id defaults to the caller's own tenant; only owners/admins may mint
// keys for another tenant (root/admin operators mint for any tenant).
// Response: {key_id, key_value, scopes}
func HandleCreateKeyV2(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req struct {
			Name      string   `json:"name"`
			TenantID  string   `json:"tenant_id"`
			Scopes    []string `json:"scopes"`
			ExpiresAt *int64   `json:"expires_at"`
			Role      string   `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Name == "" {
			jsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		// Tenant gate: default to the caller's tenant. Cross-tenant minting is
		// owner-only; a key scoping a tenant's quota and models is privilege.
		ctxTenant, _ := r.Context().Value(reqctx.TenantID).(string)
		ctxRole, _ := r.Context().Value(reqctx.Role).(string)
		targetTenant := req.TenantID
		if targetTenant == "" {
			targetTenant = ctxTenant
		}
		if targetTenant != ctxTenant && ctxRole != "owner" && ctxRole != "admin" {
			jsonError(w, http.StatusForbidden, "only owners and admins can create keys for another tenant")
			return
		}
		if targetTenant == "" {
			jsonError(w, http.StatusBadRequest, "tenant_id is required")
			return
		}

		// Verify tenant exists and is active
		var tenantStatus string
		err := db.QueryRow("SELECT status FROM tenants WHERE id = ?", targetTenant).Scan(&tenantStatus)
		if err == sql.ErrNoRows {
			jsonError(w, http.StatusNotFound, "tenant not found")
			return
		}
		if err != nil || tenantStatus != "active" {
			jsonError(w, http.StatusForbidden, "tenant is not active")
			return
		}

		keyValue, keyHash := GenerateKeyV2()
		now := time.Now().Unix()

		tx, err := db.Begin()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer tx.Rollback()

		var keyID int64
		_, err = tx.Exec(
			"INSERT INTO api_keys (tenant_id, key_hash, name, created_at, enabled) VALUES (?, ?, ?, ?, 1)",
			targetTenant, keyHash, req.Name, now,
		)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to create key")
			return
		}

		// Get the key ID
		err = tx.QueryRow("SELECT id FROM api_keys WHERE key_hash = ?", keyHash).Scan(&keyID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to retrieve key ID")
			return
		}

		// Insert scopes
		if len(req.Scopes) == 0 {
			req.Scopes = []string{"*"}
		}
		for _, scope := range req.Scopes {
			_, err = tx.Exec("INSERT INTO api_key_scopes (api_key_id, model_pattern) VALUES (?, ?)", keyID, scope)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to create scopes")
				return
			}
		}

		if err := tx.Commit(); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to commit transaction")
			return
		}

		jsonOK(w, map[string]interface{}{
			"key_id":     keyID,
			"key_value":  keyValue,
			"scopes":     req.Scopes,
			"tenant_id":  targetTenant,
			"name":       req.Name,
			"role":       "developer",
			"expires_at": req.ExpiresAt,
		})
	}
}

// HandleListKeysV2 handles GET /api/keys/v2
// Lists keys of the caller's tenant; ?tenant_id= is honored only for
// owners/admins, so a developer cannot enumerate another tenant's keys.
func HandleListKeysV2(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctxTenant, _ := r.Context().Value(reqctx.TenantID).(string)
		ctxRole, _ := r.Context().Value(reqctx.Role).(string)

		tenantID := r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			tenantID = ctxTenant
		}
		if tenantID != ctxTenant && ctxRole != "owner" && ctxRole != "admin" {
			jsonError(w, http.StatusForbidden, "owner or admin access required")
			return
		}
		if tenantID == "" {
			jsonError(w, http.StatusBadRequest, "tenant_id is required")
			return
		}

		rows, err := db.Query(`
			SELECT k.id, k.name, k.enabled, k.created_at, k.last_used_at
			FROM api_keys k
			WHERE k.tenant_id = ?
			ORDER BY k.created_at DESC
		`, tenantID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		type keyInfo struct {
			ID        int64    `json:"id"`
			Name      string   `json:"name"`
			Role      string   `json:"role"`
			Status    string   `json:"status"`
			CreatedAt int64    `json:"created_at"`
			LastUsed  *int64   `json:"last_used"`
			ExpiresAt *int64   `json:"expires_at"`
			Scopes    []string `json:"scopes"`
		}

		var keys []keyInfo
		for rows.Next() {
			var k keyInfo
			var enabled int
			var lu sql.NullInt64
			if err := rows.Scan(&k.ID, &k.Name, &enabled, &k.CreatedAt, &lu); err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to scan key")
				return
			}
			k.Role = "developer"
			if enabled == 1 {
				k.Status = "active"
			} else {
				k.Status = "disabled"
			}
			if lu.Valid {
				k.LastUsed = &lu.Int64
			}

			scopes, err := loadKeyScopes(db, k.ID)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to load scopes")
				return
			}
			k.Scopes = scopes
			keys = append(keys, k)
		}

		if keys == nil {
			keys = []keyInfo{}
		}

		jsonOK(w, map[string]interface{}{"keys": keys})
	}
}

// HandleDeleteKeyV2 handles DELETE /api/keys/v2/{id}
func HandleDeleteKeyV2(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// chi routes, not net/http pattern routes — r.PathValue is always empty here.
		id := chi.URLParam(r, "id")
		if id == "" {
			jsonError(w, http.StatusBadRequest, "key ID is required")
			return
		}

		// Tenant gate: developers may only delete their own tenant's keys.
		ctxTenant, _ := r.Context().Value(reqctx.TenantID).(string)
		ctxRole, _ := r.Context().Value(reqctx.Role).(string)
		if ctxTenant != "" && ctxRole != "owner" && ctxRole != "admin" {
			var keyTenant string
			err := db.QueryRow("SELECT tenant_id FROM api_keys WHERE id = ?", id).Scan(&keyTenant)
			if err == sql.ErrNoRows {
				jsonError(w, http.StatusNotFound, "key not found")
				return
			}
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "database error")
				return
			}
			if keyTenant != ctxTenant {
				jsonError(w, http.StatusForbidden, "cannot delete another tenant's key")
				return
			}
		}

		tx, err := db.Begin()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer tx.Rollback()

		// Delete scopes first (foreign key)
		_, err = tx.Exec("DELETE FROM api_key_scopes WHERE api_key_id = ?", id)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to delete scopes")
			return
		}

		// Delete key
		res, err := tx.Exec("DELETE FROM api_keys WHERE id = ?", id)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to delete key")
			return
		}

		rows, _ := res.RowsAffected()
		if rows == 0 {
			jsonError(w, http.StatusNotFound, "key not found")
			return
		}

		if err := tx.Commit(); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to commit")
			return
		}

		jsonOK(w, map[string]interface{}{"deleted": true, "key_id": id})
	}
}

// HandleGetKeyByValue handles GET /api/keys/v2/lookup?key=xxx
// Used by middleware to find key details by its raw value.
func HandleGetKeyByValue(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keyValue := r.URL.Query().Get("key")
		if keyValue == "" {
			jsonError(w, http.StatusBadRequest, "key parameter is required")
			return
		}

		keyHash := sha256.Sum256([]byte(keyValue))
		hashHex := hex.EncodeToString(keyHash[:])

		var keyID int64
		var tenantID string
		var enabled int

		err := db.QueryRow(`
			SELECT id, tenant_id, enabled
			FROM api_keys
			WHERE key_hash = ?
		`, hashHex).Scan(&keyID, &tenantID, &enabled)

		if err == sql.ErrNoRows {
			jsonError(w, http.StatusNotFound, "key not found")
			return
		}
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "database error")
			return
		}

		// Cross-tenant metadata leak: a developer JWT must not be able to probe
		// arbitrary key values and learn which tenant/status/scopes they carry.
		ctxTenant, _ := r.Context().Value(reqctx.TenantID).(string)
		ctxRole, _ := r.Context().Value(reqctx.Role).(string)
		if ctxTenant != "" && tenantID != ctxTenant && ctxRole != "owner" && ctxRole != "admin" {
			jsonError(w, http.StatusNotFound, "key not found")
			return
		}

		scopes, err := loadKeyScopes(db, keyID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load scopes")
			return
		}

		status := "disabled"
		if enabled == 1 {
			status = "active"
		}
		resp := map[string]interface{}{
			"key_id":    keyID,
			"tenant_id": tenantID,
			"role":      "developer",
			"status":    status,
			"scopes":    scopes,
		}

		jsonOK(w, resp)
	}
}
