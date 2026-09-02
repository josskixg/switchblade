package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ChatGLM struct{ baseURL string }

type chatglmTokens struct {
	APIKey string `json:"api_key"`
}

func (p *ChatGLM) Name() string { return "chatglm" }

func (p *ChatGLM) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("chatglm", "https://open.bigmodel.cn/api/paas/v4")
}

func (p *ChatGLM) OwnsModel(model string) bool {
	for _, s := range []string{"glm", "chatglm"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *ChatGLM) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok chatglmTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "chatglm"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "chatglm"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "chatglm", req.Stream)
}

func (p *ChatGLM) Healthy(ctx context.Context, acc *Account) bool {
	var tok chatglmTokens
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
