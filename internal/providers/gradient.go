package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Gradient struct{ baseURL string }

type gradientTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Gradient) Name() string { return "gradient" }

func (p *Gradient) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("gradient", "https://api.gradient.ai/api/v1")
}

func (p *Gradient) OwnsModel(model string) bool {
	for _, s := range []string{"gradient/", "gradient-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Gradient) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok gradientTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "gradient"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "gradient"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "gradient", req.Stream)
}

func (p *Gradient) Healthy(ctx context.Context, acc *Account) bool {
	var tok gradientTokens
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
