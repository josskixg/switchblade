// Package proxy — model alias resolution (DB-backed, cached 10s).
package proxy

import (
	"sync"
	"time"

	"switchblade/internal/db"
)

// ModelMapper resolves model aliases from DB with a short TTL cache.
type ModelMapper struct {
	mu       sync.RWMutex
	cache    map[string]string
	cachedAt time.Time
	ttl      time.Duration
	db       *db.DB
}

// NewModelMapper creates a mapper with a 10s cache TTL.
func NewModelMapper(database *db.DB) *ModelMapper {
	return &ModelMapper{
		cache: make(map[string]string),
		ttl:   10 * time.Second,
		db:    database,
	}
}

// Resolve returns the canonical model name for the given alias.
// Returns the input unchanged if no mapping exists.
func (m *ModelMapper) Resolve(model string) string {
	m.mu.RLock()
	if time.Since(m.cachedAt) < m.ttl {
		if mapped, ok := m.cache[model]; ok {
			m.mu.RUnlock()
			return mapped
		}
		m.mu.RUnlock()
		return model
	}
	m.mu.RUnlock()

	// Reload
	m.mu.Lock()
	defer m.mu.Unlock()
	// Double-check after acquiring write lock
	if time.Since(m.cachedAt) < m.ttl {
		if mapped, ok := m.cache[model]; ok {
			return mapped
		}
		return model
	}
	m.reload()
	if mapped, ok := m.cache[model]; ok {
		return mapped
	}
	return model
}

func (m *ModelMapper) reload() {
	rows, err := m.db.Query(
		`SELECT source_pattern, target_model FROM model_mappings WHERE enabled = 1 AND match_type = 'exact' ORDER BY priority ASC`,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	newCache := make(map[string]string)
	for rows.Next() {
		var source, target string
		if err := rows.Scan(&source, &target); err == nil {
			newCache[source] = target
		}
	}
	m.cache = newCache
	m.cachedAt = time.Now()
}
