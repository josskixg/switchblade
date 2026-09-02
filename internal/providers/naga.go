package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Naga struct{ baseURL string }

type nagaTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Naga) Name() string { return "naga" }

func (p *Naga) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("naga", "https://api.naga.ac/v1")
}

func (p *Naga) OwnsModel(model string) bool {
	for _, s := range []string{"naga/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Naga) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok nagaTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "naga"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "naga"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "naga", req.Stream)
}

func (p *Naga) Healthy(ctx context.Context, acc *Account) bool {
	var tok nagaTokens
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
