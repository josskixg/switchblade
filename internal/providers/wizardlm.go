package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type WizardLM struct{}

type wizardlmTokens struct {
	APIKey string `json:"api_key"`
}

func (p *WizardLM) Name() string { return "wizardlm" }

func (p *WizardLM) OwnsModel(model string) bool {
	for _, s := range []string{"wizardlm-", "wizard-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *WizardLM) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok wizardlmTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "wizardlm"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.wizardlm.ai/v1/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "wizardlm"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "wizardlm", req.Stream)
}

func (p *WizardLM) Healthy(ctx context.Context, acc *Account) bool {
	var tok wizardlmTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.wizardlm.ai/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
