package handlers

import (
	"net/http"
	"switchblade/internal/providers"
)

// Handler defines the interface for IDE-specific request interception.
type Handler interface {
	// Name returns the IDE name (e.g., "cursor", "copilot").
	Name() string

	// Domains returns the list of domains this handler intercepts.
	Domains() []string

	// Match returns true if the request should be handled by this handler.
	Match(req *http.Request) bool

	// Parse extracts the provider and model from the request.
	Parse(req *http.Request) (provider string, model string, err error)

	// Rewrite modifies the request to use the selected account's credentials.
	Rewrite(req *http.Request, acc *providers.Account) error
}

// Registry holds all registered IDE handlers.
type Registry struct {
	handlers []Handler
}

// NewRegistry creates an empty handler registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a handler to the registry.
func (r *Registry) Register(h Handler) {
	r.handlers = append(r.handlers, h)
}

// Match finds the first handler that matches the request.
func (r *Registry) Match(req *http.Request) Handler {
	for _, h := range r.handlers {
		if h.Match(req) {
			return h
		}
	}
	return nil
}

// All returns all registered handlers.
func (r *Registry) All() []Handler {
	return r.handlers
}
