package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type DeepInfra struct{ baseURL string }

type deepinfraTokens struct {
	APIKey string `json:"api_key"`
}

func (p *DeepInfra) Name() string { return "deepinfra" }

func (p *DeepInfra) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("deepinfra", "https://api.deepinfra.com/v1/openai")
}

func (p *DeepInfra) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "deepinfra/")
}

func (p *DeepInfra) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok deepinfraTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "deepinfra"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "deepinfra"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "deepinfra", req.Stream)
}

func (p *DeepInfra) Healthy(ctx context.Context, acc *Account) bool {
	var tok deepinfraTokens
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
