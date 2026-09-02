package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type TextSynth struct{}

type textsynthTokens struct {
	APIKey string `json:"api_key"`
}

func (p *TextSynth) Name() string { return "textsynth" }

func (p *TextSynth) OwnsModel(model string) bool {
	for _, s := range []string{"textsynth/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *TextSynth) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok textsynthTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "textsynth"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://textsynth.com/v1/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "textsynth"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "textsynth", req.Stream)
}

func (p *TextSynth) Healthy(ctx context.Context, acc *Account) bool {
	var tok textsynthTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://textsynth.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
