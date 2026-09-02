package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Tiangong struct{}

type tiangongTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Tiangong) Name() string { return "tiangong" }

func (p *Tiangong) OwnsModel(model string) bool {
	for _, s := range []string{"tiangong", "sky-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Tiangong) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok tiangongTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "tiangong"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://sky-api.singularity-ai.com/saas/api/v4/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "tiangong"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "tiangong", req.Stream)
}

func (p *Tiangong) Healthy(ctx context.Context, acc *Account) bool {
	var tok tiangongTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://sky-api.singularity-ai.com/saas/api/v4/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
