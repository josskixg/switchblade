package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type NvidiaNim struct{ baseURL string }

type nvidiaNimTokens struct {
	APIKey string `json:"api_key"`
}

func (p *NvidiaNim) Name() string { return "nvidianim" }

func (p *NvidiaNim) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("nvidianim", "https://integrate.api.nvidia.com/v1")
}

func (p *NvidiaNim) OwnsModel(model string) bool {
	for _, s := range []string{"nvidia/", "nim/", "meta/llama"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *NvidiaNim) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok nvidiaNimTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "nvidianim"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "nvidianim"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "nvidianim", req.Stream)
}

func (p *NvidiaNim) Healthy(ctx context.Context, acc *Account) bool {
	var tok nvidiaNimTokens
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
