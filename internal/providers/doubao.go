package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Doubao struct{ baseURL string }

type doubaoTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Doubao) Name() string { return "doubao" }

func (p *Doubao) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("doubao", "https://ark.cn-beijing.volces.com/api/v3")
}

func (p *Doubao) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "doubao/") || strings.HasPrefix(model, "ep-")
}

func (p *Doubao) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok doubaoTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "doubao"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "doubao"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "doubao", req.Stream)
}

func (p *Doubao) Healthy(ctx context.Context, acc *Account) bool {
	var tok doubaoTokens
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
