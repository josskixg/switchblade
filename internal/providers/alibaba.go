package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Alibaba wraps Alibaba Cloud DashScope (Qwen models) via OpenAI-compatible endpoint.
type Alibaba struct{ baseURL string }

type alibabaTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Alibaba) Name() string { return "alibaba" }

func (p *Alibaba) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("alibaba", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1")
}

func (p *Alibaba) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "alibaba/") || strings.HasPrefix(model, "qwen")
}

func (p *Alibaba) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok alibabaTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "alibaba"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "alibaba"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "alibaba", req.Stream)
}

func (p *Alibaba) Healthy(ctx context.Context, acc *Account) bool {
	var tok alibabaTokens
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
