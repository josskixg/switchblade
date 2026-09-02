// Package db — async request logging and usage summary.
package db

import (
	"encoding/json"
	"log"
	"time"
)

// RequestLog is the data for one logged request.
type RequestLog struct {
	AccountID        int64
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CreditsUsed      float64
	Status           string // success | error | timeout
	DurationMS       int64
	ErrorMessage     string
	RequestBody      string
	ResponseBody     string
	AccountEmail     string
	QuotaBefore      float64
	QuotaAfter       float64
	CompressionStats string // JSON
	FallbackChain    string // JSON array
}

// LogRequest inserts a request log row asynchronously.
// Non-blocking — fires and forgets in a goroutine.
func (db *DB) LogRequest(entry RequestLog) {
	go func() {
		_, err := db.Exec(`
			INSERT INTO request_logs (
				account_id, provider, model,
				prompt_tokens, completion_tokens, total_tokens, credits_used,
				status, duration_ms, error_message,
				request_body, response_body,
				account_email, account_quota_before, account_quota_after,
				compression_stats, fallback_chain,
				created_at
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			entry.AccountID, entry.Provider, entry.Model,
			entry.PromptTokens, entry.CompletionTokens, entry.TotalTokens, entry.CreditsUsed,
			entry.Status, entry.DurationMS, entry.ErrorMessage,
			entry.RequestBody, entry.ResponseBody,
			entry.AccountEmail, entry.QuotaBefore, entry.QuotaAfter,
			entry.CompressionStats, entry.FallbackChain,
			time.Now().Unix(),
		)
		if err != nil {
			log.Printf("[db] LogRequest: %v", err)
		}
	}()
}

// UpsertUsageSummary increments the hourly usage_summary bucket.
func (db *DB) UpsertUsageSummary(provider, model string, promptTok, completionTok, totalTok int, credits float64) {
	go func() {
		bucket := time.Now().UTC().Format("2006-01-02-15") // hourly bucket
		_, err := db.Exec(`
			INSERT INTO usage_summary (bucket, provider, model, prompt_tokens, completion_tokens, total_tokens, credits_used, request_count)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1)
			ON CONFLICT(bucket, provider, model) DO UPDATE SET
				prompt_tokens     = prompt_tokens     + excluded.prompt_tokens,
				completion_tokens = completion_tokens + excluded.completion_tokens,
				total_tokens      = total_tokens      + excluded.total_tokens,
				credits_used      = credits_used      + excluded.credits_used,
				request_count     = request_count     + 1`,
			bucket, provider, model, promptTok, completionTok, totalTok, credits,
		)
		if err != nil {
			log.Printf("[db] UpsertUsageSummary: %v", err)
		}
	}()
}

// GetRecentLogs returns the N most recent request_logs rows.
func (db *DB) GetRecentLogs(limit int) ([]map[string]any, error) {
	rows, err := db.Query(`
		SELECT id, provider, model, status, duration_ms, total_tokens, error_message, created_at
		FROM request_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id, durationMS, totalTokens, createdAt int64
		var provider, model, status string
		var errorMsg *string
		if err := rows.Scan(&id, &provider, &model, &status, &durationMS, &totalTokens, &errorMsg, &createdAt); err != nil {
			continue
		}
		row := map[string]any{
			"id": id, "provider": provider, "model": model,
			"status": status, "duration_ms": durationMS,
			"total_tokens": totalTokens, "created_at": createdAt,
		}
		if errorMsg != nil {
			row["error_message"] = *errorMsg
		}
		results = append(results, row)
	}
	return results, nil
}

// GetUsageStats returns the usage_summary aggregated by provider+model.
func (db *DB) GetUsageStats(since string) ([]map[string]any, error) {
	rows, err := db.Query(`
		SELECT provider, model,
			SUM(prompt_tokens), SUM(completion_tokens), SUM(total_tokens),
			SUM(credits_used), SUM(request_count)
		FROM usage_summary WHERE bucket >= ?
		GROUP BY provider, model ORDER BY SUM(total_tokens) DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var provider, model string
		var pt, ct, tt, rc int64
		var credits float64
		if err := rows.Scan(&provider, &model, &pt, &ct, &tt, &credits, &rc); err != nil {
			continue
		}
		results = append(results, map[string]any{
			"provider": provider, "model": model,
			"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": tt,
			"credits_used": credits, "request_count": rc,
		})
	}
	return results, nil
}

// jsonStr marshals v to a JSON string, returns "{}" on error.
func jsonStr(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

var _ = jsonStr // exported for use in other packages via db.jsonStr if needed
