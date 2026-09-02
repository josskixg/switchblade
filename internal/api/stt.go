package api

import (
	"github.com/go-chi/chi/v5"

	"switchblade/internal/proxy"
)

// MountSTTAPI mounts the speech-to-text endpoint on the router.
func MountSTTAPI(r chi.Router, proxyRouter *proxy.Router) {
	r.Post("/v1/audio/transcriptions", proxyRouter.ServeSTT)
}
