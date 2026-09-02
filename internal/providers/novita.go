package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Novita struct{ baseURL string }

type novitaTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Novita) Name() string { return "novita" }

func (p *Novita) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("novita", "https://api.novita.ai/v3/openai")
}

func (p *Novita) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "novita/")
}

func (p *Novita) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok novitaTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "novita"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "novita"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "novita", req.Stream)
}

func (p *Novita) Healthy(ctx context.Context, acc *Account) bool {
	var tok novitaTokens
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
