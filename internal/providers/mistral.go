package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Mistral struct{ baseURL string }

type mistralTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Mistral) Name() string { return "mistral" }

func (p *Mistral) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("mistral", "https://api.mistral.ai/v1")
}

func (p *Mistral) OwnsModel(model string) bool {
	return strings.Contains(model, "mistral") || strings.Contains(model, "mixtral") || strings.HasPrefix(model, "mistral/")
}

func (p *Mistral) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok mistralTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "mistral"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "mistral"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "mistral", req.Stream)
}

func (p *Mistral) Healthy(ctx context.Context, acc *Account) bool {
	var tok mistralTokens
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
