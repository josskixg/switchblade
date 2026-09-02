package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Qwen struct{ baseURL string }

type qwenTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Qwen) Name() string { return "qwen" }

func (p *Qwen) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1")
}

func (p *Qwen) OwnsModel(model string) bool {
	for _, s := range []string{"qwen-", "qwen/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Qwen) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok qwenTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "qwen"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "qwen"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "qwen", req.Stream)
}

func (p *Qwen) Healthy(ctx context.Context, acc *Account) bool {
	var tok qwenTokens
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
