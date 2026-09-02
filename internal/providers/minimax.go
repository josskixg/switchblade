package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type MiniMax struct{ baseURL string }

type minimaxTokens struct {
	APIKey string `json:"api_key"`
}

func (p *MiniMax) Name() string { return "minimax" }

func (p *MiniMax) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("minimax", "https://api.minimax.chat/v1")
}

func (p *MiniMax) OwnsModel(model string) bool {
	for _, s := range []string{"minimax/", "abab"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *MiniMax) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok minimaxTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "minimax"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "minimax"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "minimax", req.Stream)
}

func (p *MiniMax) Healthy(ctx context.Context, acc *Account) bool {
	var tok minimaxTokens
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
