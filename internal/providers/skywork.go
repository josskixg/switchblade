package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Skywork struct{}

type skyworkTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Skywork) Name() string { return "skywork" }

func (p *Skywork) OwnsModel(model string) bool {
	return strings.Contains(model, "skywork-")
}

func (p *Skywork) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok skyworkTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "skywork"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.skywork.ai/v1/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "skywork"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "skywork", req.Stream)
}

func (p *Skywork) Healthy(ctx context.Context, acc *Account) bool {
	var tok skyworkTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.skywork.ai/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
