package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ChimeraGPT struct{ baseURL string }

type chimeragptTokens struct {
	APIKey string `json:"api_key"`
}

func (p *ChimeraGPT) Name() string { return "chimeragpt" }

func (p *ChimeraGPT) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("chimeragpt", "https://chimeragpt.adventblocks.cc/v1")
}

func (p *ChimeraGPT) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "chimeragpt/")
}

func (p *ChimeraGPT) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok chimeragptTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "chimeragpt"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "chimeragpt"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "chimeragpt", req.Stream)
}

func (p *ChimeraGPT) Healthy(ctx context.Context, acc *Account) bool {
	var tok chimeragptTokens
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
