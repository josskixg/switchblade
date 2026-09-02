package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Moonshot struct{ baseURL string }

type moonshotTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Moonshot) Name() string { return "moonshot" }

func (p *Moonshot) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("moonshot", "https://api.moonshot.cn/v1")
}

func (p *Moonshot) OwnsModel(model string) bool {
	for _, s := range []string{"moonshot/", "moonshot-", "kimi-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Moonshot) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok moonshotTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "moonshot"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "moonshot"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "moonshot", req.Stream)
}

func (p *Moonshot) Healthy(ctx context.Context, acc *Account) bool {
	var tok moonshotTokens
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
