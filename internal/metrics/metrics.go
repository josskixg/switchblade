// Package metrics — minimal Prometheus-compatible /metrics endpoint.
package metrics

import (
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"
)

var (
	startTime     = time.Now()
	requestsTotal atomic.Int64
	requestErrors atomic.Int64
	cacheHits     atomic.Int64
	cacheMisses   atomic.Int64
)

// IncRequests increments the total requests counter.
func IncRequests() { requestsTotal.Add(1) }

// IncErrors increments the error counter.
func IncErrors() { requestErrors.Add(1) }

// IncCacheHit increments the cache hit counter.
func IncCacheHit() { cacheHits.Add(1) }

// IncCacheMiss increments the cache miss counter.
func IncCacheMiss() { cacheMisses.Add(1) }

// Handler returns an http.HandlerFunc that serves Prometheus-format metrics.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		uptime := time.Since(startTime).Seconds()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP switchblade_uptime_seconds Uptime in seconds\n")
		fmt.Fprintf(w, "# TYPE switchblade_uptime_seconds gauge\n")
		fmt.Fprintf(w, "switchblade_uptime_seconds %.2f\n\n", uptime)

		fmt.Fprintf(w, "# HELP switchblade_requests_total Total chat requests\n")
		fmt.Fprintf(w, "# TYPE switchblade_requests_total counter\n")
		fmt.Fprintf(w, "switchblade_requests_total %d\n\n", requestsTotal.Load())

		fmt.Fprintf(w, "# HELP switchblade_request_errors_total Total failed requests\n")
		fmt.Fprintf(w, "# TYPE switchblade_request_errors_total counter\n")
		fmt.Fprintf(w, "switchblade_request_errors_total %d\n\n", requestErrors.Load())

		fmt.Fprintf(w, "# HELP switchblade_cache_hits_total Cache hits\n")
		fmt.Fprintf(w, "# TYPE switchblade_cache_hits_total counter\n")
		fmt.Fprintf(w, "switchblade_cache_hits_total %d\n\n", cacheHits.Load())

		fmt.Fprintf(w, "# HELP switchblade_cache_misses_total Cache misses\n")
		fmt.Fprintf(w, "# TYPE switchblade_cache_misses_total counter\n")
		fmt.Fprintf(w, "switchblade_cache_misses_total %d\n\n", cacheMisses.Load())

		fmt.Fprintf(w, "# HELP switchblade_memory_alloc_bytes Current heap allocation\n")
		fmt.Fprintf(w, "# TYPE switchblade_memory_alloc_bytes gauge\n")
		fmt.Fprintf(w, "switchblade_memory_alloc_bytes %d\n\n", mem.Alloc)

		fmt.Fprintf(w, "# HELP switchblade_goroutines Number of goroutines\n")
		fmt.Fprintf(w, "# TYPE switchblade_goroutines gauge\n")
		fmt.Fprintf(w, "switchblade_goroutines %d\n", runtime.NumGoroutine())
	}
}
