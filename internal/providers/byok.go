package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// byokTokens is the shape of Account.Tokens for BYOK accounts.
type byokTokens struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

func parseBYOKTokens(tokens string) (byokTokens, error) {
	var t byokTokens
	if err := json.Unmarshal([]byte(tokens), &t); err != nil {
		return t, err
	}
	return t, nil
}

// BYOK is a passthrough provider that forwards requests to any OpenAI-compatible endpoint.
// ponytail: baseURL field unused — BYOK always requires per-account base_url in tokens; Configure is a no-op.
type BYOK struct{}

func (b *BYOK) Name() string { return "byok" }

func (b *BYOK) Configure(_ *ConfigStore) {}

func (b *BYOK) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "byok/")
}

func (b *BYOK) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	t, err := parseBYOKTokens(acc.Tokens)
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "byok"}
	}
	if t.BaseURL == "" {
		return nil, &ProviderError{Code: 500, Message: "missing base_url in tokens", Provider: "byok"}
	}

	// Strip "byok/" prefix from model in the raw body by patching the model field.
	// ponytail: re-encode only the model field; body is otherwise forwarded as-is.
	body := req.Body
	if strings.HasPrefix(req.Model, "byok/") {
		body, err = rewriteModel(body, strings.TrimPrefix(req.Model, "byok/"))
		if err != nil {
			return nil, &ProviderError{Code: 500, Message: "failed to rewrite model: " + err.Error(), Provider: "byok"}
		}
	}

	url := strings.TrimRight(t.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "byok"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+t.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := Exchange(httpReq, "byok", req.Stream)
	if err != nil {
		return nil, err
	}
	// Exchange buffers any status >= 400, so resp.Body holds the upstream error.
	if resp.StatusCode >= 400 {
		return nil, &ProviderError{
			Code:      resp.StatusCode,
			Message:   fmt.Sprintf("upstream returned %d: %s", resp.StatusCode, string(resp.Body)),
			Provider:  "byok",
			Retryable: resp.StatusCode >= 500,
		}
	}
	return resp, nil
}

func (b *BYOK) Healthy(ctx context.Context, acc *Account) bool {
	t, err := parseBYOKTokens(acc.Tokens)
	if err != nil || t.BaseURL == "" {
		return false
	}
	url := strings.TrimRight(t.BaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+t.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}

// rewriteModel patches the "model" field in a JSON object without touching anything else.
func rewriteModel(body []byte, model string) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return body, err
	}
	quoted, err := json.Marshal(model)
	if err != nil {
		return body, err
	}
	m["model"] = quoted
	return json.Marshal(m)
}
