package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Vertex handles Google Cloud Vertex AI.
// project_id must be in acc.Tokens.
type Vertex struct{}

type vertexTokens struct {
	APIKey    string `json:"api_key"`
	ProjectID string `json:"project_id"`
}

func (p *Vertex) Name() string { return "vertex" }

func (p *Vertex) OwnsModel(model string) bool {
	for _, s := range []string{"vertex/", "gemini-pro", "gemini-ultra"} {
		if strings.Contains(model, s) {
			return true
		}
	}
	return false
}

func (p *Vertex) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok vertexTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 500, Message: "invalid tokens: " + err.Error(), Provider: "vertex"}
	}
	projectID := tok.ProjectID
	if projectID == "" {
		projectID = "YOUR_PROJECT_ID"
	}
	url := "https://us-central1-aiplatform.googleapis.com/v1/projects/" + projectID + "/locations/us-central1/endpoints/openapi/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "vertex"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	return Exchange(httpReq, "vertex", req.Stream)
}

func (p *Vertex) Healthy(ctx context.Context, acc *Account) bool {
	var tok vertexTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	projectID := tok.ProjectID
	if projectID == "" {
		projectID = "YOUR_PROJECT_ID"
	}
	url := "https://us-central1-aiplatform.googleapis.com/v1/projects/" + projectID + "/locations/us-central1/models"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
