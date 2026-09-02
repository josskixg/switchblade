package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func geminiRouter(chat chatHandler) chi.Router {
	r := chi.NewRouter()
	MountGeminiIngress(r, chat)
	return r
}

func postGemini(t *testing.T, chat chatHandler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	geminiRouter(chat).ServeHTTP(w, req)
	return w
}

// parseGeminiSSE returns the decoded payload of each `data:` frame. Gemini's
// SSE has no event: lines and no terminating sentinel.
func parseGeminiSSE(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
			t.Fatalf("undecodable frame %q: %v", line, err)
		}
		out = append(out, v)
	}
	return out
}

func geminiText(t *testing.T, frame map[string]any) string {
	t.Helper()
	candidates, ok := frame["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		t.Fatalf("frame has no candidates: %v", frame)
	}
	content := candidates[0].(map[string]any)["content"].(map[string]any)
	var sb strings.Builder
	for _, p := range content["parts"].([]any) {
		if text, ok := p.(map[string]any)["text"].(string); ok {
			sb.WriteString(text)
		}
	}
	return sb.String()
}

// ── routing ────────────────────────────────────────────────────────────────

func TestSplitGeminiMethod(t *testing.T) {
	cases := []struct{ in, model, method string }{
		{"gemini-2.5-pro:generateContent", "gemini-2.5-pro", "generateContent"},
		{"models/gemini-2.5-flash:streamGenerateContent", "gemini-2.5-flash", "streamGenerateContent"},
		{"gemini-2.5-pro", "gemini-2.5-pro", ""},
	}
	for _, c := range cases {
		model, method := splitGeminiMethod(c.in)
		if model != c.model || method != c.method {
			t.Errorf("splitGeminiMethod(%q) = (%q, %q), want (%q, %q)", c.in, model, method, c.model, c.method)
		}
	}
}

func TestGeminiUnknownMethodIs404(t *testing.T) {
	w := postGemini(t, &fakeChat{}, "/v1beta/models/gemini-2.5-pro:embedContent", `{}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	e := got["error"].(map[string]any)
	if e["status"] != "NOT_FOUND" || e["code"].(float64) != 404 {
		t.Errorf("error = %v", e)
	}
}

func TestGeminiAuthShim(t *testing.T) {
	var seen string
	h := GeminiAuthShim(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("x-api-key")
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/m:generateContent", nil)
	req.Header.Set("x-goog-api-key", "sk_header")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "sk_header" {
		t.Errorf("x-api-key = %q, want sk_header", seen)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1beta/models/m:generateContent?key=sk_query", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "sk_query" {
		t.Errorf("x-api-key = %q, want sk_query", seen)
	}

	// An explicit credential always wins over the Gemini aliases.
	req = httptest.NewRequest(http.MethodPost, "/v1beta/models/m:generateContent?key=sk_query", nil)
	req.Header.Set("x-api-key", "sk_real")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "sk_real" {
		t.Errorf("x-api-key = %q, want sk_real", seen)
	}
}

// ── request translation ────────────────────────────────────────────────────

func TestGeminiToChat_ContentsAndSystemInstruction(t *testing.T) {
	in := &geminiRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: "be terse"}}},
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: "hello"}}},
			{Role: "model", Parts: []geminiPart{{Text: "hi"}}},
			{Role: "user", Parts: []geminiPart{{Text: "again"}}},
		},
		GenerationConfig: &geminiGenConfig{MaxOutputTokens: 256, StopSequences: []string{"END"}},
	}
	out, err := geminiToChat("gemini-2.5-pro", in, false)
	if err != nil {
		t.Fatalf("geminiToChat: %v", err)
	}
	if out.Model != "gemini-2.5-pro" || out.MaxTokens != 256 {
		t.Errorf("request = %+v", out)
	}
	if len(out.Stop) != 1 || out.Stop[0] != "END" {
		t.Errorf("stop = %v", out.Stop)
	}

	wantRoles := []string{"system", "user", "assistant", "user"}
	if len(out.Messages) != len(wantRoles) {
		t.Fatalf("got %d messages, want %d: %+v", len(out.Messages), len(wantRoles), out.Messages)
	}
	for i, role := range wantRoles {
		if out.Messages[i].Role != role {
			t.Errorf("message %d role = %q, want %q", i, out.Messages[i].Role, role)
		}
	}
	if out.Messages[0].Content != "be terse" {
		t.Errorf("system = %v", out.Messages[0].Content)
	}
}

func TestGeminiToChat_FunctionCallAndResponse(t *testing.T) {
	in := &geminiRequest{
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: "weather?"}}},
			{Role: "model", Parts: []geminiPart{{FunctionCall: &geminiFunctionCall{
				Name: "get_weather", Args: json.RawMessage(`{"city":"Paris"}`)}}}},
			{Role: "user", Parts: []geminiPart{{FunctionResponse: &geminiFunctionResponse{
				Name: "get_weather", Response: json.RawMessage(`{"temp":18}`)}}}},
		},
		Tools: []geminiTool{{FunctionDeclarations: []geminiFunctionDecl{{
			Name:        "get_weather",
			Description: "look it up",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}}}},
		ToolConfig: &geminiToolConfig{FunctionCallingConfig: &struct {
			Mode string `json:"mode"`
		}{Mode: "ANY"}},
	}

	out, err := geminiToChat("gemini-2.5-pro", in, false)
	if err != nil {
		t.Fatalf("geminiToChat: %v", err)
	}
	if len(out.Messages) != 3 {
		t.Fatalf("messages = %+v", out.Messages)
	}
	call := out.Messages[1]
	if call.Role != "assistant" || len(call.ToolCalls) != 1 {
		t.Fatalf("assistant message = %+v", call)
	}
	if call.ToolCalls[0].Function.Arguments != `{"city":"Paris"}` {
		t.Errorf("arguments = %q", call.ToolCalls[0].Function.Arguments)
	}

	result := out.Messages[2]
	if result.Role != "tool" || result.Content != `{"temp":18}` {
		t.Fatalf("tool message = %+v", result)
	}
	// Gemini has no call id, so the synthesised one has to tie the pair together.
	if result.ToolCallID == "" || result.ToolCallID != call.ToolCalls[0].ID {
		t.Errorf("tool_call_id = %q, want %q", result.ToolCallID, call.ToolCalls[0].ID)
	}

	if out.ToolChoice != "required" {
		t.Errorf("tool_choice = %v, want required", out.ToolChoice)
	}
	if len(out.Tools) != 1 || out.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tools = %+v", out.Tools)
	}
}

func TestGeminiToChat_InlineDataBecomesDataURL(t *testing.T) {
	in := &geminiRequest{Contents: []geminiContent{{Role: "user", Parts: []geminiPart{
		{Text: "what is this"},
		{InlineData: &geminiBlob{MimeType: "image/webp", Data: "QUJD"}},
	}}}}
	out, err := geminiToChat("gemini-2.5-pro", in, false)
	if err != nil {
		t.Fatalf("geminiToChat: %v", err)
	}
	parts, ok := out.Messages[0].Content.([]oaiPart)
	if !ok || len(parts) != 2 {
		t.Fatalf("content = %#v", out.Messages[0].Content)
	}
	if parts[1].ImageURL == nil || parts[1].ImageURL.URL != "data:image/webp;base64,QUJD" {
		t.Errorf("image part = %+v", parts[1])
	}
}

func TestGeminiToChat_ResponseSchemaAndThinking(t *testing.T) {
	in := &geminiRequest{
		Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: "hi"}}}},
		GenerationConfig: &geminiGenConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   json.RawMessage(`{"type":"object"}`),
			ThinkingConfig: &struct {
				ThinkingBudget int `json:"thinkingBudget"`
			}{ThinkingBudget: 1024},
		},
	}
	out, err := geminiToChat("gemini-2.5-pro", in, false)
	if err != nil {
		t.Fatalf("geminiToChat: %v", err)
	}
	if out.ReasoningEffort != "low" {
		t.Errorf("reasoning_effort = %q, want low", out.ReasoningEffort)
	}
	format, ok := out.ResponseFormat.(map[string]any)
	if !ok || format["type"] != "json_schema" {
		t.Fatalf("response_format = %#v", out.ResponseFormat)
	}
}

func TestGeminiToChat_RejectsEmptyContents(t *testing.T) {
	if _, err := geminiToChat("gemini-2.5-pro", &geminiRequest{}, false); err == nil {
		t.Fatal("expected contents to be required")
	}
}

// ── end-to-end through the metered chat path ───────────────────────────────

func TestGeminiGenerateContent_UsesChatCompletionsPath(t *testing.T) {
	chat := &fakeChat{body: `{"id":"chatcmpl-1","model":"gemini-2.5-pro","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`}
	postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	if chat.gotPath != chatCompletionsPath {
		t.Errorf("inner path = %q, want %q", chat.gotPath, chatCompletionsPath)
	}
	var sent oaiRequest
	if err := json.Unmarshal(chat.gotBody, &sent); err != nil {
		t.Fatalf("inner body is not an OpenAI request: %v", err)
	}
	if sent.Model != "gemini-2.5-pro" || sent.Stream {
		t.Errorf("inner request = %+v", sent)
	}
}

func TestGeminiGenerateContent_BufferedResponse(t *testing.T) {
	chat := &fakeChat{body: `{
		"id":"chatcmpl-1","model":"gemini-2.5-pro",
		"choices":[{"message":{"content":"hello there"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}
	}`}
	w := postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if text := geminiText(t, got); text != "hello there" {
		t.Errorf("text = %q", text)
	}
	candidate := got["candidates"].([]any)[0].(map[string]any)
	if candidate["finishReason"] != "STOP" {
		t.Errorf("finishReason = %v", candidate["finishReason"])
	}
	if candidate["content"].(map[string]any)["role"] != "model" {
		t.Errorf("role = %v, want model", candidate["content"])
	}
	usage := got["usageMetadata"].(map[string]any)
	if usage["promptTokenCount"].(float64) != 5 ||
		usage["candidatesTokenCount"].(float64) != 2 ||
		usage["totalTokenCount"].(float64) != 7 {
		t.Errorf("usageMetadata = %v", usage)
	}
}

func TestGeminiGenerateContent_ToolCall(t *testing.T) {
	chat := &fakeChat{body: `{
		"id":"chatcmpl-1","model":"gemini-2.5-pro",
		"choices":[{"message":{"content":null,"tool_calls":[
			{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},
			"finish_reason":"tool_calls"}]
	}`}
	w := postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	candidate := got["candidates"].([]any)[0].(map[string]any)
	if candidate["finishReason"] != "STOP" {
		t.Errorf("finishReason = %v, want STOP", candidate["finishReason"])
	}
	parts := candidate["content"].(map[string]any)["parts"].([]any)
	fc, ok := parts[0].(map[string]any)["functionCall"].(map[string]any)
	if !ok {
		t.Fatalf("parts = %v", parts)
	}
	if fc["name"] != "get_weather" {
		t.Errorf("functionCall = %v", fc)
	}
	args, ok := fc["args"].(map[string]any)
	if !ok || args["city"] != "Paris" {
		t.Errorf("args = %#v, want an object", fc["args"])
	}
}

func TestGeminiGenerateContent_MaxTokensFinishReason(t *testing.T) {
	chat := &fakeChat{body: `{"id":"c","model":"m","choices":[{"message":{"content":"x"},"finish_reason":"length"}]}`}
	w := postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	candidate := got["candidates"].([]any)[0].(map[string]any)
	if candidate["finishReason"] != "MAX_TOKENS" {
		t.Errorf("finishReason = %v, want MAX_TOKENS", candidate["finishReason"])
	}
}

func TestGeminiStreamGenerateContent_SSE(t *testing.T) {
	chat := &fakeChat{sse: []string{
		"data: {\"id\":\"chatcmpl-s\",\"model\":\"gemini-2.5-pro\",\"choices\":[{\"delta\":{\"content\":\"Hel\"},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n",
		"data: [DONE]\n\n",
	}}
	w := postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if strings.Contains(w.Body.String(), "[DONE]") {
		t.Error("Gemini streams carry no [DONE] sentinel")
	}
	if strings.Contains(w.Body.String(), "event:") {
		t.Error("Gemini streams carry no event: lines")
	}

	frames := parseGeminiSSE(t, w.Body.String())
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want 3: %v", len(frames), frames)
	}

	var text strings.Builder
	for _, f := range frames {
		text.WriteString(geminiText(t, f))
	}
	if text.String() != "Hello" {
		t.Errorf("streamed text = %q, want Hello", text.String())
	}

	// finishReason and usageMetadata land on the terminal frame only.
	for i, f := range frames[:2] {
		candidate := f["candidates"].([]any)[0].(map[string]any)
		if _, ok := candidate["finishReason"]; ok {
			t.Errorf("frame %d carries a finishReason", i)
		}
		if _, ok := f["usageMetadata"]; ok {
			t.Errorf("frame %d carries usageMetadata", i)
		}
	}
	last := frames[2]
	candidate := last["candidates"].([]any)[0].(map[string]any)
	if candidate["finishReason"] != "STOP" {
		t.Errorf("terminal finishReason = %v", candidate["finishReason"])
	}
	usage := last["usageMetadata"].(map[string]any)
	if usage["promptTokenCount"].(float64) != 4 || usage["candidatesTokenCount"].(float64) != 2 {
		t.Errorf("usageMetadata = %v", usage)
	}
	if last["modelVersion"] != "gemini-2.5-pro" {
		t.Errorf("modelVersion = %v", last["modelVersion"])
	}
}

func TestGeminiStreamGenerateContent_JSONArray(t *testing.T) {
	chat := &fakeChat{sse: []string{
		"data: {\"id\":\"chatcmpl-s\",\"model\":\"gemini-2.5-pro\",\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
		"data: [DONE]\n\n",
	}}
	w := postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:streamGenerateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var frames []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &frames); err != nil {
		t.Fatalf("array form is not valid JSON (%v): %s", err, w.Body.String())
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if geminiText(t, frames[0]) != "hi" {
		t.Errorf("first frame = %v", frames[0])
	}
}

func TestGeminiStream_ToolCallsAssembledOnFinalFrame(t *testing.T) {
	chat := &fakeChat{sse: []string{
		"data: {\"id\":\"chatcmpl-t\",\"model\":\"gemini-2.5-pro\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"city\\\":\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"Paris\\\"}\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n",
		"data: [DONE]\n\n",
	}}
	w := postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	frames := parseGeminiSSE(t, w.Body.String())
	// Gemini has no partial-argument frame, so nothing is emitted until the
	// arguments are complete.
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1: %v", len(frames), frames)
	}
	parts := frames[0]["candidates"].([]any)[0].(map[string]any)["content"].(map[string]any)["parts"].([]any)
	fc, ok := parts[0].(map[string]any)["functionCall"].(map[string]any)
	if !ok {
		t.Fatalf("parts = %v", parts)
	}
	if fc["name"] != "get_weather" {
		t.Errorf("functionCall = %v", fc)
	}
	args := fc["args"].(map[string]any)
	if args["city"] != "Paris" {
		t.Errorf("args = %v, want the fragments reassembled", args)
	}
}

func TestGeminiStream_EmptyStreamStillTerminates(t *testing.T) {
	chat := &fakeChat{sse: []string{"data: [DONE]\n\n"}}
	w := postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	frames := parseGeminiSSE(t, w.Body.String())
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	candidate := frames[0]["candidates"].([]any)[0].(map[string]any)
	if candidate["finishReason"] != "STOP" {
		t.Errorf("finishReason = %v", candidate["finishReason"])
	}
}

func TestGeminiStream_RequestsUsage(t *testing.T) {
	chat := &fakeChat{sse: []string{"data: [DONE]\n\n"}}
	postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	var sent oaiRequest
	if err := json.Unmarshal(chat.gotBody, &sent); err != nil {
		t.Fatalf("decode inner: %v", err)
	}
	if !sent.Stream {
		t.Error("stream flag was not propagated to the chat path")
	}
	if sent.StreamOptions == nil || !sent.StreamOptions.IncludeUsage {
		t.Error("streaming requests must ask upstream for a usage block")
	}
}

func TestGeminiGenerateContent_ErrorEnvelope(t *testing.T) {
	chat := &fakeChat{status: http.StatusPaymentRequired, body: `{"error":"insufficient credit","code":"no_credit"}`}
	w := postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	e := got["error"].(map[string]any)
	if e["code"].(float64) != 402 || e["status"] != "PERMISSION_DENIED" || e["message"] != "insufficient credit" {
		t.Errorf("error = %v", e)
	}
}

func TestGeminiGenerateContent_RejectsInvalidBody(t *testing.T) {
	chat := &fakeChat{}
	w := postGemini(t, chat, "/v1beta/models/gemini-2.5-pro:generateContent", `{"contents":`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if chat.gotBody != nil {
		t.Error("a malformed request must not reach the chat path")
	}
}
