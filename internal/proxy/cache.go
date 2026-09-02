// Package proxy — in-memory response cache keyed by prompt hash.
package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// CacheEntry holds a cached response.
type CacheEntry struct {
	Body      []byte
	Status    int
	ExpiresAt time.Time
}

// ResponseCache is a simple in-memory TTL cache.
type ResponseCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	ttl     time.Duration
	enabled bool
}

// NewResponseCache creates a cache with the given TTL in seconds.
func NewResponseCache(ttlSec int, enabled bool) *ResponseCache {
	c := &ResponseCache{
		entries: make(map[string]*CacheEntry),
		ttl:     time.Duration(ttlSec) * time.Second,
		enabled: enabled,
	}
	if enabled {
		go c.evictLoop()
	}
	return c
}

// HashKey returns a stable cache key from a raw request body.
func HashKey(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// cacheKeyFor scopes an entry to the tenant, resolved provider and model as
// well as the body. Keyed on the body alone, one tenant's completion would
// answer every other tenant that sends the same prompt — a cross-tenant data
// leak — and a model alias remapped mid-flight would keep serving replies
// generated under the old target.
func cacheKeyFor(tenant, provider, model string, body []byte) string {
	h := sha256.New()
	for _, part := range []string{tenant, provider, model} {
		h.Write([]byte(part))
		h.Write([]byte{0}) // unambiguous boundary: none of the parts contain NUL
	}
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns a cached entry if it exists and hasn't expired.
func (c *ResponseCache) Get(key string) (*CacheEntry, bool) {
	if !c.enabled {
		return nil, false
	}
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.ExpiresAt) {
		return nil, false
	}
	return e, true
}

// Set stores a response in the cache.
func (c *ResponseCache) Set(key string, body []byte, status int) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	c.entries[key] = &CacheEntry{
		Body:      body,
		Status:    status,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Purge removes all entries from the cache.
func (c *ResponseCache) Purge() {
	c.mu.Lock()
	c.entries = make(map[string]*CacheEntry)
	c.mu.Unlock()
}

// Stats returns the number of live (non-expired) entries.
func (c *ResponseCache) Stats() (total, live int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	for _, e := range c.entries {
		total++
		if now.Before(e.ExpiresAt) {
			live++
		}
	}
	return
}

// evictLoop runs every minute to remove expired entries.
func (c *ResponseCache) evictLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		c.mu.Lock()
		for k, e := range c.entries {
			if now.After(e.ExpiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}
