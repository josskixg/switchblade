package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type TabNine struct{}

type tabnineTokens struct {
	APIKey string `json:"api_key"`
}

func (p *TabNine) Name() string { return "tabnine" }

func (p *TabNine) OwnsModel(model string) bool {
	return strings.Contains(model, "tabnine-")
}

func (p *TabNine) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok tabnineTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "tabnine"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tabnine.com/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "tabnine"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "tabnine", req.Stream)
}

func (p *TabNine) Healthy(ctx context.Context, acc *Account) bool {
	var tok tabnineTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.tabnine.com/chat/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
