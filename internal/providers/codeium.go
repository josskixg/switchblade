package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Codeium struct{ baseURL string }

type codeiumTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Codeium) Name() string { return "codeium" }

func (p *Codeium) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("codeium", "https://api.codeium.com/v1")
}

func (p *Codeium) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "codeium/")
}

func (p *Codeium) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok codeiumTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "codeium"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "codeium"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "codeium", req.Stream)
}

func (p *Codeium) Healthy(ctx context.Context, acc *Account) bool {
	var tok codeiumTokens
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
