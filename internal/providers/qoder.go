package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Qoder wraps qoder.sh via COSY Bearer protocol.
type Qoder struct{ baseURL string }

type qoderTokens struct {
	CosyBearer string `json:"cosy_bearer"`
	BaseURL    string `json:"base_url"` // per-account override takes highest priority
}

func (p *Qoder) Name() string { return "qoder" }

func (p *Qoder) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("qoder", "https://api.qoder.ai/v1")
}

func (p *Qoder) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "qd-") || strings.HasPrefix(model, "qoder/")
}

func (p *Qoder) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok qoderTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "qoder"}
	}
	// Priority: per-account token BaseURL > DB/ConfigStore > hardcoded default
	base := tok.BaseURL
	if base == "" {
		base = p.baseURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "qoder"}
	}
	httpReq.Header.Set("Authorization", "COSY "+tok.CosyBearer)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "qoder", req.Stream)
}

func (p *Qoder) Healthy(ctx context.Context, acc *Account) bool {
	return len(acc.Tokens) > 10
}
