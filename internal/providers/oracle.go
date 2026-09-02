package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Oracle struct{ baseURL string }

type oracleTokens struct {
	APIKey string `json:"api_key"`
}

func (p *Oracle) Name() string { return "oracle" }

func (p *Oracle) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("oracle", "https://inference.generativeai.us-chicago-1.oci.oraclecloud.com/20231130")
}

func (p *Oracle) OwnsModel(model string) bool {
	for _, s := range []string{"oracle/", "oci/", "cohere."} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Oracle) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok oracleTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "oracle"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "oracle"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "oracle", req.Stream)
}

func (p *Oracle) Healthy(ctx context.Context, acc *Account) bool {
	var tok oracleTokens
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
