package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Mimo wraps xiaomimimo.com via Bearer API key.
type Mimo struct{ baseURL string }

type mimoTokens struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"` // per-account override takes highest priority
}

func (p *Mimo) Name() string { return "mimo" }

func (p *Mimo) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("mimo", "https://api.getmimo.com/v1")
}

func (p *Mimo) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "mimo/") || strings.Contains(model, "xiaomimimo")
}

func (p *Mimo) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok mimoTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "mimo"}
	}
	// Priority: per-account token BaseURL > DB/ConfigStore > hardcoded default
	base := tok.BaseURL
	if base == "" {
		base = p.baseURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "mimo"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "mimo", req.Stream)
}

func (p *Mimo) Healthy(ctx context.Context, acc *Account) bool {
	var tok mimoTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	return len(tok.APIKey) > 0
}
