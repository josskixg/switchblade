package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type EleutherAI struct{ baseURL string }

type eleutheraiTokens struct {
	APIKey string `json:"api_key"`
}

func (p *EleutherAI) Name() string { return "eleutherai" }

func (p *EleutherAI) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("eleutherai", "https://api.eleutherai.org")
}

func (p *EleutherAI) OwnsModel(model string) bool {
	for _, s := range []string{"eleutherai/", "gpt-neox", "pythia"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *EleutherAI) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok eleutheraiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "eleutherai"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "eleutherai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "eleutherai", req.Stream)
}

func (p *EleutherAI) Healthy(ctx context.Context, acc *Account) bool {
	var tok eleutheraiTokens
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
