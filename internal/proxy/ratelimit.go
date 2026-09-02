// Package proxy — token-bucket rate limiter keyed by client IP or API key.
package proxy

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// bucket is one client's token bucket.
type bucket struct {
	tokens   float64
	lastTick time.Time
}

// RateLimiter is a per-client token bucket rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   float64 // max burst
	enabled bool
}

// NewRateLimiter creates a limiter. rate = requests/sec, burst = max burst size.
func NewRateLimiter(ctx context.Context, ratePerMin, burst int, enabled bool) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    float64(ratePerMin) / 60.0,
		burst:   float64(burst),
		enabled: enabled,
	}
	if enabled {
		go rl.cleanupLoop(ctx)
	}
	return rl
}

// Allow returns true if the given key is within rate limits.
func (rl *RateLimiter) Allow(key string) bool {
	if !rl.enabled {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.burst, lastTick: now}
		rl.buckets[key] = b
	}

	elapsed := now.Sub(b.lastTick).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastTick = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Middleware returns an http.Handler middleware that rate-limits by client IP.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientKey(r)
		if !rl.Allow(key) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientKey(r *http.Request) string {
	if k := r.Header.Get("x-api-key"); k != "" {
		return "key:" + k
	}

	// X-Forwarded-For: first IP = original client
	if xff := r.Header.Get("x-forwarded-for"); xff != "" {
		ip := strings.TrimSpace(strings.Split(xff, ",")[0])
		return "ip:" + stripPort(ip)
	}

	// X-Real-IP
	if xri := r.Header.Get("x-real-ip"); xri != "" {
		return "ip:" + stripPort(xri)
	}

	return "ip:" + stripPort(r.RemoteAddr)
}

func stripPort(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

// cleanupLoop evicts idle buckets every 5 minutes.
func (rl *RateLimiter) cleanupLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("[ratelimit] cleanup stopped")
			return
		case <-t.C:
			rl.mu.Lock()
			for k, b := range rl.buckets {
				if time.Since(b.lastTick) > 10*time.Minute {
					delete(rl.buckets, k)
				}
			}
			rl.mu.Unlock()
		}
	}
}
