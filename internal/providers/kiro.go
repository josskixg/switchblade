package providers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// kiroTokens is the shape of Account.Tokens for Kiro (AWS CodeWhisperer) accounts.
type kiroTokens struct {
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token"`
	ProfileArn    string `json:"profile_arn"`
	ProfileArnAlt string `json:"profileArn"` // ponytail: some tokens use camelCase
	Endpoint      string `json:"endpoint"`
}

func parseKiroTokens(raw string) (kiroTokens, error) {
	var t kiroTokens
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return t, err
	}
	return t, nil
}

func (t kiroTokens) profileArn() string {
	if t.ProfileArn != "" {
		return t.ProfileArn
	}
	return t.ProfileArnAlt
}

// resolveBaseURL returns token endpoint > configstore value > hardcoded default.
func (t kiroTokens) resolveBaseURL(configBase string) string {
	if t.Endpoint != "" {
		return t.Endpoint
	}
	if configBase != "" {
		return configBase
	}
	return "https://q.us-east-1.amazonaws.com"
}

// Kiro is the fallback provider backed by AWS CodeWhisperer / Amazon Q.
// OwnsModel always returns true — it sits last in the priority order.
type Kiro struct{ baseURL string }

func (k *Kiro) Name() string { return "kiro" }

func (k *Kiro) Configure(store *ConfigStore) {
	k.baseURL = store.GetBaseURL("kiro", "https://q.us-east-1.amazonaws.com")
}

// OwnsModel always returns true — kiro is the catch-all fallback.
func (k *Kiro) OwnsModel(_ string) bool { return true }

// Healthy checks that access_token is present and non-trivially short.
// ponytail: real expiry/refresh is the Python auth bot's job — length check only.
func (k *Kiro) Healthy(_ context.Context, acc *Account) bool {
	t, err := parseKiroTokens(acc.Tokens)
	if err != nil {
		return false
	}
	return len(t.AccessToken) > 20
}

// Chat posts the raw request body to the Kiro generateAssistantResponse endpoint.
// The body is forwarded as-is; format translation is handled upstream by the router.
func (k *Kiro) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	t, err := parseKiroTokens(acc.Tokens)
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "kiro"}
	}
	if t.AccessToken == "" {
		return nil, &ProviderError{Code: 401, Message: "missing access_token", Provider: "kiro"}
	}

	modelName := req.Model
	body := req.Body
	if strings.HasPrefix(modelName, "kp-") {
		modelName = strings.TrimPrefix(modelName, "kp-")
		if len(body) > 0 {
			body, err = rewriteModel(body, modelName)
			if err != nil {
				return nil, &ProviderError{Code: 500, Message: "failed to rewrite model: " + err.Error(), Provider: "kiro"}
			}
		}
	}

	if len(body) == 0 {
		// ponytail: no raw body — build minimal Kiro request from structured fields
		body, err = buildKiroBody(&ChatRequest{Model: modelName, Messages: req.Messages, Stream: req.Stream}, t.profileArn())
		if err != nil {
			return nil, &ProviderError{Code: 500, Message: "building request: " + err.Error(), Provider: "kiro"}
		}
	}

	url := t.resolveBaseURL(k.baseURL) + "/generateAssistantResponse"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "kiro"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+t.AccessToken)
	httpReq.Header.Set("Content-Type", "application/json")
	if arn := t.profileArn(); arn != "" {
		httpReq.Header.Set("x-amzn-codewhisperer-optout", "false")
		httpReq.Header.Set("profileArn", arn)
	}

	return Exchange(httpReq, "kiro", req.Stream)
}

func buildKiroBody(req *ChatRequest, profileArn string) ([]byte, error) {
	var combined string
	for _, m := range req.Messages {
		combined += m.Role + ": " + m.Content + "\n"
	}
	convID := newUUID()
	body := map[string]any{
		"conversationState": map[string]any{
			"agentTaskType":   "vibe",
			"chatTriggerType": "MANUAL",
			"conversationId":  convID,
			"currentMessage": map[string]any{
				"userInputMessage": map[string]any{
					"content": combined,
					"modelId": req.Model,
					"origin":  "AI_EDITOR",
					"userInputMessageContext": map[string]any{
						"tools": []any{},
					},
				},
			},
			"history": []any{},
		},
	}
	if profileArn != "" {
		body["profileArn"] = profileArn
	}
	return json.Marshal(body)
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
