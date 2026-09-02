package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Xuanyuan struct{}

type xuanyuanTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Xuanyuan) Name() string { return "xuanyuan" }

func (p *Xuanyuan) OwnsModel(model string) bool {
	return strings.Contains(model, "xuanyuan")
}

func (p *Xuanyuan) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok xuanyuanTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "xuanyuan"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.xuanyuan.finance/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "xuanyuan"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "xuanyuan", req.Stream)
}

func (p *Xuanyuan) Healthy(ctx context.Context, acc *Account) bool {
	var tok xuanyuanTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.xuanyuan.finance/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
