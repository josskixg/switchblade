package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type LingYiWanWu struct{ baseURL string }

type lingyiwanwuTokens struct {
	APIKey string `json:"api_key"`
}

func (p *LingYiWanWu) Name() string { return "lingyiwanwu" }

func (p *LingYiWanWu) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("lingyiwanwu", "https://api.lingyiwanwu.com/v1")
}

func (p *LingYiWanWu) OwnsModel(model string) bool {
	for _, s := range []string{"yi-", "lingyiwanwu/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *LingYiWanWu) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok lingyiwanwuTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "lingyiwanwu"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "lingyiwanwu"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "lingyiwanwu", req.Stream)
}

func (p *LingYiWanWu) Healthy(ctx context.Context, acc *Account) bool {
	var tok lingyiwanwuTokens
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
