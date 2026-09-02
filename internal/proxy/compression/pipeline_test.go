package compression

import (
	"encoding/json"
	"testing"
)

func TestCompress_NoOp(t *testing.T) {
	body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`)
	out, _ := Compress(body)
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	// must still be valid JSON
	var v any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestCompress_Stats(t *testing.T) {
	body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello world"}]}`)
	_, stats := Compress(body)
	if stats.Total < 0 {
		t.Fatalf("stats.Total should be >= 0, got %d", stats.Total)
	}
}

func TestCompress_InvalidJSON(t *testing.T) {
	body := []byte(`not json at all`)
	out, _ := Compress(body)
	if string(out) != string(body) {
		t.Fatalf("expected original body back on invalid JSON, got %q", out)
	}
}
