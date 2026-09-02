package providers

import (
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// ProviderConfig holds DB-driven config for one provider.
type ProviderConfig struct {
	Provider string
	BaseURL  string
	Models   string
	Enabled  bool
	Extra    string
}

// Configurable is implemented by providers that accept a ConfigStore.
type Configurable interface {
	Configure(store *ConfigStore)
}

// ConfigStore hot-reloads provider config from DB every 30s.
type ConfigStore struct {
	mu   sync.RWMutex
	data map[string]ProviderConfig
	db   *sql.DB
	stop chan struct{}
}

// NewConfigStore loads immediately then starts a 30s background refresh.
func NewConfigStore(db *sql.DB) *ConfigStore {
	cs := &ConfigStore{
		data: make(map[string]ProviderConfig),
		db:   db,
		stop: make(chan struct{}),
	}
	_ = cs.load() // ponytail: ignore startup error, providers fall back to hardcoded defaults
	go cs.loop()
	return cs
}

func (cs *ConfigStore) loop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			_ = cs.load()
		case <-cs.stop:
			return
		}
	}
}

func (cs *ConfigStore) load() error {
	rows, err := cs.db.Query(`SELECT provider, base_url, models, enabled, extra FROM provider_config`)
	if err != nil {
		return err
	}
	defer rows.Close()

	m := make(map[string]ProviderConfig)
	for rows.Next() {
		var c ProviderConfig
		var enabled int
		if err := rows.Scan(&c.Provider, &c.BaseURL, &c.Models, &enabled, &c.Extra); err != nil {
			continue
		}
		c.Enabled = enabled != 0
		m[c.Provider] = c
	}

	cs.mu.Lock()
	cs.data = m
	cs.mu.Unlock()
	return rows.Err()
}

// Get returns config for a provider, zero value if not in DB.
func (cs *ConfigStore) Get(provider string) ProviderConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.data[provider]
}

// GetBaseURL returns DB value if non-empty, else defaultURL.
func (cs *ConfigStore) GetBaseURL(provider, defaultURL string) string {
	if u := cs.Get(provider).BaseURL; u != "" {
		return u
	}
	return defaultURL
}

// Models returns the configured model IDs. JSON arrays are canonical; comma-separated
// values remain accepted for databases created before the provider config API used JSON.
func (cs *ConfigStore) Models(provider string) ([]string, bool) {
	c := cs.Get(provider)
	if c.Provider == "" {
		return nil, false
	}
	var models []string
	if json.Unmarshal([]byte(c.Models), &models) != nil {
		models = strings.Split(c.Models, ",")
	}
	clean := models[:0]
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			clean = append(clean, model)
		}
	}
	return clean, true
}

// Stop cancels the background refresh goroutine.
func (cs *ConfigStore) Stop() {
	close(cs.stop)
}
