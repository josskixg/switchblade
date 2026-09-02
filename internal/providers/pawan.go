package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Pawan struct{ baseURL string }

type pawanTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Pawan) Name() string { return "pawan" }

func (p *Pawan) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("pawan", "https://api.pawan.krd/v1")
}

func (p *Pawan) OwnsModel(model string) bool {
	for _, s := range []string{"pawan/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Pawan) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok pawanTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "pawan"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "pawan"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "pawan", req.Stream)
}

func (p *Pawan) Healthy(ctx context.Context, acc *Account) bool {
	var tok pawanTokens
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
