package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type OpenRouter struct{ baseURL string }

type openrouterTokens struct {
	APIKey string `json:"api_key"`
}

func (p *OpenRouter) Name() string { return "openrouter" }

func (p *OpenRouter) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("openrouter", "https://openrouter.ai/api/v1")
}

func (p *OpenRouter) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "openrouter/") || strings.Contains(model, "/")
}

func (p *OpenRouter) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok openrouterTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "openrouter"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openrouter"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "openrouter", req.Stream)
}

func (p *OpenRouter) Healthy(ctx context.Context, acc *Account) bool {
	var tok openrouterTokens
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
