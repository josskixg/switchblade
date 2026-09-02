package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Clarifai struct{ baseURL string }

type clarifaiTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Clarifai) Name() string { return "clarifai" }

func (p *Clarifai) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("clarifai", "https://api.clarifai.com/v2")
}

func (p *Clarifai) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "clarifai/")
}

func (p *Clarifai) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok clarifaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "clarifai"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "clarifai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "clarifai", req.Stream)
}

func (p *Clarifai) Healthy(ctx context.Context, acc *Account) bool {
	var tok clarifaiTokens
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
