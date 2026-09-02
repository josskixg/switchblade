package providers

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sseServer emits frames one at a time, blocking until the caller signals, so a
// test can prove Exchange hands back a live body rather than one that only
// resolves after the upstream has finished.
func sseServer(t *testing.T, release <-chan struct{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f := w.(http.Flusher)

		io.WriteString(w, "data: first\n\n")
		f.Flush()

		<-release // hold the connection open

		io.WriteString(w, "data: second\n\n")
		f.Flush()
	}))
}

func TestExchange_StreamReturnsLiveBody(t *testing.T) {
	release := make(chan struct{})
	srv := sseServer(t, release)
	defer srv.Close()
	defer close(release)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := Exchange(req, "test", true)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	defer resp.BodyStream.Close()

	if resp.BodyStream == nil {
		t.Fatal("BodyStream is nil — streaming request was buffered")
	}
	if resp.Body != nil {
		t.Errorf("Body = %q, want nil for a streaming response", resp.Body)
	}
	if !resp.Stream {
		t.Error("Stream flag not set")
	}

	// The first frame must be readable while the server is still holding the
	// connection open. If Exchange had buffered, this read would block until
	// release is closed and the deadline below would fire.
	done := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(resp.BodyStream).ReadString('\n')
		done <- line
	}()

	select {
	case line := <-done:
		if strings.TrimSpace(line) != "data: first" {
			t.Errorf("first frame = %q, want \"data: first\"", strings.TrimSpace(line))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the first frame — response is buffered, not streamed")
	}
}

func TestExchange_NonStreamBuffers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := Exchange(req, "test", false)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if resp.BodyStream != nil {
		t.Error("BodyStream set for a non-streaming request")
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Errorf("Body = %q", resp.Body)
	}
	if resp.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q", resp.Headers["Content-Type"])
	}
}

// TestExchange_ErrorStatusBuffersEvenWhenStreaming keeps fallback and logging
// working: an upstream failure must be readable, not handed back as an opaque
// stream the router would blindly relay to the client.
func TestExchange_ErrorStatusBuffersEvenWhenStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":"rate limited"}`)
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := Exchange(req, "test", true)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if resp.BodyStream != nil {
		t.Error("error response was streamed — it must be buffered so it can be inspected")
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want 429", resp.StatusCode)
	}
	if string(resp.Body) != `{"error":"rate limited"}` {
		t.Errorf("Body = %q", resp.Body)
	}
}

func TestExchange_DialFailureIsRetryable(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:1/nope", nil)
	_, err := Exchange(req, "test", false)
	if err == nil {
		t.Fatal("expected an error dialing a closed port")
	}
	pe, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error type = %T, want *ProviderError", err)
	}
	if !pe.Retryable {
		t.Error("dial failure should be retryable so fallback moves to the next provider")
	}
}
