// Package api — webhook delivery system.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"switchblade/internal/db"
	"switchblade/internal/ssrf"
)

// WebhookEvent is the payload sent to configured webhook URLs.
type WebhookEvent struct {
	Event     string         `json:"event"`
	Timestamp int64          `json:"timestamp"`
	Data      map[string]any `json:"data"`
}

// FireWebhooks loads active webhooks from DB and delivers event to each.
// Runs asynchronously — caller does not wait.
func FireWebhooks(ctx context.Context, database *db.DB, event string, data map[string]any) {
	go func() {
		rows, err := database.Query(
			`SELECT id, url, secret FROM webhook_logs WHERE event_type = ? AND status = 'active'`,
			event,
		)
		if err != nil {
			return
		}
		defer rows.Close()

		payload := WebhookEvent{
			Event:     event,
			Timestamp: time.Now().Unix(),
			Data:      data,
		}
		body, _ := json.Marshal(payload)

		for rows.Next() {
			var id int64
			var url, secret string
			if err := rows.Scan(&id, &url, &secret); err != nil {
				continue
			}
			go deliver(ctx, database, id, url, secret, body)
		}
	}()
}

func deliver(ctx context.Context, database *db.DB, webhookID int64, url, secret string, body []byte) {
	const maxAttempts = 3
	if err := ssrf.ValidateURL(url); err != nil {
		log.Printf("[webhook] SSRF validation rejected URL %s: %v", url, err)
		return
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			log.Printf("[webhook] delivery cancelled")
			return
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			break
		}
		req.Header.Set("Content-Type", "application/json")
		if secret != "" {
			req.Header.Set("X-Webhook-Secret", secret)
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		status := "delivered"
		errMsg := ""
		if err != nil {
			status = "failed"
			errMsg = err.Error()
		} else {
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				status = "failed"
				errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
		}
		// log delivery attempt
		_, _ = database.Exec(
			`INSERT INTO webhook_logs (event_type, url, status, error_message, attempt, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			"delivery", url, status, errMsg, attempt, time.Now().Unix(),
		)
		if status == "delivered" {
			return
		}
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt*attempt) * time.Second) // 1s, 4s backoff
		}
	}
	log.Printf("[webhook] delivery failed after %d attempts: %s", maxAttempts, url)
}
