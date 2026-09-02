package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Forefront struct{ baseURL string }

type forefrontTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Forefront) Name() string { return "forefront" }

func (p *Forefront) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("forefront", "https://api.forefront.ai/v1")
}

func (p *Forefront) OwnsModel(model string) bool {
	for _, s := range []string{"forefront/", "forefront-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Forefront) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok forefrontTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "forefront"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "forefront"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "forefront", req.Stream)
}

func (p *Forefront) Healthy(ctx context.Context, acc *Account) bool {
	var tok forefrontTokens
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
