package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"switchblade/internal/auth"
)

// AuthKeyV2OrLegacy is a hybrid middleware for proxy routes.
// It tries DB-backed API keys first (tenant-scoped, with quota + scope checks).
// Falls back to the legacy global API_KEY for backward compatibility.
func AuthKeyV2OrLegacy(db *sql.DB, legacyKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract key from header
			keyValue := r.Header.Get("x-api-key")
			if keyValue == "" {
				if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
					keyValue = strings.TrimPrefix(auth, "Bearer ")
				}
			}
			if keyValue == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "missing API key"})
				return
			}

			// Try 1: DB-backed v2 key (tenant-scoped)
			if tryAuthKeyV2(db, next, w, r, keyValue) {
				return
			}

			// Try 2: Legacy global API_KEY (backward compat)
			if keyValue == legacyKey {
				next.ServeHTTP(w, r)
				return
			}

			// Neither matched
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid API key"})
		})
	}
}

// tryAuthKeyV2 validates a key against the database.
// Returns true once the request has been answered — by the next handler or by an
// error response — and false only when the key is genuinely absent from api_keys
// and legacy auth should get a look at it.
func tryAuthKeyV2(db *sql.DB, next http.Handler, w http.ResponseWriter, r *http.Request, keyValue string) bool {
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
		// Not a v2 key — fall through to legacy
		return false
	}
	if err != nil {
		// A lookup that could not run says nothing about the key. Falling through
		// would offer the request to the legacy comparison, and a match there admits
		// it with no tenant, no scopes and no quota — so a database outage would
		// quietly widen every caller's privileges. Report the outage instead.
		log.Printf("[auth] v2 key lookup failed: %v", err)
		jsonError(w, http.StatusServiceUnavailable, "authentication temporarily unavailable")
		return true
	}

	// api_keys.enabled = 1 means active; 0 means disabled
	if enabled != 1 {
		jsonError(w, http.StatusForbidden, "API key is not active")
		return true
	}

	// A tenant that is missing or suspended is a decision and stays a 403; a
	// tenants table that cannot be read is an outage and must not be reported as
	// one, or every failure looks like a permissions problem to the operator.
	var tenantStatus string
	terr := db.QueryRow("SELECT status FROM tenants WHERE id = ?", tenantID).Scan(&tenantStatus)
	if terr != nil && terr != sql.ErrNoRows {
		log.Printf("[auth] tenant status lookup failed: %v", terr)
		jsonError(w, http.StatusServiceUnavailable, "authentication temporarily unavailable")
		return true
	}
	if terr == sql.ErrNoRows || tenantStatus != "active" {
		jsonError(w, http.StatusForbidden, "tenant is not active")
		return true
	}

	// Default role for v2 keys: admin (scopes handle restriction)
	role := "admin"

	// Load scopes
	scopes, err := loadKeyScopes(db, keyID)
	if err != nil {
		// Unknown scopes are not the same as unrestricted scopes: continuing here
		// would grant the key every model it was explicitly fenced off from.
		log.Printf("[auth] failed to load scopes: %v", err)
		jsonError(w, http.StatusServiceUnavailable, "authentication temporarily unavailable")
		return true
	}

	// Check model scope if body is JSON
	if len(scopes) > 0 && isJSONRequest(r) {
		model, body, berr := extractModelFromBody(r.Body)
		if berr == nil && model != "" {
			if !CanAccessModel(scopes, model) {
				jsonError(w, http.StatusForbidden, "API key does not have access to model: "+model)
				return true
			}
			// Restore body for downstream handler
			r.Body = body
		}
	}

	// Check quota
	withinQuota, qerr := CheckQuota(db, tenantID)
	if qerr != nil {
		log.Printf("[auth] quota check failed: %v", qerr)
		// Don't block on quota DB error — let request through
	} else if !withinQuota {
		jsonError(w, http.StatusTooManyRequests, "Daily/Monthly token quota exceeded")
		return true
	}

	// Update last_used (fire and forget)
	go db.Exec("UPDATE api_keys SET last_used_at = ? WHERE id = ?", time.Now().Unix(), keyID)

	// Inject context values
	ctx := r.Context()
	ctx = context.WithValue(ctx, ContextTenantID, tenantID)
	ctx = context.WithValue(ctx, ContextRole, role)
	ctx = context.WithValue(ctx, ContextKeyID, keyID)
	ctx = context.WithValue(ctx, ContextScopes, scopes)
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}

// isJSONRequest reports whether r carries a JSON body worth inspecting for a
// model name. The media type is parsed rather than compared, because an exact
// match against "application/json" lets any client skip the scope check by
// sending the equally valid "application/json; charset=utf-8".
func isJSONRequest(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

// AuthAPIKey validates requests via Authorization: Bearer <key> or x-api-key header.
// Skips /health and /ready paths.
func AuthAPIKey(next http.Handler) http.Handler {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "switchblade-secret"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}

		key := ""

		// Try Authorization: Bearer <key>
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			key = strings.TrimPrefix(auth, "Bearer ")
		}

		// Fallback to x-api-key header
		if key == "" {
			key = r.Header.Get("x-api-key")
		}

		if key == apiKey {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", "Bearer")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	})
}

// AuthJWTOrAPIKey tries JWT first, then falls back to legacy API key.
// JWT auth sets tenant_id/role/user_id in context; API key auth does not.
func AuthJWTOrAPIKey(jwtSecret []byte, database *sql.DB, legacyKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" || r.URL.Path == "/ready" {
				next.ServeHTTP(w, r)
				return
			}

			// Extract bearer token
			token := ""
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				token = strings.TrimPrefix(auth, "Bearer ")
			}

			// Try JWT first — if token has 3 dot-separated parts
			if strings.Count(token, ".") == 2 {
				// Check JWT blacklist
				var count int
				database.QueryRow("SELECT COUNT(*) FROM jwt_blacklist WHERE token = ?", token).Scan(&count)
				if count == 0 {
					claims, err := auth.VerifyJWT(token, jwtSecret)
					if err == nil {
						ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
						ctx = context.WithValue(ctx, ctxTenantID, claims.TenantID)
						ctx = context.WithValue(ctx, ctxRole, claims.Role)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			// Fallback to API key
			if token == legacyKey {
				next.ServeHTTP(w, r)
				return
			}

			// Also check x-api-key header
			if key := r.Header.Get("x-api-key"); key == legacyKey {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		})
	}
}

// CORS adds cross-origin headers and handles preflight OPTIONS requests.
func CORS(next http.Handler) http.Handler {
	origin := os.Getenv("CORS_ORIGIN")
	if origin == "" {
		origin = "*"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequestLogger logs method, path, status code, and duration.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)

		log.Printf("%s %s %d %s", r.Method, r.URL.Path, ww.status, time.Since(start).Round(time.Microsecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// LimitBodySize limits request body size for JSON API handlers to prevent DoS via oversized payloads.
func LimitBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				json.NewEncoder(w).Encode(map[string]string{"error": "request body too large"})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// RequireJSONHandler returns a handler that validates Content-Type is application/json
// before delegating to the wrapped handler. Returns 415 if the content-type is wrong.
func RequireJSONHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "" && !strings.Contains(strings.ToLower(ct), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			json.NewEncoder(w).Encode(map[string]string{"error": "application/json content-type required"})
			return
		}
		h(w, r)
	}
}
