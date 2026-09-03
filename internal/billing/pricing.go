package billing

import (
	"database/sql"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

// Price is the per-model rate card, stored in the model_pricing table.
type Price struct {
	Model string
	// InputNanoPerMTok is the cost in Nano of one million prompt tokens.
	InputNanoPerMTok int64
	// OutputNanoPerMTok is the cost in Nano of one million completion tokens.
	OutputNanoPerMTok int64
	// MarginBPS is the markup applied on top of upstream cost, in basis points.
	// 2000 = +20%. This is where the business makes money.
	MarginBPS int64
}

// Cost returns the billable amount for u under this price, margin included.
func (p Price) Cost(u Usage) Nano {
	in := int64(u.PromptTokens) * p.InputNanoPerMTok / tokensPerMTok
	out := int64(u.CompletionTokens) * p.OutputNanoPerMTok / tokensPerMTok
	total := in + out
	if p.MarginBPS != 0 {
		total = total * (bpsScale + p.MarginBPS) / bpsScale
	}
	return Nano(total)
}

// PricingCache holds the rate card in memory, refreshed on a TTL so operators
// can edit prices from the dashboard without a restart.
type PricingCache struct {
	db  *sql.DB
	ttl time.Duration

	mu       sync.RWMutex
	exact    map[string]Price
	prefixes []Price // longest model name first, for versioned-model fallback
	loadedAt time.Time
}

// NewPricingCache returns a cache that reloads at most every 30 seconds.
func NewPricingCache(db *sql.DB) *PricingCache {
	return &PricingCache{db: db, ttl: 30 * time.Second, exact: map[string]Price{}}
}

// Lookup resolves a model to its price. An exact match wins; otherwise the
// longest configured model name that prefixes the request is used, so
// "gpt-4o-mini-2024-07-18" bills at the "gpt-4o-mini" rate without every dated
// snapshot needing its own row.
func (c *PricingCache) Lookup(model string) (Price, bool) {
	c.maybeReload()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if p, ok := c.exact[model]; ok {
		return p, true
	}
	for _, p := range c.prefixes {
		if strings.HasPrefix(model, p.Model) {
			return p, true
		}
	}
	return Price{}, false
}

// Cost prices usage for a model. The bool reports whether a rate card existed —
// an unpriced model is recorded at zero cost rather than silently billed wrong.
func (c *PricingCache) Cost(model string, u Usage) (Nano, bool) {
	p, ok := c.Lookup(model)
	if !ok {
		return 0, false
	}
	return p.Cost(u), true
}

func (c *PricingCache) maybeReload() {
	c.mu.RLock()
	fresh := time.Since(c.loadedAt) < c.ttl
	c.mu.RUnlock()
	if fresh {
		return
	}

	rows, err := c.db.Query(`
		SELECT model, input_nano_per_mtok, output_nano_per_mtok, margin_bps
		FROM model_pricing WHERE enabled = 1`)
	if err != nil {
		slog.Warn("[billing] pricing reload", "err", err)
		// Keep serving the previous rate card; back off so a broken DB does not
		// turn into a query storm.
		c.mu.Lock()
		c.loadedAt = time.Now()
		c.mu.Unlock()
		return
	}
	defer rows.Close()

	exact := make(map[string]Price)
	var prefixes []Price
	for rows.Next() {
		var p Price
		if err := rows.Scan(&p.Model, &p.InputNanoPerMTok, &p.OutputNanoPerMTok, &p.MarginBPS); err != nil {
			continue
		}
		exact[p.Model] = p
		prefixes = append(prefixes, p)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i].Model) > len(prefixes[j].Model)
	})

	c.mu.Lock()
	c.exact = exact
	c.prefixes = prefixes
	c.loadedAt = time.Now()
	c.mu.Unlock()
}
