package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Azure struct{ baseURL string }

type azureTokens struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

func (p *Azure) Name() string { return "azure" }

func (p *Azure) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("azure", "https://YOUR_RESOURCE.openai.azure.com/openai/deployments")
}

func (p *Azure) OwnsModel(model string) bool {
	for _, s := range []string{"azure/", "azure-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Azure) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok azureTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "azure"}
	}
	// per-account base_url takes priority over global default
	baseURL := tok.BaseURL
	if baseURL == "" {
		baseURL = p.baseURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "azure"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "azure", req.Stream)
}

func (p *Azure) Healthy(ctx context.Context, acc *Account) bool {
	var tok azureTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	baseURL := tok.BaseURL
	if baseURL == "" {
		baseURL = p.baseURL
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
