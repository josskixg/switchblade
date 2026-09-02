package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Hunyuan struct{ baseURL string }

type hunyuanTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Hunyuan) Name() string { return "hunyuan" }

func (p *Hunyuan) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("hunyuan", "https://hunyuan.tencentcloudapi.com")
}

func (p *Hunyuan) OwnsModel(model string) bool {
	for _, s := range []string{"hunyuan-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Hunyuan) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok hunyuanTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "hunyuan"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "hunyuan"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "hunyuan", req.Stream)
}

func (p *Hunyuan) Healthy(ctx context.Context, acc *Account) bool {
	var tok hunyuanTokens
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
