package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// CloudflareAI routes to Cloudflare Workers AI via the OpenAI-compatible endpoint.
// ponytail: baseURL not used — URL is per-account (includes AccountID), Configure is a no-op.
type CloudflareAI struct{}

type cloudflareTokens struct {
	APIKey    string `json:"api_key"`
	AccountID string `json:"account_id"`
}

func (p *CloudflareAI) Name() string { return "cloudflareai" }

func (p *CloudflareAI) Configure(_ *ConfigStore) {}

func (p *CloudflareAI) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "cf/") || strings.HasPrefix(model, "cloudflare/") || strings.HasPrefix(model, "@cf/")
}

func (p *CloudflareAI) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok cloudflareTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "cloudflareai"}
	}
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/v1/chat/completions", tok.AccountID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "cloudflareai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "cloudflareai", req.Stream)
}

func (p *CloudflareAI) Healthy(ctx context.Context, acc *Account) bool {
	var tok cloudflareTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/v1/models", tok.AccountID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
