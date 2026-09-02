package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type InternLM struct{ baseURL string }

type internlmTokens struct {
	APIKey string `json:"api_key"`
}

func (p *InternLM) Name() string { return "internlm" }

func (p *InternLM) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("internlm", "https://internlm-chat.intern-ai.org.cn/puyu/api/v1")
}

func (p *InternLM) OwnsModel(model string) bool {
	for _, s := range []string{"internlm-", "internlm/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *InternLM) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok internlmTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "internlm"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "internlm"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "internlm", req.Stream)
}

func (p *InternLM) Healthy(ctx context.Context, acc *Account) bool {
	var tok internlmTokens
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
