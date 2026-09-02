package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

type OpenAI struct{ baseURL string }

type openaiTokens struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

func (p *OpenAI) Name() string { return "openai" }

func (p *OpenAI) Configure(store *ConfigStore) {
	p.baseURL = store.GetBaseURL("openai", "https://api.openai.com/v1")
}

func (p *OpenAI) OwnsModel(model string) bool {
	for _, prefix := range []string{"gpt", "o1", "o3", "o4", "dall-e", "whisper", "tts", "text-embedding"} {
		if strings.Contains(model, prefix) {
			return true
		}
	}
	return false
}

func (p *OpenAI) Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error) {
	var tok openaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 400, Message: "invalid tokens: " + err.Error(), Provider: "openai"}
	}
	// Priority: per-account token BaseURL > DB/ConfigStore > hardcoded default
	base := tok.BaseURL
	if base == "" {
		base = p.baseURL
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(req.Body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	return Exchange(httpReq, "openai", req.Stream)
}

func (p *OpenAI) Healthy(ctx context.Context, acc *Account) bool {
	var tok openaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return false
	}
	// Priority: per-account token BaseURL > DB/ConfigStore > hardcoded default
	base := tok.BaseURL
	if base == "" {
		base = p.baseURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+tok.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}

func (p *OpenAI) Embeddings(ctx context.Context, acc *Account, req *EmbeddingsRequest) (*EmbeddingsResponse, error) {
	var tok openaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 400, Message: "invalid tokens: " + err.Error(), Provider: "openai"}
	}
	base := tok.BaseURL
	if base == "" {
		base = p.baseURL
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Code: 502, Message: err.Error(), Provider: "openai", Retryable: true}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Code: 502, Message: err.Error(), Provider: "openai"}
	}

	var embResp EmbeddingsResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, &ProviderError{Code: 502, Message: "failed to parse response: " + err.Error(), Provider: "openai"}
	}
	embResp.StatusCode = resp.StatusCode
	return &embResp, nil
}

func (p *OpenAI) TTS(ctx context.Context, acc *Account, req *TTSRequest) (*TTSResponse, error) {
	var tok openaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 400, Message: "invalid tokens: " + err.Error(), Provider: "openai"}
	}
	base := tok.BaseURL
	if base == "" {
		base = p.baseURL
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Code: 502, Message: err.Error(), Provider: "openai", Retryable: true}
	}
	defer resp.Body.Close()

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Code: 502, Message: err.Error(), Provider: "openai"}
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}

	return &TTSResponse{
		Audio:       audioBytes,
		ContentType: contentType,
		StatusCode:  resp.StatusCode,
	}, nil
}

func (p *OpenAI) STT(ctx context.Context, acc *Account, req *STTRequest) (*STTResponse, error) {
	var tok openaiTokens
	if err := json.Unmarshal([]byte(acc.Tokens), &tok); err != nil {
		return nil, &ProviderError{Code: 400, Message: "invalid tokens: " + err.Error(), Provider: "openai"}
	}
	base := tok.BaseURL
	if base == "" {
		base = p.baseURL
	}

	// Build multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file as proper multipart file upload
	filename := req.Filename
	if filename == "" {
		filename = "audio.mp3"
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}
	if _, err := part.Write(req.File); err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}
	if err := writer.WriteField("model", req.Model); err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}
	if req.Language != "" {
		if err := writer.WriteField("language", req.Language); err != nil {
			return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
		}
	}
	if req.Prompt != "" {
		if err := writer.WriteField("prompt", req.Prompt); err != nil {
			return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
		}
	}
	if req.ResponseFormat != "" {
		if err := writer.WriteField("response_format", req.ResponseFormat); err != nil {
			return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/audio/transcriptions", &buf)
	if err != nil {
		return nil, &ProviderError{Code: 500, Message: err.Error(), Provider: "openai"}
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok.APIKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Code: 502, Message: err.Error(), Provider: "openai", Retryable: true}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Code: 502, Message: err.Error(), Provider: "openai"}
	}

	var sttResp STTResponse
	if err := json.Unmarshal(respBody, &sttResp); err != nil {
		return nil, &ProviderError{Code: 502, Message: "failed to parse response: " + err.Error(), Provider: "openai"}
	}
	sttResp.StatusCode = resp.StatusCode
	sttResp.Body = respBody
	return &sttResp, nil
}
