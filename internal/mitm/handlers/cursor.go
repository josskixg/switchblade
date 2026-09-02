package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"switchblade/internal/providers"
)

// CursorHandler intercepts Cursor IDE traffic (api2.cursor.sh).
type CursorHandler struct {
	domains []string
}

// NewCursorHandler creates a handler for Cursor IDE.
func NewCursorHandler() *CursorHandler {
	return &CursorHandler{
		domains: []string{"api2.cursor.sh", "cursor.sh"},
	}
}

func (h *CursorHandler) Name() string { return "cursor" }

func (h *CursorHandler) Domains() []string { return h.domains }

func (h *CursorHandler) Match(req *http.Request) bool {
	host := strings.ToLower(req.Host)
	for _, d := range h.domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (h *CursorHandler) Parse(req *http.Request) (string, string, error) {
	// Cursor uses OpenAI-compatible format
	if req.Body == nil {
		return "cursor", "", nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "cursor", "", err
	}
	req.Body.Close()
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "cursor", "", nil
	}
	return "cursor", payload.Model, nil
}

func (h *CursorHandler) Rewrite(req *http.Request, acc *providers.Account) error {
	// Inject account tokens into Authorization header
	var tokens map[string]interface{}
	if err := json.Unmarshal([]byte(acc.Tokens), &tokens); err != nil {
		return fmt.Errorf("parse cursor tokens: %w", err)
	}

	if accessToken, ok := tokens["access_token"].(string); ok {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return nil
}
