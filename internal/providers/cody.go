package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Cody struct{ baseURL string }

type codyTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Cody) Name() string { return "cody" }

func (p *Cody) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("cody", "https://sourcegraph.com/.api/llm")
}

func (p *Cody) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "cody/")
}

func (p *Cody) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok codyTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "cody"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "cody"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "cody", req.Stream)
}

func (p *Cody) Healthy(ctx context.Context, acc *Account) bool {
	var tok codyTokens
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
