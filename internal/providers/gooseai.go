package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type GooseAI struct{ baseURL string }

type gooseaiTokens struct {
	APIKey string `json:"api_key"`
}

func (p *GooseAI) Name() string { return "gooseai" }

func (p *GooseAI) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("gooseai", "https://api.goose.ai/v1")
}

func (p *GooseAI) OwnsModel(model string) bool {
	for _, s := range []string{"gooseai/", "goose-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *GooseAI) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok gooseaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "gooseai"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "gooseai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "gooseai", req.Stream)
}

func (p *GooseAI) Healthy(ctx context.Context, acc *Account) bool {
	var tok gooseaiTokens
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
