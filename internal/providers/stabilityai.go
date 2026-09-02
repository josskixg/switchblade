package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type StabilityAI struct{}

type stabilityaiTokens struct {
	APIKey string `json:"api_key"`
}

func (p *StabilityAI) Name() string { return "stabilityai" }

func (p *StabilityAI) OwnsModel(model string) bool {
	for _, s := range []string{"stable-diffusion", "stable-image", "sdxl"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *StabilityAI) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok stabilityaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "stabilityai"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stability.ai/v1/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "stabilityai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "stabilityai", req.Stream)
}

func (p *StabilityAI) Healthy(ctx context.Context, acc *Account) bool {
	var tok stabilityaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.stability.ai/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
