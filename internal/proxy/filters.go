// Package proxy — PUDIDIL content filter engine.
// Strips Claude/Anthropic identity markers and custom filter rules from request bodies.
package proxy

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"time"

	"switchblade/internal/db"
)

// hardcoded patterns that must always be stripped (Layer 1)
var hardcodedPatterns = []string{
	"cc_entrypoint",
	"cc_version",
	"<cc_identity>",
	"</cc_identity>",
}

var cchHashRe = regexp.MustCompile(`cch=[a-f0-9]{8,}`)

// FilterRule is one DB-backed rule.
type FilterRule struct {
	Pattern     string
	Replacement string
	IsRegex     bool
	compiled    *regexp.Regexp
}

// ProviderJailbreak stores provider-specific jailbreak override properties.
type ProviderJailbreak struct {
	Enabled bool
	Prompt  string
}

// Filters holds the PUDIDIL engine state.
type Filters struct {
	mu                 sync.RWMutex
	rules              []FilterRule
	jailbreakPrompt    string
	jailbreakEnabled   bool
	providerJailbreaks map[string]ProviderJailbreak
	cachedAt           time.Time
	ttl                time.Duration
	db                 *db.DB
}

// NewFilters creates a filter engine with a 5s rule reload TTL.
func NewFilters(database *db.DB) *Filters {
	return &Filters{
		ttl: 5 * time.Second,
		db:  database,
	}
}

// Apply runs the filter pipeline on a raw request body and returns the cleaned body.
func (f *Filters) Apply(body []byte) []byte {
	s := string(body)

	// Layer 1: hardcoded patterns
	for _, p := range hardcodedPatterns {
		s = strings.ReplaceAll(s, p, "")
	}
	s = cchHashRe.ReplaceAllString(s, "")

	// Layer 2: DB-backed rules (hot-reloaded)
	rules := f.getRules()
	for _, rule := range rules {
		if rule.IsRegex && rule.compiled != nil {
			s = rule.compiled.ReplaceAllString(s, rule.Replacement)
		} else {
			s = strings.ReplaceAll(s, rule.Pattern, rule.Replacement)
		}
	}

	return []byte(s)
}

// ApplyToMessages applies filters to the messages array in a request body.
// More targeted than Apply — only touches message content strings.
func (f *Filters) ApplyToMessages(body []byte, providerName string) []byte {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return f.Apply(body) // fallback: apply to whole body
	}
	msgsRaw, ok := req["messages"]
	if !ok {
		return body
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(msgsRaw, &messages); err != nil {
		return body
	}

	// Trigger reload check
	_ = f.getRules()

	f.mu.RLock()
	jbEnabled := f.jailbreakEnabled
	jbPrompt := f.jailbreakPrompt
	// Resolve provider-specific override first if providerName is set
	if providerName != "" {
		if pj, ok := f.providerJailbreaks[providerName]; ok {
			jbEnabled = pj.Enabled
			jbPrompt = pj.Prompt
		}
	}
	f.mu.RUnlock()

	changed := false

	// Inject / Override jailbreak system prompt first
	if jbEnabled && jbPrompt != "" {
		foundSystem := false
		for i, msg := range messages {
			var role string
			if json.Unmarshal(msg["role"], &role) == nil && role == "system" {
				var content string
				if json.Unmarshal(msg["content"], &content) == nil {
					newContent := jbPrompt + "\n\n" + content
					contentRaw, err := json.Marshal(newContent)
					if err == nil {
						messages[i]["content"] = contentRaw
						foundSystem = true
						changed = true
						break
					}
				}
			}
		}
		if !foundSystem {
			roleRaw, _ := json.Marshal("system")
			contentRaw, _ := json.Marshal(jbPrompt)
			newSystemMsg := map[string]json.RawMessage{
				"role":    roleRaw,
				"content": contentRaw,
			}
			messages = append([]map[string]json.RawMessage{newSystemMsg}, messages...)
			changed = true
		}
	}

	for i, msg := range messages {
		// Apply safety filter replacements to content
		contentRaw, ok := msg["content"]
		if !ok {
			continue
		}
		var content string
		if json.Unmarshal(contentRaw, &content) != nil {
			continue
		}
		filtered := string(f.Apply([]byte(content)))
		if filtered != content {
			newContent, err := json.Marshal(filtered)
			if err == nil {
				messages[i]["content"] = newContent
				changed = true
			}
		}
	}

	if !changed {
		return body
	}
	newMsgs, err := json.Marshal(messages)
	if err != nil {
		return body
	}
	req["messages"] = newMsgs
	out, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return out
}

func (f *Filters) getRules() []FilterRule {
	f.mu.RLock()
	if time.Since(f.cachedAt) < f.ttl {
		rules := f.rules
		f.mu.RUnlock()
		return rules
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()
	if time.Since(f.cachedAt) < f.ttl {
		return f.rules
	}
	f.reload()
	return f.rules
}

func (f *Filters) reload() {
	f.rules = nil
	rows, err := f.db.Query(
		`SELECT pattern, replacement, is_regex FROM filter_rules WHERE is_active = 1 ORDER BY sort_order`,
	)
	if err == nil {
		defer rows.Close()
		var rules []FilterRule
		for rows.Next() {
			var r FilterRule
			var isRegex int
			if err := rows.Scan(&r.Pattern, &r.Replacement, &isRegex); err != nil {
				continue
			}
			r.IsRegex = isRegex == 1
			if r.IsRegex {
				if re, err := regexp.Compile(r.Pattern); err == nil {
					r.compiled = re
				} else {
					continue // skip broken regex
				}
			}
			rules = append(rules, r)
		}
		f.rules = rules
	}

	// Load global jailbreak configuration
	var enabledVal string
	_ = f.db.QueryRow(`SELECT value FROM settings WHERE key = 'jailbreak_enabled'`).Scan(&enabledVal)
	f.jailbreakEnabled = enabledVal == "true"

	var promptVal string
	_ = f.db.QueryRow(`SELECT value FROM settings WHERE key = 'jailbreak_prompt'`).Scan(&promptVal)
	f.jailbreakPrompt = promptVal

	// Load provider-specific jailbreak configuration overrides
	f.providerJailbreaks = make(map[string]ProviderJailbreak)
	pcRows, err := f.db.Query(`SELECT provider, extra FROM provider_config WHERE enabled = 1`)
	if err == nil {
		defer pcRows.Close()
		for pcRows.Next() {
			var provider, extraStr string
			if err := pcRows.Scan(&provider, &extraStr); err == nil && extraStr != "" && extraStr != "{}" {
				var extra struct {
					JailbreakEnabled bool   `json:"jailbreak_enabled"`
					JailbreakPrompt  string `json:"jailbreak_prompt"`
				}
				if json.Unmarshal([]byte(extraStr), &extra) == nil {
					if extra.JailbreakEnabled && extra.JailbreakPrompt != "" {
						f.providerJailbreaks[provider] = ProviderJailbreak{
							Enabled: extra.JailbreakEnabled,
							Prompt:  extra.JailbreakPrompt,
						}
					}
				}
			}
		}
	}

	f.cachedAt = time.Now()
}
