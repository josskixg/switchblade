package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var allowedHeaders = map[string]bool{
	"Accept":          true,
	"Accept-Encoding": true,
	"Accept-Language": true,
	"Authorization":   true,
	"Cache-Control":   true,
	"Content-Length":  true,
	"Content-Type":    true,
	"Idempotency-Key": true,
	"User-Agent":      true,
	"X-Request-Id":    true,
}

// Client forwards HTTP requests to a remote relay server.
type Client struct {
	remoteURL string
	secret    string
	http      *http.Client
}

// NewClient creates a relay client targeting remoteURL with the given secret.
func NewClient(remoteURL, secret string) *Client {
	return &Client{
		remoteURL: remoteURL,
		secret:    secret,
		http:      &http.Client{Timeout: 60 * time.Second},
	}
}

// Forward clones r, rewrites its URL to the remote relay, injects the secret
// header, and returns the relay's response.
func (c *Client) Forward(ctx context.Context, r *http.Request) (*http.Response, error) {
	target, err := url.Parse(c.remoteURL)
	if err != nil {
		return nil, fmt.Errorf("relay: invalid remote URL: %w", err)
	}

	// Rewrite destination, keep path + query from original request.
	target.Path = r.URL.Path
	target.RawQuery = r.URL.RawQuery

	out, err := http.NewRequestWithContext(ctx, r.Method, target.String(), r.Body)
	if err != nil {
		return nil, fmt.Errorf("relay: build request: %w", err)
	}

	// Copy only whitelisted headers from original request.
	for k, vv := range r.Header {
		if !allowedHeaders[k] {
			continue
		}
		for _, v := range vv {
			out.Header.Add(k, v)
		}
	}

	if c.secret != "" {
		out.Header.Set("X-Relay-Secret", c.secret)
	}

	return c.http.Do(out)
}
