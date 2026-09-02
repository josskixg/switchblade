// Package providers defines the Provider interface and shared types.
package providers

import (
	"context"
	"fmt"
	"io"
)

// Valid account tiers — must match tier_config table.
const (
	TierSubscription = "subscription"
	TierCheap        = "cheap"
	TierFree         = "free"
)

// ValidTiers is the set of allowed tier values.
var ValidTiers = map[string]bool{
	TierSubscription: true,
	TierCheap:        true,
	TierFree:         true,
}

// ValidateTier returns an error if tier is not a recognized value.
func ValidateTier(tier string) error {
	if !ValidTiers[tier] {
		return fmt.Errorf("invalid tier %q: must be one of subscription, cheap, free", tier)
	}
	return nil
}

// Provider is the interface every AI backend must implement.
type Provider interface {
	Name() string
	OwnsModel(model string) bool
	Chat(ctx context.Context, acc *Account, req *ChatRequest) (*ChatResponse, error)
	Healthy(ctx context.Context, acc *Account) bool
}

// Account mirrors the accounts DB table.
type Account struct {
	ID             int64
	Provider       string
	Email          string
	Password       string // XOR+base64 encrypted
	Status         string // pending | active | exhausted | error | disabled
	Enabled        bool
	Tokens         string // JSON blob: access_token, refresh_token, etc.
	QuotaLimit     float64
	QuotaRemaining float64
	QuotaResetAt   int64
	LastUsedAt     int64
	LastLoginAt    int64
	ErrorMessage   string
	Metadata       string // arbitrary JSON
	Tier           string // subscription | cheap | free
	CreatedAt      int64
	UpdatedAt      int64
}

// Message is a single chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the inbound request passed to a provider.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Body        []byte    `json:"-"` // raw request body for passthrough
}

// ChatResponse is what a provider returns.
//
// Exactly one of Body or BodyStream carries the payload. BodyStream is set when
// the client asked for a stream and the upstream succeeded; the router pipes it
// through and is responsible for closing it. Otherwise Body holds the full
// buffered reply.
type ChatResponse struct {
	Body       []byte
	BodyStream io.ReadCloser
	StatusCode int
	Stream     bool
	Headers    map[string]string
}

// ProviderError is a typed error from a provider.
type ProviderError struct {
	Code      int
	Message   string
	Provider  string
	Retryable bool
}

func (e *ProviderError) Error() string {
	return e.Provider + ": " + e.Message
}

// Embedder is the optional interface for embedding-capable providers.
type Embedder interface {
	Embeddings(ctx context.Context, acc *Account, req *EmbeddingsRequest) (*EmbeddingsResponse, error)
}

// TTSProvider is the optional interface for text-to-speech-capable providers.
type TTSProvider interface {
	TTS(ctx context.Context, acc *Account, req *TTSRequest) (*TTSResponse, error)
}

// STTProvider is the optional interface for speech-to-text-capable providers.
type STTProvider interface {
	STT(ctx context.Context, acc *Account, req *STTRequest) (*STTResponse, error)
}
