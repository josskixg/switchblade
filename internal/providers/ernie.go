package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Ernie struct{ baseURL string }

type ernieTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Ernie) Name() string { return "ernie" }

func (p *Ernie) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("ernie", "https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop")
}

func (p *Ernie) OwnsModel(model string) bool {
	for _, s := range []string{"ernie-", "wenxin"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Ernie) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok ernieTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "ernie"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "ernie"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "ernie", req.Stream)
}

func (p *Ernie) Healthy(ctx context.Context, acc *Account) bool {
	var tok ernieTokens
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
