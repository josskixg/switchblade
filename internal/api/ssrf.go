package api

import (
	"net/http"

	"switchblade/internal/ssrf"
)

// IsSafeURL validates that rawURL does not point to an internal/private address.
func IsSafeURL(rawURL string) error {
	return ssrf.ValidateURL(rawURL)
}

// ValidateBaseURL is an alias for backward compatibility with existing callers.
// Deprecated: use IsSafeURL instead.
func ValidateBaseURL(rawURL string) error {
	return IsSafeURL(rawURL)
}

// IsInternalIP delegates to the shared ssrf package.
func IsInternalIP(host string) bool {
	return ssrf.IsInternalIP(host)
}

// HandleSSRFCheck returns an HTTP handler for POST /api/ssrf/check?url=xxx.
func HandleSSRFCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := r.URL.Query().Get("url")
		if u == "" {
			jsonError(w, http.StatusBadRequest, "url parameter required")
			return
		}

		err := IsSafeURL(u)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			jsonOK(w, map[string]any{"safe": false, "error": err.Error()})
			return
		}
		jsonOK(w, map[string]any{"safe": true})
	}
}
