package api

import (
	"github.com/go-chi/chi/v5"

	"switchblade/internal/proxy"
)

// MountTTSAPI mounts the text-to-speech endpoint on the router.
func MountTTSAPI(r chi.Router, proxyRouter *proxy.Router) {
	r.Post("/v1/audio/speech", proxyRouter.ServeTTS)
}
