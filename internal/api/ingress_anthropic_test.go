package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeChat stands in for *proxy.Router: it records the translated request the
// ingress produced and replays a canned OpenAI-shaped response.
type fakeChat struct {
	gotBody   []byte
	gotPath   string
	gotHeader http.Header

	status int
	body   string
	sse    []string
}

func (f *fakeChat) ServeChat(w http.ResponseWriter, r *http.Request) {
	f.gotBody, _ = io.ReadAll(r.Body)
	f.gotPath = r.URL.Path
	f.gotHeader = r.Header.Clone()

	if len(f.sse) > 0 {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, line := range f.sse {
			_, _ = w.Write([]byte(line))
		}
		return
	}

	status := f.status
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(f.body))
}

type sseEvent struct {
	name string
	data map[string]any
}

func parseSSE(t *testing.T, body string) []sseEvent {
	t.Helper()
	var events []sseEvent
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		var ev sseEvent
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev.data); err != nil {
					t.Fatalf("undecodable SSE data %q: %v", line, err)
				}
			}
		}
		if ev.name != "" {
			events = append(events, ev)
		}
	}
	return events
}

func eventNames(events []sseEvent) []string {
	names := make([]string, 0, len(events))
	for _, e := range events {
		names = append(names, e.name)
	}
	return names
}

func postMessages(t *testing.T, chat chatHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleAnthropicMessages(chat)(w, req)
	return w
}

// ── request translation ────────────────────────────────────────────────────

func TestAnthropicToChat_SystemAndMultiTurn(t *testing.T) {
	in := &anthropicRequest{
		Model:         "claude-sonnet-4-5",
		MaxTokens:     1024,
		System:        json.RawMessage(`"be terse"`),
		StopSequences: []string{"STOP"},
		Messages: []anthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"hi"}]`)},
			{Role: "user", Content: json.RawMessage(`"and again"`)},
		},
	}

	out, err := anthropicToChat(in)
	if err != nil {
		t.Fatalf("anthropicToChat: %v", err)
	}
	if out.MaxTokens != 1024 {
		t.Errorf("max_tokens = %d, want 1024", out.MaxTokens)
	}
	if len(out.Stop) != 1 || out.Stop[0] != "STOP" {
		t.Errorf("stop = %v, want [STOP]", out.Stop)
	}
	if len(out.Messages) != 4 {
		t.Fatalf("got %d messages, want 4: %+v", len(out.Messages), out.Messages)
	}
	if out.Messages[0].Role != "system" || out.Messages[0].Content != "be terse" {
		t.Errorf("system message = %+v", out.Messages[0])
	}
	want := []string{"system", "user", "assistant", "user"}
	for i, role := range want {
		if out.Messages[i].Role != role {
			t.Errorf("message %d role = %q, want %q", i, out.Messages[i].Role, role)
		}
	}
	if out.Messages[2].Content != "hi" {
		t.Errorf("assistant content = %v, want a plain string", out.Messages[2].Content)
	}
}

func TestAnthropicToChat_SystemBlockArray(t *testing.T) {
	in := &anthropicRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 16,
		System:    json.RawMessage(`[{"type":"text","text":"one"},{"type":"text","text":"two"}]`),
		Messages:  []anthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}
	out, err := anthropicToChat(in)
	if err != nil {
		t.Fatalf("anthropicToChat: %v", err)
	}
	if out.Messages[0].Content != "one\n\ntwo" {
		t.Errorf("system = %q, want %q", out.Messages[0].Content, "one\n\ntwo")
	}
}

func TestAnthropicToChat_ToolUseAndToolResult(t *testing.T) {
	in := &anthropicRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 64,
		Messages: []anthropicMessage{
			{Role: "user", Content: json.RawMessage(`"weather?"`)},
			{Role: "assistant", Content: json.RawMessage(
				`[{"type":"text","text":"checking"},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Paris"}}]`)},
			{Role: "user", Content: json.RawMessage(
				`[{"type":"tool_result","tool_use_id":"toolu_1","content":"18C"},{"type":"text","text":"thanks"}]`)},
		},
		Tools: []anthropicTool{{
			Name:        "get_weather",
			Description: "look up weather",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		}},
		ToolChoice: &anthropicToolChoice{Type: "tool", Name: "get_weather", DisableParallelToolUse: true},
	}

	out, err := anthropicToChat(in)
	if err != nil {
		t.Fatalf("anthropicToChat: %v", err)
	}

	// The tool result has to precede the user turn it was bundled with.
	roles := make([]string, len(out.Messages))
	for i, m := range out.Messages {
		roles[i] = m.Role
	}
	wantRoles := []string{"user", "assistant", "tool", "user"}
	if strings.Join(roles, ",") != strings.Join(wantRoles, ",") {
		t.Fatalf("roles = %v, want %v", roles, wantRoles)
	}

	assistant := out.Messages[1]
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("assistant tool_calls = %d, want 1", len(assistant.ToolCalls))
	}
	tc := assistant.ToolCalls[0]
	if tc.ID != "toolu_1" || tc.Type != "function" || tc.Function.Name != "get_weather" {
		t.Errorf("tool call = %+v", tc)
	}
	if tc.Function.Arguments != `{"city":"Paris"}` {
		t.Errorf("arguments = %q, want the object serialised as a string", tc.Function.Arguments)
	}
	if assistant.Content != "checking" {
		t.Errorf("assistant content = %v", assistant.Content)
	}

	toolMsg := out.Messages[2]
	if toolMsg.ToolCallID != "toolu_1" || toolMsg.Content != "18C" {
		t.Errorf("tool message = %+v", toolMsg)
	}

	if len(out.Tools) != 1 || out.Tools[0].Function.Name != "get_weather" {
		t.Fatalf("tools = %+v", out.Tools)
	}
	choice, ok := out.ToolChoice.(map[string]any)
	if !ok || choice["type"] != "function" {
		t.Fatalf("tool_choice = %#v", out.ToolChoice)
	}
	if out.ParallelToolCalls == nil || *out.ParallelToolCalls {
		t.Errorf("parallel_tool_calls = %v, want false", out.ParallelToolCalls)
	}
}

func TestAnthropicToChat_ToolChoiceModes(t *testing.T) {
	cases := map[string]any{
		"auto": "auto",
		"any":  "required",
		"none": "none",
	}
	for kind, want := range cases {
		in := &anthropicRequest{
			Model:      "claude-sonnet-4-5",
			MaxTokens:  8,
			Messages:   []anthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
			ToolChoice: &anthropicToolChoice{Type: kind},
		}
		out, err := anthropicToChat(in)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if out.ToolChoice != want {
			t.Errorf("tool_choice for %q = %v, want %v", kind, out.ToolChoice, want)
		}
	}
}

func TestAnthropicToChat_ImageBlock(t *testing.T) {
	in := &anthropicRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 8,
		Messages: []anthropicMessage{{Role: "user", Content: json.RawMessage(
			`[{"type":"text","text":"what is this"},{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"QUJD"}}]`)}},
	}
	out, err := anthropicToChat(in)
	if err != nil {
		t.Fatalf("anthropicToChat: %v", err)
	}
	parts, ok := out.Messages[0].Content.([]oaiPart)
	if !ok {
		t.Fatalf("content = %#v, want a part array", out.Messages[0].Content)
	}
	if len(parts) != 2 || parts[1].ImageURL == nil {
		t.Fatalf("parts = %+v", parts)
	}
	if parts[1].ImageURL.URL != "data:image/jpeg;base64,QUJD" {
		t.Errorf("image url = %q", parts[1].ImageURL.URL)
	}
}

func TestAnthropicToChat_ThinkingBecomesReasoningEffort(t *testing.T) {
	in := &anthropicRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 8,
		Messages:  []anthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Thinking:  &anthropicThinking{Type: "enabled", BudgetTokens: 16384},
	}
	out, err := anthropicToChat(in)
	if err != nil {
		t.Fatalf("anthropicToChat: %v", err)
	}
	if out.ReasoningEffort != "high" {
		t.Errorf("reasoning_effort = %q, want high", out.ReasoningEffort)
	}
}

func TestAnthropicToChat_RejectsMissingMaxTokens(t *testing.T) {
	in := &anthropicRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []anthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}
	if _, err := anthropicToChat(in); err == nil {
		t.Fatal("expected max_tokens to be required")
	}
}

// ── end-to-end through the metered chat path ───────────────────────────────

func TestAnthropicMessages_UsesChatCompletionsPath(t *testing.T) {
	chat := &fakeChat{body: `{"id":"chatcmpl-1","model":"m","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`}
	postMessages(t, chat, `{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	if chat.gotPath != chatCompletionsPath {
		t.Errorf("inner path = %q, want %q", chat.gotPath, chatCompletionsPath)
	}
	if ct := chat.gotHeader.Get("Content-Type"); ct != "application/json" {
		t.Errorf("inner Content-Type = %q", ct)
	}
	var sent oaiRequest
	if err := json.Unmarshal(chat.gotBody, &sent); err != nil {
		t.Fatalf("inner body is not an OpenAI request: %v", err)
	}
	if sent.Model != "claude-sonnet-4-5" || len(sent.Messages) != 1 {
		t.Errorf("inner request = %+v", sent)
	}
}

func TestAnthropicMessages_BufferedResponse(t *testing.T) {
	chat := &fakeChat{body: `{
		"id":"chatcmpl-abc","model":"claude-sonnet-4-5",
		"choices":[{"message":{"content":"hello there"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}
	}`}
	w := postMessages(t, chat, `{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["type"] != "message" || got["role"] != "assistant" {
		t.Errorf("envelope = %v", got)
	}
	if got["id"] != "msg_abc" {
		t.Errorf("id = %v, want msg_abc", got["id"])
	}
	if got["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v", got["stop_reason"])
	}
	content, _ := got["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content = %v", got["content"])
	}
	block := content[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "hello there" {
		t.Errorf("block = %v", block)
	}
	usage := got["usage"].(map[string]any)
	if usage["input_tokens"].(float64) != 11 || usage["output_tokens"].(float64) != 3 {
		t.Errorf("usage = %v", usage)
	}
}

func TestAnthropicMessages_BufferedToolUse(t *testing.T) {
	chat := &fakeChat{body: `{
		"id":"chatcmpl-1","model":"m",
		"choices":[{"message":{"content":null,"tool_calls":[
			{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},
			"finish_reason":"tool_calls"}]
	}`}
	w := postMessages(t, chat, `{"model":"m","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["stop_reason"] != "tool_use" {
		t.Errorf("stop_reason = %v, want tool_use", got["stop_reason"])
	}
	block := got["content"].([]any)[0].(map[string]any)
	if block["type"] != "tool_use" || block["name"] != "get_weather" {
		t.Fatalf("block = %v", block)
	}
	input, ok := block["input"].(map[string]any)
	if !ok || input["city"] != "Paris" {
		t.Errorf("input = %#v, want the arguments parsed into an object", block["input"])
	}
}

func TestAnthropicMessages_MaxTokensStopReason(t *testing.T) {
	chat := &fakeChat{body: `{"id":"chatcmpl-1","model":"m","choices":[{"message":{"content":"x"},"finish_reason":"length"}]}`}
	w := postMessages(t, chat, `{"model":"m","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`)

	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["stop_reason"] != "max_tokens" {
		t.Errorf("stop_reason = %v, want max_tokens", got["stop_reason"])
	}
}

func TestAnthropicMessages_StreamEventSequence(t *testing.T) {
	chat := &fakeChat{sse: []string{
		"data: {\"id\":\"chatcmpl-s\",\"model\":\"claude-sonnet-4-5\",\"choices\":[{\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-s\",\"choices\":[{\"delta\":{\"content\":\"Hel\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-s\",\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-s\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
		"data: {\"id\":\"chatcmpl-s\",\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2,\"total_tokens\":9}}\n\n",
		"data: [DONE]\n\n",
	}}
	w := postMessages(t, chat, `{"model":"claude-sonnet-4-5","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if strings.Contains(w.Body.String(), "[DONE]") {
		t.Error("Anthropic streams must not carry a [DONE] sentinel")
	}

	events := parseSSE(t, w.Body.String())
	want := []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}
	got := eventNames(events)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("event sequence = %v, want %v", got, want)
	}

	// Every frame's `type` must agree with its event name — the SDKs dispatch
	// on both.
	for _, e := range events {
		if e.data["type"] != e.name {
			t.Errorf("event %q carries type %v", e.name, e.data["type"])
		}
	}

	start := events[0].data["message"].(map[string]any)
	if start["id"] != "msg_s" || start["model"] != "claude-sonnet-4-5" || start["role"] != "assistant" {
		t.Errorf("message_start = %v", start)
	}

	blockStart := events[1].data["content_block"].(map[string]any)
	if blockStart["type"] != "text" || blockStart["text"] != "" {
		t.Errorf("content_block_start = %v", blockStart)
	}
	if events[1].data["index"].(float64) != 0 {
		t.Errorf("first block index = %v, want 0", events[1].data["index"])
	}

	var text strings.Builder
	for _, e := range events[2:4] {
		delta := e.data["delta"].(map[string]any)
		if delta["type"] != "text_delta" {
			t.Fatalf("delta = %v", delta)
		}
		text.WriteString(delta["text"].(string))
	}
	if text.String() != "Hello" {
		t.Errorf("streamed text = %q, want Hello", text.String())
	}

	msgDelta := events[5].data
	if msgDelta["delta"].(map[string]any)["stop_reason"] != "end_turn" {
		t.Errorf("message_delta = %v", msgDelta)
	}
	if msgDelta["usage"].(map[string]any)["output_tokens"].(float64) != 2 {
		t.Errorf("message_delta usage = %v", msgDelta["usage"])
	}
}

func TestAnthropicMessages_StreamToolCall(t *testing.T) {
	chat := &fakeChat{sse: []string{
		"data: {\"id\":\"chatcmpl-t\",\"model\":\"m\",\"choices\":[{\"delta\":{\"content\":\"one moment\"},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"city\\\":\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"Paris\\\"}\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n",
		"data: [DONE]\n\n",
	}}
	w := postMessages(t, chat, `{"model":"m","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	events := parseSSE(t, w.Body.String())
	want := []string{
		"message_start",
		"content_block_start", // text
		"content_block_delta", // "one moment"
		"content_block_stop",  // text closed when the tool block opens
		"content_block_start", // tool_use
		"content_block_delta", // {"city":
		"content_block_delta", // "Paris"}
		"content_block_stop",  // tool closed at the end of the turn
		"message_delta",
		"message_stop",
	}
	got := eventNames(events)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("event sequence = %v, want %v", got, want)
	}

	tool := events[4].data["content_block"].(map[string]any)
	if tool["type"] != "tool_use" || tool["id"] != "call_1" || tool["name"] != "get_weather" {
		t.Errorf("tool block = %v", tool)
	}
	if events[4].data["index"].(float64) != 1 {
		t.Errorf("tool block index = %v, want 1", events[4].data["index"])
	}

	var args strings.Builder
	for _, e := range events[5:7] {
		delta := e.data["delta"].(map[string]any)
		if delta["type"] != "input_json_delta" {
			t.Fatalf("delta = %v", delta)
		}
		args.WriteString(delta["partial_json"].(string))
	}
	if args.String() != `{"city":"Paris"}` {
		t.Errorf("reassembled arguments = %q", args.String())
	}

	if events[8].data["delta"].(map[string]any)["stop_reason"] != "tool_use" {
		t.Errorf("stop_reason = %v, want tool_use", events[8].data["delta"])
	}
}

func TestAnthropicMessages_StreamWithNoChunksStillCloses(t *testing.T) {
	chat := &fakeChat{sse: []string{"data: [DONE]\n\n"}}
	w := postMessages(t, chat, `{"model":"m","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	got := eventNames(parseSSE(t, w.Body.String()))
	want := []string{"message_start", "message_delta", "message_stop"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("event sequence = %v, want %v", got, want)
	}
}

func TestAnthropicMessages_StreamRequestsUsage(t *testing.T) {
	chat := &fakeChat{sse: []string{"data: [DONE]\n\n"}}
	postMessages(t, chat, `{"model":"m","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	var sent oaiRequest
	if err := json.Unmarshal(chat.gotBody, &sent); err != nil {
		t.Fatalf("decode inner: %v", err)
	}
	if sent.StreamOptions == nil || !sent.StreamOptions.IncludeUsage {
		t.Error("streaming requests must ask upstream for a usage block")
	}
}

func TestAnthropicMessages_ErrorEnvelope(t *testing.T) {
	chat := &fakeChat{status: http.StatusTooManyRequests, body: `{"error":"rate limited"}`}
	w := postMessages(t, chat, `{"model":"m","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["type"] != "error" {
		t.Fatalf("envelope = %v", got)
	}
	e := got["error"].(map[string]any)
	if e["type"] != "rate_limit_error" || e["message"] != "rate limited" {
		t.Errorf("error = %v", e)
	}
}

func TestAnthropicMessages_UpstreamErrorObjectEnvelope(t *testing.T) {
	chat := &fakeChat{status: http.StatusBadRequest, body: `{"error":{"message":"bad model","type":"invalid_request_error"}}`}
	w := postMessages(t, chat, `{"model":"m","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)

	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	e := got["error"].(map[string]any)
	if e["message"] != "bad model" || e["type"] != "invalid_request_error" {
		t.Errorf("error = %v", e)
	}
}

func TestAnthropicMessages_RejectsInvalidBody(t *testing.T) {
	chat := &fakeChat{}
	w := postMessages(t, chat, `{"model":`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if chat.gotBody != nil {
		t.Error("a malformed request must not reach the chat path")
	}
}

func TestAnthropicCountTokens(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"12345678"}]}`))
	w := httptest.NewRecorder()
	handleAnthropicCountTokens(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["input_tokens"] != 2 {
		t.Errorf("input_tokens = %d, want 2", got["input_tokens"])
	}
}

func TestAnthropicMessageIDRoundTrip(t *testing.T) {
	if got := anthropicMessageID("chatcmpl-XYZ"); got != "msg_XYZ" {
		t.Errorf("anthropicMessageID = %q, want msg_XYZ", got)
	}
	if got := anthropicMessageID(""); !strings.HasPrefix(got, "msg_") {
		t.Errorf("anthropicMessageID(\"\") = %q", got)
	}
}

func TestToolInputFallsBackToEmptyObject(t *testing.T) {
	for _, args := range []string{"", "{\"a\":", "[1,2]", "null"} {
		got, ok := toolInput(args).(map[string]any)
		if !ok || len(got) != 0 {
			t.Errorf("toolInput(%q) = %#v, want an empty object", args, got)
		}
	}
}
