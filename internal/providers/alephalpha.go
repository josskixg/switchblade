package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type AlephAlpha struct{ baseURL string }

type alephalphaTokens struct {
	APIKey string `json:"api_key"`
}

func (p *AlephAlpha) Name() string { return "alephalpha" }

func (p *AlephAlpha) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("alephalpha", "https://api.aleph-alpha.com")
}

func (p *AlephAlpha) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "luminous")
}

func (p *AlephAlpha) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok alephalphaTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "alephalpha"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "alephalpha"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "alephalpha", req.Stream)
}

func (p *AlephAlpha) Healthy(ctx context.Context, acc *Account) bool {
	var tok alephalphaTokens
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
