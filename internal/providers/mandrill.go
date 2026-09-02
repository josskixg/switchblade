package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Mandrill struct{ baseURL string }

type mandrillTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Mandrill) Name() string { return "mandrill" }

func (p *Mandrill) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("mandrill", "https://api.mandrillapp.com/api/1.0")
}

func (p *Mandrill) OwnsModel(model string) bool {
	return strings.Contains(model, "mandrill/")
}

func (p *Mandrill) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok mandrillTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "mandrill"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "mandrill"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "mandrill", req.Stream)
}

func (p *Mandrill) Healthy(ctx context.Context, acc *Account) bool {
	var tok mandrillTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	// ponytail: /users/ping is mandrill's actual health endpoint, not /models
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/users/ping", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
