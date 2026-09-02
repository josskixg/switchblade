package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Zhipu struct{}

type zhipuTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Zhipu) Name() string { return "zhipu" }

func (p *Zhipu) OwnsModel(model string) bool {
	for _, s := range []string{"glm", "cogview", "cogvideo"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Zhipu) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok zhipuTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "zhipu"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://open.bigmodel.cn/api/paas/v4/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "zhipu"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "zhipu", req.Stream)
}

func (p *Zhipu) Healthy(ctx context.Context, acc *Account) bool {
	var tok zhipuTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://open.bigmodel.cn/api/paas/v4/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
