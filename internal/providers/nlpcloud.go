package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type NLPCloud struct{ baseURL string }

type nlpcloudTokens struct {
	APIKey string `json:"api_key"`
}

func (p *NLPCloud) Name() string { return "nlpcloud" }

func (p *NLPCloud) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("nlpcloud", "https://api.nlpcloud.io/v1")
}

func (p *NLPCloud) OwnsModel(model string) bool {
	for _, s := range []string{"nlpcloud/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *NLPCloud) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok nlpcloudTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "nlpcloud"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "nlpcloud"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "nlpcloud", req.Stream)
}

func (p *NLPCloud) Healthy(ctx context.Context, acc *Account) bool {
	var tok nlpcloudTokens
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
