package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"switchblade/internal/providers"
)

// CopilotHandler intercepts GitHub Copilot traffic.
type CopilotHandler struct {
	domains []string
}

// NewCopilotHandler creates a handler for GitHub Copilot.
func NewCopilotHandler() *CopilotHandler {
	return &CopilotHandler{
		domains: []string{
			"api.github.com",
			"copilot-proxy.githubusercontent.com",
			"github.com",
		},
	}
}

func (h *CopilotHandler) Name() string { return "copilot" }

func (h *CopilotHandler) Domains() []string { return h.domains }

func (h *CopilotHandler) Match(req *http.Request) bool {
	host := strings.ToLower(req.Host)
	for _, d := range h.domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (h *CopilotHandler) Parse(req *http.Request) (string, string, error) {
	// Copilot uses GitHub's API format
	if req.Body == nil {
		return "copilot", "", nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "copilot", "", err
	}
	req.Body.Close()
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "copilot", "", nil
	}
	return "copilot", payload.Model, nil
}

func (h *CopilotHandler) Rewrite(req *http.Request, acc *providers.Account) error {
	// Inject GitHub token
	var tokens map[string]interface{}
	if err := json.Unmarshal([]byte(acc.Tokens), &tokens); err != nil {
		return fmt.Errorf("parse copilot tokens: %w", err)
	}

	if accessToken, ok := tokens["access_token"].(string); ok {
		req.Header.Set("Authorization", "token "+accessToken)
		req.Header.Set("Editor-Version", "vscode/1.85.0")
		req.Header.Set("Editor-Plugin-Version", "copilot-chat/0.12.0")
	}
	return nil
}
