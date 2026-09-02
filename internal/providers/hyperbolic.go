package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Hyperbolic struct{ baseURL string }

type hyperbolicTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Hyperbolic) Name() string { return "hyperbolic" }

func (p *Hyperbolic) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("hyperbolic", "https://api.hyperbolic.xyz/v1")
}

func (p *Hyperbolic) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "hyperbolic/")
}

func (p *Hyperbolic) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok hyperbolicTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "hyperbolic"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "hyperbolic"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "hyperbolic", req.Stream)
}

func (p *Hyperbolic) Healthy(ctx context.Context, acc *Account) bool {
	var tok hyperbolicTokens
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
