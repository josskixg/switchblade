package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Cohere struct{ baseURL string }

type cohereTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Cohere) Name() string { return "cohere" }

func (p *Cohere) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("cohere", "https://api.cohere.com/compatibility/v1")
}

func (p *Cohere) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "cohere/") || strings.HasPrefix(model, "command")
}

func (p *Cohere) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok cohereTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "cohere"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "cohere"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "cohere", req.Stream)
}

func (p *Cohere) Healthy(ctx context.Context, acc *Account) bool {
	var tok cohereTokens
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
