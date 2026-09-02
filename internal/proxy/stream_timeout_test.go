package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"switchblade/internal/config"
	"switchblade/internal/providers"
)

// withProviderTimeout pins the provider timeout for one test, restoring the
// global config afterwards.
func withProviderTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := config.C
	config.C = &config.Config{ProviderRequestTimeoutMS: int(d.Milliseconds())}
	t.Cleanup(func() { config.C = old })
}

// sseServer streams frames frames spaced interval apart after flushing
// headers immediately, the way a real generation trickles tokens.
func sseServer(t *testing.T, frames int, interval time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for i := 0; i < frames; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(interval):
			}
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"tok%d\"}}]}\n\n", i)
			w.(http.Flusher).Flush()
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestServeChat_LongStreamOutlivesProviderTimeout is the regression guard for
// the 120s mid-stream truncation: a generation that keeps emitting frames must
// run past the provider timeout, which bounds time-to-response only.
func TestServeChat_LongStreamOutlivesProviderTimeout(t *testing.T) {
	withProviderTimeout(t, 250*time.Millisecond)

	// 12 frames at 50ms ≈ 600ms of streaming, well past the 250ms timeout.
	srv := sseServer(t, 12, 50*time.Millisecond)

	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(&liveProvider{url: srv.URL}, meter).
		ServeChat(w, chatRequest(`{"model":"any","stream":true}`))

	got := w.Body.String()
	for i := 0; i < 12; i++ {
		if !strings.Contains(got, fmt.Sprintf("tok%d", i)) {
			t.Fatalf("frame %d missing — the stream was cut mid-generation:\n%s", i, got)
		}
	}
	if !strings.Contains(got, "[DONE]") {
		t.Errorf("terminator missing:\n%s", got)
	}
	if ev := meter.only(t); ev.Status != "success" {
		t.Errorf("status = %q (err %q), want success", ev.Status, ev.ErrMessage)
	}
}

// A provider that goes silent mid-stream must not pin the handler forever:
// the idle watchdog has to end the request once no frames arrive.
func TestServeChat_SilentStreamIsTerminated(t *testing.T) {
	withProviderTimeout(t, 200*time.Millisecond)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done() // silence until the proxy gives up
	}))
	t.Cleanup(srv.Close)

	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	start := time.Now()
	routerWith(&liveProvider{url: srv.URL}, meter).
		ServeChat(w, chatRequest(`{"model":"any","stream":true}`))

	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("handler held for %v after the provider stalled", elapsed)
	}
	if ev := meter.only(t); ev.ErrMessage == "" {
		t.Error("stalled stream left no error trace")
	}
}

// TestExecute_StreamOutlivesAttemptTimeout closes the same hole on the
// fallback path: the per-attempt deadline may bound the wait for a response,
// never the stream that follows it.
func TestExecute_StreamOutlivesAttemptTimeout(t *testing.T) {
	srv := sseServer(t, 8, 40*time.Millisecond) // ≈320ms of frames

	reg := &providers.Registry{}
	reg.Register(&liveProvider{url: srv.URL})
	fe := NewFallbackExecutor(reg, nil, 0, 100) // 100ms attempt timeout

	out, err := fe.Execute(context.Background(), &providers.ChatRequest{Stream: true}, []string{"any"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Response.BodyStream == nil {
		t.Fatal("no BodyStream — the streaming path was not taken")
	}
	defer out.Response.BodyStream.Close()

	got, err := io.ReadAll(out.Response.BodyStream)
	if err != nil {
		t.Fatalf("stream died %v — the attempt deadline cut it mid-generation", err)
	}
	if !strings.Contains(string(got), "tok7") || !strings.Contains(string(got), "[DONE]") {
		t.Errorf("stream incomplete:\n%s", got)
	}
}
