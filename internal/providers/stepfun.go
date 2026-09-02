package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Stepfun struct{}

type stepfunTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Stepfun) Name() string { return "stepfun" }

func (p *Stepfun) OwnsModel(model string) bool {
	for _, s := range []string{"step-"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Stepfun) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok stepfunTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "stepfun"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stepfun.com/v1/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "stepfun"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "stepfun", req.Stream)
}

func (p *Stepfun) Healthy(ctx context.Context, acc *Account) bool {
	var tok stepfunTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.stepfun.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
