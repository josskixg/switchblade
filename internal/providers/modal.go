package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Modal struct{ baseURL string }

type modalTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Modal) Name() string { return "modal" }

func (p *Modal) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("modal", "https://api.modal.com/v1")
}

func (p *Modal) OwnsModel(model string) bool {
	for _, s := range []string{"modal/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Modal) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok modalTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "modal"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "modal"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "modal", req.Stream)
}

func (p *Modal) Healthy(ctx context.Context, acc *Account) bool {
	var tok modalTokens
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
