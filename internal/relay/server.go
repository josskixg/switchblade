package relay

import (
	"io"
	"log/slog"
	"net/http"
)

// Server proxies forwarded HTTP requests to a local handler,
// enforcing a shared secret via X-Relay-Secret.
type Server struct {
	handler http.Handler
	secret  string
}

// NewServer creates a relay server that delegates to handler.
// secret may be empty to disable secret validation.
func NewServer(handler http.Handler, secret string) *Server {
	return &Server{handler: handler, secret: secret}
}

// ServeHTTP implements http.Handler. Clients POST the original request
// body to any path; the server reconstructs and forwards it internally.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.secret != "" && r.Header.Get("X-Relay-Secret") != s.secret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Strip the relay secret before forwarding downstream.
	r.Header.Del("X-Relay-Secret")

	// ponytail: reuse the incoming request directly — no body copy needed
	// because the handler reads r.Body and we own the connection.
	rec := &responseRecorder{header: make(http.Header), code: http.StatusOK}
	s.handler.ServeHTTP(rec, r)

	for k, vv := range rec.header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rec.code)
	if _, err := w.Write(rec.body); err != nil {
		slog.Warn("[relay] write response", "err", err)
	}
}

// responseRecorder is a minimal http.ResponseWriter buffer.
type responseRecorder struct {
	header http.Header
	code   int
	body   []byte
}

func (r *responseRecorder) Header() http.Header  { return r.header }
func (r *responseRecorder) WriteHeader(code int) { r.code = code }
func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return len(b), nil
}

// ensure responseRecorder satisfies io.Writer (used internally)
var _ io.Writer = (*responseRecorder)(nil)
