package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type AI21 struct{ baseURL string }

type ai21Tokens struct {
	APIKey string `json:"api_key"`
}

func (p *AI21) Name() string { return "ai21" }

func (p *AI21) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("ai21", "https://api.ai21.com/studio/v1")
}

func (p *AI21) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "ai21/") || strings.HasPrefix(model, "jamba") || strings.HasPrefix(model, "j2-")
}

func (p *AI21) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok ai21Tokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "ai21"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "ai21"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "ai21", req.Stream)
}

func (p *AI21) Healthy(ctx context.Context, acc *Account) bool {
	var tok ai21Tokens
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
