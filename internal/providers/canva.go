package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Canva wraps Canva Magic Media image generation.
// Auth via browser cookies (caz token) obtained by Python bot.
type Canva struct{ baseURL string }

type canvaTokens struct {
	CazToken string `json:"caz_token"`
}

func (p *Canva) Name() string { return "canva" }

func (p *Canva) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("canva", "https://api.canva.com/rest/v1")
}

func (p *Canva) OwnsModel(model string) bool {
	return strings.HasPrefix(model, "canva-") || strings.HasPrefix(model, "canva/")
}

func (p *Canva) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok canvaTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "canva"}
	}
	// ponytail: Canva uses a proprietary GraphQL API — stub until Python bridge is implemented
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/ai/generate", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "canva"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.CazToken)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "canva", req.Stream)
}

func (p *Canva) Healthy(ctx context.Context, acc *Account) bool {
	var tok canvaTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	return len(tok.CazToken) > 10
}
