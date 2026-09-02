package providers

import "errors"

// ErrNotSupported is returned by providers that don't implement a service kind.
var ErrNotSupported = errors.New("service not supported")

// EmbeddingsRequest is the inbound embeddings request.
type EmbeddingsRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"` // string or []string
}

// EmbeddingsResponse is what a provider returns for embeddings.
type EmbeddingsResponse struct {
	Object     string            `json:"object"` // "list"
	Data       []EmbeddingObject `json:"data"`
	Model      string            `json:"model"`
	Usage      EmbeddingUsage    `json:"usage"`
	StatusCode int               `json:"-"`
}

// EmbeddingObject is a single embedding vector.
type EmbeddingObject struct {
	Object    string    `json:"object"` // "embedding"
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// EmbeddingUsage tracks token usage for embeddings.
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// TTSRequest is the inbound TTS request.
type TTSRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"` // mp3|opus|aac|flac|wav|pcm
	Speed          float64 `json:"speed,omitempty"`
}

// TTSResponse is what a provider returns for TTS.
type TTSResponse struct {
	Audio       []byte // raw audio bytes
	ContentType string // e.g. "audio/mpeg", "audio/wav"
	StatusCode  int
}

// STTRequest is the inbound STT request (multipart form).
type STTRequest struct {
	File           []byte // uploaded audio file bytes
	Filename       string
	Model          string
	Language       string
	Prompt         string
	ResponseFormat string // json|text|srt|verbose_json|vtt
	Temperature    float64
}

// STTResponse is what a provider returns for STT.
type STTResponse struct {
	Text       string `json:"text"`
	StatusCode int    `json:"-"`
	Body       []byte `json:"-"` // raw response body for passthrough
}
