package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// CodeBuddy wraps the CodeBuddy API (API key or cookies).
type CodeBuddy struct{ baseURL string }

type codebuddyTokens struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"` // per-account override takes highest priority
}

func (p *CodeBuddy) Name() string { return "codebuddy" }

func (p *CodeBuddy) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("codebuddy", "https://aicodebuddy.vivotek.com/v1")
}

func (p *CodeBuddy) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "cb-") || strings.HasPrefix(model, "codebuddy/")
}

func (p *CodeBuddy) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok codebuddyTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "codebuddy"}
	}
	// Priority: per-account token BaseURL > DB/ConfigStore > hardcoded default
	base := tok.BaseURL
	if base == "" {
		base = p.baseURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "codebuddy"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "codebuddy", req.Stream)
}

func (p *CodeBuddy) Healthy(ctx context.Context, acc *Account) bool {
	var tok codebuddyTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	return len(tok.APIKey) > 0
}
