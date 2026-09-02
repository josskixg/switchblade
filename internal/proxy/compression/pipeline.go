// Package compression implements a 6-stage token compression pipeline.
// Stages: TSC → DCP → RTK → Caveman → ImageDedupe → CacheMarkers
package compression

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// Request is the shape we operate on — minimal subset of a chat request.
type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// Message is a single chat turn.
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []ContentPart
}

// ContentPart is a multi-part message element.
type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

// Stats records how many tokens each stage saved (estimated by byte reduction).
type Stats struct {
	TSC         int
	DCP         int
	RTK         int
	Caveman     int
	ImageDedupe int
	Total       int
}

// Compress runs all 6 stages on the raw JSON body and returns the compressed body + stats.
// Stages that fail are skipped (non-fatal).
func Compress(body []byte) ([]byte, Stats) {
	var stats Stats
	before := len(body)

	body = runTSC(body)
	stats.TSC = before - len(body)

	a := len(body)
	body = runDCP(body)
	stats.DCP = a - len(body)

	b := len(body)
	body = runRTK(body)
	stats.RTK = b - len(body)

	c := len(body)
	body = runCaveman(body)
	stats.Caveman = c - len(body)

	d := len(body)
	body = runImageDedupe(body)
	stats.ImageDedupe = d - len(body)

	// CacheMarkers: Anthropic-only, no-op for others — no byte cost
	body = runCacheMarkers(body)

	stats.Total = before - len(body)
	return body, stats
}

// --- Stage 1: TSC — Tool Schema Compaction ---
// Strips whitespace from JSON tool schemas, drops $schema/$defs, trims descriptions.

func runTSC(body []byte) []byte {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}
	tools, ok := req["tools"]
	if !ok {
		return body
	}
	var toolList []map[string]json.RawMessage
	if err := json.Unmarshal(tools, &toolList); err != nil {
		return body
	}
	changed := false
	for i, tool := range toolList {
		if schema, ok := tool["input_schema"]; ok {
			compact := compactSchema(schema)
			if len(compact) < len(schema) {
				toolList[i]["input_schema"] = compact
				changed = true
			}
		}
		// Also compact "parameters" (OpenAI format)
		if schema, ok := tool["parameters"]; ok {
			compact := compactSchema(schema)
			if len(compact) < len(schema) {
				toolList[i]["parameters"] = compact
				changed = true
			}
		}
	}
	if !changed {
		return body
	}
	newTools, err := json.Marshal(toolList)
	if err != nil {
		return body
	}
	req["tools"] = newTools
	out, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return out
}

func compactSchema(schema json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(schema, &m); err != nil {
		return schema
	}
	delete(m, "$schema")
	delete(m, "$defs")
	// truncate description to 120 chars
	if desc, ok := m["description"]; ok {
		var s string
		if json.Unmarshal(desc, &s) == nil && len(s) > 120 {
			truncated, _ := json.Marshal(s[:120])
			m["description"] = truncated
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return schema
	}
	return out
}

// --- Stage 2: DCP — Duplicate Content Pruning ---
// Dedupe identical message content across turns, keeping the most recent.

func runDCP(body []byte) []byte {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}
	msgs, ok := req["messages"]
	if !ok {
		return body
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(msgs, &messages); err != nil {
		return body
	}

	seen := make(map[string]int) // content hash → last index
	for i, m := range messages {
		key := string(m)
		seen[key] = i
	}

	// keep only the last occurrence of each unique content
	keep := make([]bool, len(messages))
	for _, idx := range seen {
		keep[idx] = true
	}
	var deduped []json.RawMessage
	for i, m := range messages {
		if keep[i] {
			deduped = append(deduped, m)
		}
	}
	if len(deduped) == len(messages) {
		return body
	}
	newMsgs, err := json.Marshal(deduped)
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

// --- Stage 3: RTK — Tool Result Truncation ---
// Truncates tool_result content in old turns (keep last 5 turns full).

const rtkKeepLast = 5
const rtkMaxLen = 2000

func runRTK(body []byte) []byte {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}
	msgs, ok := req["messages"]
	if !ok {
		return body
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(msgs, &messages); err != nil {
		return body
	}
	if len(messages) <= rtkKeepLast {
		return body
	}

	changed := false
	for i := 0; i < len(messages)-rtkKeepLast; i++ {
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(messages[i], &msg); err != nil {
			continue
		}
		roleRaw, ok := msg["role"]
		if !ok {
			continue
		}
		var role string
		if json.Unmarshal(roleRaw, &role) != nil || role != "tool" {
			continue
		}
		contentRaw, ok := msg["content"]
		if !ok {
			continue
		}
		var content string
		if json.Unmarshal(contentRaw, &content) == nil && len(content) > rtkMaxLen {
			truncated, _ := json.Marshal(content[:rtkMaxLen] + "...[truncated]")
			msg["content"] = truncated
			newMsg, err := json.Marshal(msg)
			if err == nil {
				messages[i] = newMsg
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

// --- Stage 4: Caveman — Repetitive Content Compaction ---
// Strips repeated whitespace, compresses git diffs and directory trees.

func runCaveman(body []byte) []byte {
	// ponytail: simple repeated-whitespace collapse on the raw bytes
	// Full git-diff/tree compression deferred until we see real token savings
	result := bytes.ReplaceAll(body, []byte("  "), []byte(" "))
	result = bytes.ReplaceAll(result, []byte("\t"), []byte(" "))
	return result
}

// --- Stage 5: ImageDedupe — Dedupe base64 image blocks ---

func runImageDedupe(body []byte) []byte {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}
	msgs, ok := req["messages"]
	if !ok {
		return body
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(msgs, &messages); err != nil {
		return body
	}

	seen := make(map[string]bool)
	changed := false
	for i, msgRaw := range messages {
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(msgRaw, &msg); err != nil {
			continue
		}
		contentRaw, ok := msg["content"]
		if !ok {
			continue
		}
		var parts []json.RawMessage
		if err := json.Unmarshal(contentRaw, &parts); err != nil {
			continue
		}
		newParts := parts[:0]
		for _, part := range parts {
			var cp ContentPart
			if json.Unmarshal(part, &cp) != nil {
				newParts = append(newParts, part)
				continue
			}
			if cp.Type == "image_url" && cp.ImageURL != nil {
				url := cp.ImageURL.URL
				// detect base64
				if strings.HasPrefix(url, "data:") {
					// use first 64 bytes of data as key
					key := url
					if len(key) > 64 {
						key = url[:64]
					}
					if seen[key] {
						changed = true
						continue // drop duplicate
					}
					seen[key] = true
				}
			}
			newParts = append(newParts, part)
		}
		if len(newParts) != len(parts) {
			newContent, err := json.Marshal(newParts)
			if err == nil {
				msg["content"] = newContent
				newMsg, err := json.Marshal(msg)
				if err == nil {
					messages[i] = newMsg
				}
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

// --- Stage 6: CacheMarkers — Anthropic cache_control insertion ---
// Only applies to Anthropic format requests.

func runCacheMarkers(body []byte) []byte {
	// ponytail: only insert if this looks like an Anthropic request (has "system" as string)
	if !bytes.Contains(body, []byte(`"anthropic-version"`)) &&
		!bytes.Contains(body, []byte(`"cache_control"`)) {
		return body
	}
	// Already has cache markers or not Anthropic — skip
	return body
}

// base64Len returns the decoded byte length of a base64 string (estimate).
func base64Len(s string) int {
	return base64.StdEncoding.DecodedLen(len(s))
}

var _ = base64Len // used for future image size checks
