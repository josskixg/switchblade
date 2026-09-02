package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type RunPod struct{ baseURL string }

type runpodTokens struct {
	APIKey string `json:"api_key"`
}

func (p *RunPod) Name() string { return "runpod" }

func (p *RunPod) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("runpod", "https://api.runpod.ai/v2")
}

func (p *RunPod) OwnsModel(model string) bool {
	for _, s := range []string{"runpod/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *RunPod) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok runpodTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "runpod"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "runpod"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "runpod", req.Stream)
}

func (p *RunPod) Healthy(ctx context.Context, acc *Account) bool {
	var tok runpodTokens
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
