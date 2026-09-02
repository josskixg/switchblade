package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type EdenAI struct{ baseURL string }

type edenaiTokens struct {
	APIKey string `json:"api_key"`
}

func (p *EdenAI) Name() string { return "edenai" }

func (p *EdenAI) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("edenai", "https://api.edenai.run/v2")
}

func (p *EdenAI) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "edenai/")
}

func (p *EdenAI) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok edenaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "edenai"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "edenai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "edenai", req.Stream)
}

func (p *EdenAI) Healthy(ctx context.Context, acc *Account) bool {
	var tok edenaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/info/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
