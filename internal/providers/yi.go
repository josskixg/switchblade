package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Yi struct{}

type yiTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Yi) Name() string { return "yi" }

func (p *Yi) OwnsModel(model string) bool {
	for _, s := range []string{"yi-", "zero-one"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Yi) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok yiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "yi"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.lingyiwanwu.com/v1/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "yi"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "yi", req.Stream)
}

func (p *Yi) Healthy(ctx context.Context, acc *Account) bool {
	var tok yiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.lingyiwanwu.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
