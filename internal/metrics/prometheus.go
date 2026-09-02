package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// Metrics holds named atomic counters for caller-managed tracking.
type Metrics struct {
	RequestsTotal   int64
	RequestsSuccess int64
	RequestsError   int64
	TokensTotal     int64
	LatencySum      int64 // nanoseconds
}

// NewMetrics returns a new zero-value Metrics.
func NewMetrics() *Metrics { return &Metrics{} }

// Inc atomically adds 1 to field.
func Inc(field *int64) { atomic.AddInt64(field, 1) }

// Add atomically adds n to field.
func Add(field *int64, n int64) { atomic.AddInt64(field, n) }

// Snapshot returns a copy of all counters.
func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"requests_total":   atomic.LoadInt64(&m.RequestsTotal),
		"requests_success": atomic.LoadInt64(&m.RequestsSuccess),
		"requests_error":   atomic.LoadInt64(&m.RequestsError),
		"tokens_total":     atomic.LoadInt64(&m.TokensTotal),
		"latency_sum_ns":   atomic.LoadInt64(&m.LatencySum),
	}
}

// ServeHTTP exposes the Metrics instance as plain text (key=value per line).
func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for k, v := range m.Snapshot() {
		fmt.Fprintf(w, "%s=%d\n", k, v)
	}
}
