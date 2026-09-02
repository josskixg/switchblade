package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type MyShell struct{ baseURL string }

type myshellTokens struct {
	APIKey string `json:"api_key"`
}

func (p *MyShell) Name() string { return "myshell" }

func (p *MyShell) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("myshell", "https://api.myshell.ai/v1")
}

func (p *MyShell) OwnsModel(model string) bool {
	for _, s := range []string{"myshell/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *MyShell) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok myshellTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "myshell"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "myshell"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "myshell", req.Stream)
}

func (p *MyShell) Healthy(ctx context.Context, acc *Account) bool {
	var tok myshellTokens
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
