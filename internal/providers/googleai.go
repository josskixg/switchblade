package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// GoogleAI wraps Google AI Studio (Gemini) via OpenAI-compatible endpoint.
type GoogleAI struct{ baseURL string }

type googleaiTokens struct {
	APIKey string `json:"api_key"`
}

func (p *GoogleAI) Name() string { return "googleai" }

func (p *GoogleAI) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("googleai", "https://generativelanguage.googleapis.com/v1beta/openai")
}

func (p *GoogleAI) OwnsModel(model string) bool {
	return strings.Contains(model, "gemini") || strings.HasPrefix(model, "google/") || strings.HasPrefix(model, "googleai/")
}

func (p *GoogleAI) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok googleaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "googleai"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "googleai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "googleai", req.Stream)
}

func (p *GoogleAI) Healthy(ctx context.Context, acc *Account) bool {
	var tok googleaiTokens
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
