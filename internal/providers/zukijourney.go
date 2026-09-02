package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ZukiJourney struct{}

type zukijourneyTokens struct {
	APIKey string `json:"api_key"`
}

func (p *ZukiJourney) Name() string { return "zukijourney" }

func (p *ZukiJourney) OwnsModel(model string) bool {
	return strings.Contains(model, "zuki/")
}

func (p *ZukiJourney) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok zukijourneyTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "zukijourney"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.zukijourney.com/v1/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "zukijourney"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "zukijourney", req.Stream)
}

func (p *ZukiJourney) Healthy(ctx context.Context, acc *Account) bool {
	var tok zukijourneyTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.zukijourney.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
