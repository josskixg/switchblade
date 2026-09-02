package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Inflection struct{ baseURL string }

type inflectionTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Inflection) Name() string { return "inflection" }

func (p *Inflection) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("inflection", "https://api.inflection.ai/v1")
}

func (p *Inflection) OwnsModel(model string) bool {
	for _, s := range []string{"inflection/", "pi-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Inflection) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok inflectionTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "inflection"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "inflection"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "inflection", req.Stream)
}

func (p *Inflection) Healthy(ctx context.Context, acc *Account) bool {
	var tok inflectionTokens
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
