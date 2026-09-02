package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// ABAB handles MiniMax ABAB series models.
type ABAB struct{ baseURL string }

type ababTokens struct {
	APIKey string `json:"api_key"`
}

func (p *ABAB) Name() string { return "abab" }

func (p *ABAB) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("abab", "https://api.minimax.chat/v1")
}

func (p *ABAB) OwnsModel(model string) bool {
	for _, s := range []string{"abab5", "abab6"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *ABAB) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok ababTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "abab"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "abab"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "abab", req.Stream)
}

func (p *ABAB) Healthy(ctx context.Context, acc *Account) bool {
	var tok ababTokens
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
