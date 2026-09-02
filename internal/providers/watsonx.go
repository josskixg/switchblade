package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type WatsonX struct{}

type watsonxTokens struct {
	APIKey string `json:"api_key"`
}

func (p *WatsonX) Name() string { return "watsonx" }

func (p *WatsonX) OwnsModel(model string) bool {
	for _, s := range []string{"ibm/", "watsonx/", "granite-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *WatsonX) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok watsonxTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "watsonx"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://us-south.ml.cloud.ibm.com/ml/v1/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "watsonx"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "watsonx", req.Stream)
}

func (p *WatsonX) Healthy(ctx context.Context, acc *Account) bool {
	var tok watsonxTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://us-south.ml.cloud.ibm.com/ml/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
