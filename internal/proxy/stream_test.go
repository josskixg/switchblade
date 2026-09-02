package proxy

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// flushRecorder counts Flush calls so a test can prove bytes leave the handler
// during the stream rather than all at once when it returns.
type flushRecorder struct {
	*httptest.ResponseRecorder
	flushes    int
	seenAtLast int // body length observed at the most recent flush
}

func (f *flushRecorder) Flush() {
	f.flushes++
	f.seenAtLast = f.Body.Len()
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func TestPipeStream_OpenAIUsageAndPassthrough(t *testing.T) {
	// OpenAI reports usage once, in the final chunk before [DONE].
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		``,
		`data: {"choices":[{"delta":{}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	w := newFlushRecorder()
	usage, written, err := pipeStream(w, strings.NewReader(stream), streamOpts{})
	if err != nil {
		t.Fatalf("pipeStream: %v", err)
	}

	if got := w.Body.String(); got != stream {
		t.Errorf("body was altered in transit:\n got %q\nwant %q", got, stream)
	}
	if written != int64(len(stream)) {
		t.Errorf("written = %d, want %d", written, len(stream))
	}
	if usage.PromptTokens != 11 || usage.CompletionTokens != 7 || usage.TotalTokens != 18 {
		t.Errorf("usage = %+v, want {11 7 18}", usage)
	}
}

func TestPipeStream_AnthropicSplitUsage(t *testing.T) {
	// Anthropic splits usage: input_tokens in message_start, output_tokens
	// accumulating across message_delta frames. The last non-zero value of each
	// field is the right answer.
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":42,"output_tokens":1}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"text":"hi"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","usage":{"output_tokens":99}}`,
		``,
	}, "\n")

	w := newFlushRecorder()
	usage, _, err := pipeStream(w, strings.NewReader(stream), streamOpts{})
	if err != nil {
		t.Fatalf("pipeStream: %v", err)
	}
	if usage.PromptTokens != 42 {
		t.Errorf("PromptTokens = %d, want 42", usage.PromptTokens)
	}
	if usage.CompletionTokens != 99 {
		t.Errorf("CompletionTokens = %d, want 99", usage.CompletionTokens)
	}
	// Neither frame carried a total, so it is derived.
	if usage.TotalTokens != 141 {
		t.Errorf("TotalTokens = %d, want 141", usage.TotalTokens)
	}
}

// TestPipeStream_FlushesIncrementally is the regression guard for the behaviour
// that motivated streaming at all: a client must see early frames before the
// generation finishes, not one buffered dump at the end.
func TestPipeStream_FlushesIncrementally(t *testing.T) {
	stream := "data: a\n\ndata: b\n\ndata: c\n\n"

	w := newFlushRecorder()
	if _, _, err := pipeStream(w, strings.NewReader(stream), streamOpts{}); err != nil {
		t.Fatalf("pipeStream: %v", err)
	}

	// Six lines (three data lines, three frame terminators) → six flushes.
	if w.flushes < 6 {
		t.Errorf("flushes = %d, want at least 6 — output is being buffered", w.flushes)
	}
	if w.seenAtLast != len(stream) {
		t.Errorf("final flush saw %d bytes, want %d", w.seenAtLast, len(stream))
	}
}

func TestPipeStream_MalformedFramesAreForwarded(t *testing.T) {
	// A frame we cannot parse must still reach the client untouched; usage
	// scraping is best-effort and must never eat the payload.
	stream := "data: {not json\n\ndata: {\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4}}\n\n"

	w := newFlushRecorder()
	usage, _, err := pipeStream(w, strings.NewReader(stream), streamOpts{})
	if err != nil {
		t.Fatalf("pipeStream: %v", err)
	}
	if w.Body.String() != stream {
		t.Errorf("body altered: got %q", w.Body.String())
	}
	if usage.PromptTokens != 3 || usage.CompletionTokens != 4 || usage.TotalTokens != 7 {
		t.Errorf("usage = %+v, want {3 4 7}", usage)
	}
}

func TestExtractUsage_BufferedResponse(t *testing.T) {
	body := []byte(`{"id":"x","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":6,"total_tokens":11}}`)
	u := extractUsage(body)
	if u.PromptTokens != 5 || u.CompletionTokens != 6 || u.TotalTokens != 11 {
		t.Errorf("usage = %+v, want {5 6 11}", u)
	}

	if got := extractUsage([]byte(`{"error":"boom"}`)); got.TotalTokens != 0 {
		t.Errorf("usage on error body = %+v, want zero", got)
	}
}
