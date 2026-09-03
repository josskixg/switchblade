// Package api — JWT auth handlers and middleware for Switchblade.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/auth"
	"switchblade/internal/reqctx"
	"switchblade/internal/ws"
)

// AuthConfig wires database, hub, and JWT secret into auth handlers.
type AuthConfig struct {
	DB        *sql.DB
	Hub       *ws.Hub
	JWTSecret []byte
}

// Request-context keys. The canonical definitions live in internal/reqctx so
// that api and proxy share one set — these are short aliases for local use.
const (
	ctxUserID   = reqctx.UserID
	ctxTenantID = reqctx.TenantID
	ctxRole     = reqctx.Role
)

// MountAuthAPI attaches all /api/auth/* routes to the router.
func MountAuthAPI(r chi.Router, cfg AuthConfig) {
	// Public — login + refresh only. No self-registration.
	r.Group(func(r chi.Router) {
		r.Post("/api/auth/login", RequireJSONHandler(HandleLogin(cfg)))
		r.Post("/api/auth/refresh", RequireJSONHandler(HandleRefreshToken(cfg)))
	})

	// Protected — requires JWT.
	r.Group(func(r chi.Router) {
		r.Use(AuthJWT(cfg.JWTSecret, cfg.DB))
		r.Post("/api/auth/logout", RequireJSONHandler(HandleLogout(cfg)))
		r.Get("/api/auth/me", HandleMe(cfg))

		// User management — owner/admin only.
		r.Get("/api/auth/users", HandleListUsers(cfg))
		r.Post("/api/auth/users", RequireJSONHandler(HandleCreateUser(cfg)))
	})

	// Ensure JWT blacklist table exists.
	cfg.DB.Exec(`CREATE TABLE IF NOT EXISTS jwt_blacklist (
		token TEXT PRIMARY KEY,
		expires_at DATETIME
	)`)
}

// HandleCreateUser creates a new user under the caller's tenant.
// Requires owner or admin role. The new user gets role 'developer' by default.
func HandleCreateUser(cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Role check — only owner/admin can invite users.
		role, _ := r.Context().Value(ctxRole).(string)
		if role != "owner" && role != "admin" {
			jsonError(w, http.StatusForbidden, "only owners and admins can create users")
			return
		}

		tenantID, _ := r.Context().Value(ctxTenantID).(string)
		if tenantID == "" {
			jsonError(w, http.StatusForbidden, "no tenant in context")
			return
		}

		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Email    string `json:"email"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if body.Username == "" || body.Password == "" {
			jsonError(w, http.StatusBadRequest, "username and password required")
			return
		}
		// Only owner can create admin/owner users.
		newRole := body.Role
		if newRole == "" {
			newRole = "developer"
		}
		if (newRole == "owner" || newRole == "admin") && role != "owner" {
			jsonError(w, http.StatusForbidden, "only owners can create admin/owner users")
			return
		}
		if newRole != "owner" && newRole != "admin" && newRole != "developer" && newRole != "viewer" {
			jsonError(w, http.StatusBadRequest, "invalid role")
			return
		}

		passwordHash, err := auth.HashPassword(body.Password)
		if err != nil {
			slog.Error("[auth] hash password failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		_, err = cfg.DB.Exec(
			`INSERT INTO users (username, password_hash, email, tenant_id, role, status, created_at) VALUES (?, ?, ?, ?, ?, 'active', ?)`,
			body.Username, passwordHash, body.Email, tenantID, newRole, time.Now().Unix(),
		)
		if err != nil {
			jsonError(w, http.StatusConflict, "username already exists")
			return
		}

		jsonOK(w, map[string]any{
			"username": body.Username,
			"email":    body.Email,
			"role":     newRole,
		})
	}
}

// HandleListUsers returns all users in the caller's tenant.
func HandleListUsers(cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(ctxRole).(string)
		if role != "owner" && role != "admin" {
			jsonError(w, http.StatusForbidden, "only owners and admins can list users")
			return
		}

		tenantID, _ := r.Context().Value(ctxTenantID).(string)
		rows, err := cfg.DB.Query(
			`SELECT username, email, role, status, created_at FROM users WHERE tenant_id = ? ORDER BY created_at DESC`,
			tenantID,
		)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		users := make([]map[string]any, 0)
		for rows.Next() {
			var username, email, userRole, status string
			var createdAt int64
			if err := rows.Scan(&username, &email, &userRole, &status, &createdAt); err != nil {
				continue
			}
			users = append(users, map[string]any{
				"username":   username,
				"email":      email,
				"role":       userRole,
				"status":     status,
				"created_at": createdAt,
			})
		}

		jsonOK(w, users)
	}
}

// HandleLogin verifies credentials and returns JWT token.
func HandleLogin(cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		var userID, passwordHash, tenantID, tenantName, role, tenantStatus string
		var email string
		err := cfg.DB.QueryRow(`
			SELECT u.username, u.password_hash, t.id, t.name, u.role, t.status, u.email
			FROM users u JOIN tenants t ON u.tenant_id = t.id
			WHERE u.username = ?`,
			body.Username,
		).Scan(&userID, &passwordHash, &tenantID, &tenantName, &role, &tenantStatus, &email)
		if err == sql.ErrNoRows {
			jsonError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if err != nil {
			slog.Error("[auth] login query failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		if !auth.VerifyPassword(body.Password, passwordHash) {
			jsonError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		if tenantStatus != "active" {
			jsonError(w, http.StatusForbidden, "tenant is not active")
			return
		}

		token, err := auth.SignJWT(auth.JWTClaims{
			UserID:    userID,
			TenantID:  tenantID,
			Role:      role,
			ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		}, cfg.JWTSecret)
		if err != nil {
			slog.Error("[auth] sign jwt failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		jsonOK(w, map[string]any{
			"token":  token,
			"user":   map[string]any{"username": userID, "email": email, "role": role},
			"tenant": map[string]any{"name": tenantName, "tier": "free", "status": tenantStatus},
		})
	}
}

// HandleLogout blacklists the current JWT.
func HandleLogout(cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearer(r)
		if token == "" {
			jsonError(w, http.StatusUnauthorized, "missing token")
			return
		}

		claims, err := auth.VerifyJWT(token, cfg.JWTSecret)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		expiry := time.Unix(claims.ExpiresAt, 0).Format(time.RFC3339)
		cfg.DB.Exec(`INSERT OR IGNORE INTO jwt_blacklist (token, expires_at) VALUES (?, ?)`, token, expiry)

		jsonOK(w, map[string]string{"status": "logged out"})
	}
}

// HandleMe returns the current authenticated user.
func HandleMe(cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(ctxUserID).(string)

		var username, email, role, tenantName string
		err := cfg.DB.QueryRow(`
			SELECT u.username, u.email, u.role, t.name
			FROM users u JOIN tenants t ON u.tenant_id = t.id
			WHERE u.username = ?`,
			userID,
		).Scan(&username, &email, &role, &tenantName)
		if err == sql.ErrNoRows {
			jsonError(w, http.StatusNotFound, "user not found")
			return
		}
		if err != nil {
			slog.Error("[auth] me query failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		jsonOK(w, map[string]any{
			"username": username,
			"email":    email,
			"role":     role,
			"tenant":   tenantName,
		})
	}
}

// HandleRefreshToken issues a new JWT for the current user.
func HandleRefreshToken(cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearer(r)
		if token == "" {
			jsonError(w, http.StatusUnauthorized, "missing token")
			return
		}

		claims, err := auth.VerifyJWT(token, cfg.JWTSecret)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		newToken, err := auth.SignJWT(auth.JWTClaims{
			UserID:    claims.UserID,
			TenantID:  claims.TenantID,
			Role:      claims.Role,
			ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		}, cfg.JWTSecret)
		if err != nil {
			slog.Error("[auth] refresh sign failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		jsonOK(w, map[string]string{"token": newToken})
	}
}

// AuthJWT middleware extracts and validates JWT from the Authorization header.
// On success it sets user_id, tenant_id, and role in the request context.
func AuthJWT(jwtSecret []byte, db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r)
			if token == "" {
				jsonError(w, http.StatusUnauthorized, "missing authorization")
				return
			}

			// Check blacklist.
			var count int
			db.QueryRow(`SELECT COUNT(*) FROM jwt_blacklist WHERE token = ?`, token).Scan(&count)
			if count > 0 {
				jsonError(w, http.StatusUnauthorized, "token revoked")
				return
			}

			claims, err := auth.VerifyJWT(token, jwtSecret)
			if err != nil {
				slog.Error("[auth] jwt verify failed", "err", err)
				jsonError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
			ctx = context.WithValue(ctx, ctxTenantID, claims.TenantID)
			ctx = context.WithValue(ctx, ctxRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearer returns the token from "Authorization: Bearer <token>".
func extractBearer(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" {
		return ""
	}
	return token
}
