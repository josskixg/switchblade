package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Baichuan struct{ baseURL string }

type baichuanTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Baichuan) Name() string { return "baichuan" }

func (p *Baichuan) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("baichuan", "https://api.baichuan-ai.com/v1")
}

func (p *Baichuan) OwnsModel(model string) bool {
	for _, s := range []string{"baichuan"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Baichuan) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok baichuanTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "baichuan"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "baichuan"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "baichuan", req.Stream)
}

func (p *Baichuan) Healthy(ctx context.Context, acc *Account) bool {
	var tok baichuanTokens
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
