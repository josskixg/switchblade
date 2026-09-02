package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type cacheEntry struct {
	val     []byte
	expires time.Time
}

// Cache is a simple in-memory TTL cache.
type Cache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]cacheEntry
}

// NewCache creates a Cache with the given TTL in seconds.
func NewCache(ttlSec int) *Cache {
	return &Cache{
		ttl: time.Duration(ttlSec) * time.Second,
		m:   make(map[string]cacheEntry),
	}
}

// Get returns a cached value and whether it was found and still valid.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.val, true
}

// Set stores val under key, overwriting any prior entry.
func (c *Cache) Set(key string, val []byte) {
	c.mu.Lock()
	c.m[key] = cacheEntry{val: val, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// Invalidate removes a single key.
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	delete(c.m, key)
	c.mu.Unlock()
}

// flush removes all entries.
func (c *Cache) flush() {
	c.mu.Lock()
	c.m = make(map[string]cacheEntry)
	c.mu.Unlock()
}

// MountCacheAPI registers cache management endpoints.
func MountCacheAPI(r chi.Router, c *Cache) {
	r.Get("/api/cache/stats", func(w http.ResponseWriter, r *http.Request) {
		c.mu.RLock()
		keys := make([]string, 0, len(c.m))
		for k := range c.m {
			keys = append(keys, k)
		}
		c.mu.RUnlock()
		jsonOK(w, map[string]any{"count": len(keys), "keys": keys})
	})

	r.Delete("/api/cache", func(w http.ResponseWriter, r *http.Request) {
		c.flush()
		jsonOK(w, map[string]string{"status": "flushed"})
	})
}
