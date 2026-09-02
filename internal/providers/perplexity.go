package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Perplexity struct{ baseURL string }

type perplexityTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Perplexity) Name() string { return "perplexity" }

func (p *Perplexity) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("perplexity", "https://api.perplexity.ai")
}

func (p *Perplexity) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "perplexity/") || strings.HasPrefix(model, "pplx-") || strings.Contains(model, "sonar")
}

func (p *Perplexity) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok perplexityTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "perplexity"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "perplexity"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "perplexity", req.Stream)
}

func (p *Perplexity) Healthy(ctx context.Context, acc *Account) bool {
	return len(acc.Tokens) > 10 // ponytail: perplexity has no public /models endpoint; token presence is enough
}
