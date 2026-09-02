package providers

import (
	"io"
	"net/http"
)

// Exchange performs httpReq and converts the upstream reply into a ChatResponse.
//
// When stream is true and the upstream returned a success status, the body is
// handed back unread as BodyStream so the router can pipe bytes to the client as
// they arrive — the caller owns closing it. Every other case (buffered request,
// or an error status we want to inspect and log) is read into Body and closed
// here, so fallback and caching keep working exactly as before.
func Exchange(httpReq *http.Request, providerName string, stream bool) (*ChatResponse, error) {
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Code: 502, Message: err.Error(), Provider: providerName, Retryable: true}
	}

	var headers map[string]string
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		headers = map[string]string{"Content-Type": ct}
	}

	if stream && resp.StatusCode < 400 {
		return &ChatResponse{
			StatusCode: resp.StatusCode,
			BodyStream: resp.Body,
			Stream:     true,
			Headers:    headers,
		}, nil
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{Code: 502, Message: err.Error(), Provider: providerName, Retryable: true}
	}
	return &ChatResponse{StatusCode: resp.StatusCode, Body: body, Headers: headers}, nil
}
