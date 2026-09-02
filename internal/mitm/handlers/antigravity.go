package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"switchblade/internal/providers"
)

// AntigravityHandler intercepts Antigravity IDE traffic.
type AntigravityHandler struct {
	domains []string
}

// NewAntigravityHandler creates a handler for Antigravity IDE.
func NewAntigravityHandler() *AntigravityHandler {
	return &AntigravityHandler{
		domains: []string{"api.antigravity.dev", "antigravity.dev"},
	}
}

func (h *AntigravityHandler) Name() string { return "antigravity" }

func (h *AntigravityHandler) Domains() []string { return h.domains }

func (h *AntigravityHandler) Match(req *http.Request) bool {
	host := strings.ToLower(req.Host)
	for _, d := range h.domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (h *AntigravityHandler) Parse(req *http.Request) (string, string, error) {
	// Antigravity uses OpenAI-compatible format
	if req.Body == nil {
		return "antigravity", "", nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "antigravity", "", err
	}
	req.Body.Close()
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "antigravity", "", nil
	}
	return "antigravity", payload.Model, nil
}

func (h *AntigravityHandler) Rewrite(req *http.Request, acc *providers.Account) error {
	// Inject account tokens
	var tokens map[string]interface{}
	if err := json.Unmarshal([]byte(acc.Tokens), &tokens); err != nil {
		return fmt.Errorf("parse antigravity tokens: %w", err)
	}

	if accessToken, ok := tokens["access_token"].(string); ok {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return nil
}
