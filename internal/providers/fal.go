package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Fal struct{ baseURL string }

type falTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Fal) Name() string { return "fal" }

func (p *Fal) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("fal", "https://fal.run/v1")
}

func (p *Fal) OwnsModel(model string) bool {
	for _, s := range []string{"fal/", "fal-ai/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Fal) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok falTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "fal"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "fal"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "fal", req.Stream)
}

func (p *Fal) Healthy(ctx context.Context, acc *Account) bool {
	var tok falTokens
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
