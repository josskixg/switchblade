package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Anthropic struct{ baseURL string }

type anthropicTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Anthropic) Name() string { return "anthropic" }

func (p *Anthropic) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("anthropic", "https://api.anthropic.com/v1")
}

func (p *Anthropic) OwnsModel(model string) bool {
	return strings.Contains(model, "claude")
}

func (p *Anthropic) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok anthropicTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 400, Message: "invalid tokens: " + err.Error(), Provider: "anthropic"}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "anthropic"}
	}
	httpReq.Header.Set("x-api-key", tok.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")

	return Exchange(httpReq, "anthropic", req.Stream)
}

func (p *Anthropic) Healthy(ctx context.Context, acc *Account) bool {
	var tok anthropicTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("x-api-key", tok.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
