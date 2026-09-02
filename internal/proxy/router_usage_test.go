package proxy

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"switchblade/internal/providers"
)

// streamingStub returns a canned SSE stream and captures the outbound body.
func streamingStub(name, upstream string, gotBody *[]byte) *stubProvider {
	return &stubProvider{name: name, fn: func(req *providers.ChatRequest) (*providers.ChatResponse, error) {
		*gotBody = append((*gotBody)[:0], req.Body...)
		return &providers.ChatResponse{
			StatusCode: 200,
			Stream:     true,
			BodyStream: io.NopCloser(strings.NewReader(upstream)),
			Headers:    map[string]string{"Content-Type": "text/event-stream"},
		}, nil
	}}
}

func includeUsageOf(t *testing.T, body []byte) (present, value bool) {
	t.Helper()
	var probe struct {
		StreamOptions *struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		t.Fatalf("outbound body is not JSON: %v\n%s", err, body)
	}
	if probe.StreamOptions == nil {
		return false, false
	}
	return true, probe.StreamOptions.IncludeUsage
}

const usageChunk = `data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":34,"total_tokens":46}}`

var usageStream = "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
	usageChunk + "\n\n" +
	"data: [DONE]\n\n"

// TestServeChat_StreamInjectsIncludeUsage pins the revenue fix: a client that
// streams without asking for usage must still produce a metered request, and
// the frame the proxy requested for itself must not reach that client.
func TestServeChat_StreamInjectsIncludeUsage(t *testing.T) {
	var outbound []byte
	p := streamingStub("stub", usageStream, &outbound)

	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(p, meter).ServeChat(w, chatRequest(`{"model":"any","stream":true}`))

	if present, value := includeUsageOf(t, outbound); !present || !value {
		t.Errorf("outbound body lacks stream_options.include_usage=true: %s", outbound)
	}

	got := w.Body.String()
	if strings.Contains(got, `"usage":{"prompt_tokens":12`) {
		t.Errorf("injected usage frame leaked to a client that never asked for it:\n%s", got)
	}
	if !strings.Contains(got, `"content":"hi"`) || !strings.Contains(got, "[DONE]") {
		t.Errorf("content or terminator missing from relayed stream:\n%s", got)
	}

	ev := meter.only(t)
	if ev.Usage.TotalTokens != 46 || ev.Usage.PromptTokens != 12 {
		t.Errorf("usage = %+v, want the scraped {12 34 46}", ev.Usage)
	}
	if ev.Usage.Estimated {
		t.Error("scraped usage marked as an estimate")
	}
}

// A client that opted in gets the frame it asked for, untouched.
func TestServeChat_StreamClientRequestedUsageIsRelayed(t *testing.T) {
	var outbound []byte
	p := streamingStub("stub", usageStream, &outbound)

	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(p, meter).ServeChat(w,
		chatRequest(`{"model":"any","stream":true,"stream_options":{"include_usage":true}}`))

	if present, value := includeUsageOf(t, outbound); !present || !value {
		t.Errorf("client's stream_options were dropped: %s", outbound)
	}
	if got := w.Body.String(); !strings.Contains(got, `"usage":{"prompt_tokens":12`) {
		t.Errorf("usage frame the client asked for was stripped:\n%s", got)
	}
	if ev := meter.only(t); ev.Usage.TotalTokens != 46 {
		t.Errorf("usage = %+v, want 46 total", ev.Usage)
	}
}

// Anthropic-dialect bodies reject unknown top-level fields, so nothing may be
// injected when the target does not speak the OpenAI streaming dialect.
func TestServeChat_StreamNoInjectionForNonOpenAIDialect(t *testing.T) {
	anthropicStream := "event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":1}}}\n\n" +
		"event: message_delta\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":9}}\n\n"

	var outbound []byte
	p := streamingStub("anthropic", anthropicStream, &outbound)

	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(p, meter).ServeChat(w, chatRequest(`{"model":"any","stream":true}`))

	if present, _ := includeUsageOf(t, outbound); present {
		t.Errorf("stream_options injected into a non-OpenAI dialect body: %s", outbound)
	}
	if got := w.Body.String(); got != anthropicStream {
		t.Errorf("anthropic stream altered in transit:\n got %q\nwant %q", got, anthropicStream)
	}
	if ev := meter.only(t); ev.Usage.PromptTokens != 5 || ev.Usage.CompletionTokens != 9 {
		t.Errorf("usage = %+v, want {5 9 14}", ev.Usage)
	}
}

// TestServeChat_UnbilledStreamIsEstimated is the belt-and-braces guard: an
// upstream that never reports usage must produce an estimated charge rather
// than free inference.
func TestServeChat_UnbilledStreamIsEstimated(t *testing.T) {
	silent := "data: {\"choices\":[{\"delta\":{\"content\":\"a long enough completion\"}}]}\n\n" +
		"data: [DONE]\n\n"

	var outbound []byte
	p := streamingStub("stub", silent, &outbound)

	meter := &capturingMeter{}
	w := httptest.NewRecorder()
	routerWith(p, meter).ServeChat(w, chatRequest(`{"model":"any","stream":true}`))

	ev := meter.only(t)
	if !ev.Usage.Estimated {
		t.Fatal("zero-usage stream not flagged as estimated — the traffic is unbilled")
	}
	if ev.Usage.TotalTokens == 0 {
		t.Error("estimated usage is still zero — the tenant is not charged")
	}
	if ev.Status != "success" {
		t.Errorf("status = %q, want success", ev.Status)
	}
}
