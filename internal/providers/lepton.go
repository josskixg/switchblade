package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Lepton struct{ baseURL string }

type leptonTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Lepton) Name() string { return "lepton" }

func (p *Lepton) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("lepton", "https://api.lepton.ai/api/v1")
}

func (p *Lepton) OwnsModel(model string) bool {
	for _, s := range []string{"lepton/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Lepton) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok leptonTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "lepton"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "lepton"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "lepton", req.Stream)
}

func (p *Lepton) Healthy(ctx context.Context, acc *Account) bool {
	var tok leptonTokens
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
