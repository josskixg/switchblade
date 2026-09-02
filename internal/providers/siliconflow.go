package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type SiliconFlow struct{ baseURL string }

type siliconflowTokens struct {
	APIKey string `json:"api_key"`
}

func (p *SiliconFlow) Name() string { return "siliconflow" }

func (p *SiliconFlow) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("siliconflow", "https://api.siliconflow.cn/v1")
}

func (p *SiliconFlow) OwnsModel(model string) bool {
	for _, s := range []string{"siliconflow/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *SiliconFlow) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok siliconflowTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "siliconflow"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "siliconflow"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "siliconflow", req.Stream)
}

func (p *SiliconFlow) Healthy(ctx context.Context, acc *Account) bool {
	var tok siliconflowTokens
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
