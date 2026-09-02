package api

import (
	"github.com/go-chi/chi/v5"

	"switchblade/internal/proxy"
)

// MountEmbeddingsAPI mounts the embeddings endpoint on the router.
func MountEmbeddingsAPI(r chi.Router, proxyRouter *proxy.Router) {
	r.Post("/v1/embeddings", proxyRouter.ServeEmbeddings)
}
