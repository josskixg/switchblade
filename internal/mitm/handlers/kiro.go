package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"switchblade/internal/providers"
)

// KiroHandler intercepts Kiro IDE traffic.
type KiroHandler struct {
	domains []string
}

// NewKiroHandler creates a handler for Kiro IDE.
func NewKiroHandler() *KiroHandler {
	return &KiroHandler{
		domains: []string{"api.kiro.dev", "kiro.dev"},
	}
}

func (h *KiroHandler) Name() string { return "kiro" }

func (h *KiroHandler) Domains() []string { return h.domains }

func (h *KiroHandler) Match(req *http.Request) bool {
	host := strings.ToLower(req.Host)
	for _, d := range h.domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (h *KiroHandler) Parse(req *http.Request) (string, string, error) {
	// Kiro uses OpenAI-compatible format
	if req.Body == nil {
		return "kiro", "", nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "kiro", "", err
	}
	req.Body.Close()
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "kiro", "", nil
	}
	return "kiro", payload.Model, nil
}

func (h *KiroHandler) Rewrite(req *http.Request, acc *providers.Account) error {
	// Inject account tokens
	var tokens map[string]interface{}
	if err := json.Unmarshal([]byte(acc.Tokens), &tokens); err != nil {
		return fmt.Errorf("parse kiro tokens: %w", err)
	}

	if accessToken, ok := tokens["access_token"].(string); ok {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return nil
}
