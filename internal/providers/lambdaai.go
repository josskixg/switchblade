package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type LambdaAI struct{ baseURL string }

type lambdaaiTokens struct {
	APIKey string `json:"api_key"`
}

func (p *LambdaAI) Name() string { return "lambdaai" }

func (p *LambdaAI) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("lambdaai", "https://api.lambdalabs.com/v1")
}

func (p *LambdaAI) OwnsModel(model string) bool {
	for _, s := range []string{"lambda/", "lambdalabs/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *LambdaAI) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok lambdaaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "lambdaai"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "lambdaai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "lambdaai", req.Stream)
}

func (p *LambdaAI) Healthy(ctx context.Context, acc *Account) bool {
	var tok lambdaaiTokens
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
