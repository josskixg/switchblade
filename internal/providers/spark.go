package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Spark struct{}

type sparkTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Spark) Name() string { return "spark" }

func (p *Spark) OwnsModel(model string) bool {
	for _, s := range []string{"spark", "xunfei"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Spark) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok sparkTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "spark"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://spark-api-open.xf-yun.com/v1/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "spark"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "spark", req.Stream)
}

func (p *Spark) Healthy(ctx context.Context, acc *Account) bool {
	var tok sparkTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://spark-api-open.xf-yun.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
