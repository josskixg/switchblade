package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Anyscale struct{ baseURL string }

type anyscaleTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Anyscale) Name() string { return "anyscale" }

func (p *Anyscale) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("anyscale", "https://api.endpoints.anyscale.com/v1")
}

func (p *Anyscale) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "anyscale/")
}

func (p *Anyscale) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok anyscaleTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "anyscale"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "anyscale"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "anyscale", req.Stream)
}

func (p *Anyscale) Healthy(ctx context.Context, acc *Account) bool {
	var tok anyscaleTokens
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
