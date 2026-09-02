package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type MonsterAPI struct{ baseURL string }

type monsterapiTokens struct {
	APIKey string `json:"api_key"`
}

func (p *MonsterAPI) Name() string { return "monsterapi" }

func (p *MonsterAPI) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("monsterapi", "https://api.monsterapi.ai/v1")
}

func (p *MonsterAPI) OwnsModel(model string) bool {
	for _, s := range []string{"monsterapi/"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *MonsterAPI) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok monsterapiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "monsterapi"}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "monsterapi"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "monsterapi", req.Stream)
}

func (p *MonsterAPI) Healthy(ctx context.Context, acc *Account) bool {
	var tok monsterapiTokens
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
