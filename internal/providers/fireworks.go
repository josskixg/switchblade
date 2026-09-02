package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Fireworks struct{ baseURL string }

type fireworksTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Fireworks) Name() string { return "fireworks" }

func (p *Fireworks) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("fireworks", "https://api.fireworks.ai/inference/v1")
}

func (p *Fireworks) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "fireworks/") || strings.HasPrefix(model, "accounts/fireworks/")
}

func (p *Fireworks) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok fireworksTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "fireworks"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "fireworks"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "fireworks", req.Stream)
}

func (p *Fireworks) Healthy(ctx context.Context, acc *Account) bool {
	var tok fireworksTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
