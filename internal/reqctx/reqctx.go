// Package reqctx holds the canonical request-scoped context keys.
//
// These used to be declared twice — api.ctxTenantID and api.ContextTenantID —
// as two distinct unexported/exported types carrying the same string. A handler
// reading one could not see a value written under the other, so the tenant id
// silently vanished on some auth paths. Every package now shares these keys so
// that cannot happen again, and proxy can read them without importing api.
package reqctx

import "context"

type key string

const (
	TenantID  key = "tenant_id"
	UserID    key = "user_id"
	Role      key = "role"
	KeyID     key = "key_id"
	Scopes    key = "scopes"
	RequestID key = "request_id"
)

// WithTenant returns ctx carrying the tenant id.
func WithTenant(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, TenantID, id)
}

// Tenant returns the tenant id, or "" when the request was not tenant-scoped
// (legacy global API key, or an unauthenticated public route).
func Tenant(ctx context.Context) string {
	s, _ := ctx.Value(TenantID).(string)
	return s
}

// User returns the authenticated user id, or "".
func User(ctx context.Context) string {
	s, _ := ctx.Value(UserID).(string)
	return s
}

// RoleOf returns the caller's role, or "".
func RoleOf(ctx context.Context) string {
	s, _ := ctx.Value(Role).(string)
	return s
}

// APIKeyID returns the id of the API key that authenticated the request, or 0.
func APIKeyID(ctx context.Context) int64 {
	id, _ := ctx.Value(KeyID).(int64)
	return id
}

// ModelScopes returns the model patterns the key is limited to, or nil.
func ModelScopes(ctx context.Context) []string {
	s, _ := ctx.Value(Scopes).([]string)
	return s
}

// ReqID returns the per-request correlation id, or "".
func ReqID(ctx context.Context) string {
	s, _ := ctx.Value(RequestID).(string)
	return s
}
