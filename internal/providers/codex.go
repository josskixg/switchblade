package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Codex wraps OpenAI Codex (GPT-5) via standard OpenAI API.
type Codex struct{ baseURL string }

type codexTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Codex) Name() string { return "codex" }

func (p *Codex) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("codex", "https://api.openai.com/v1")
}

func (p *Codex) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "gpt-5-codex") || strings.HasPrefix(model, "codex/")
}

func (p *Codex) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok codexTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "codex"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "codex"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "codex", req.Stream)
}

func (p *Codex) Healthy(ctx context.Context, acc *Account) bool {
	var tok codexTokens
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
