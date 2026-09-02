package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type SiliconCloud struct{ baseURL string }

type siliconcloudTokens struct {
	APIKey string `json:"api_key"`
}

func (p *SiliconCloud) Name() string { return "siliconcloud" }

func (p *SiliconCloud) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("siliconcloud", "https://api.siliconcloud.net/v1")
}

func (p *SiliconCloud) OwnsModel(model string) bool {
	for _, s := range []string{"siliconcloud/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *SiliconCloud) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok siliconcloudTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "siliconcloud"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "siliconcloud"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "siliconcloud", req.Stream)
}

func (p *SiliconCloud) Healthy(ctx context.Context, acc *Account) bool {
	var tok siliconcloudTokens
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
