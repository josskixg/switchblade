package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// chatHandler is the metered OpenAI-shaped chat path that every native ingress
// dialect is funnelled through. Depending on the behaviour rather than
// *proxy.Router keeps the translators testable without a provider registry
// standing behind them.
type chatHandler interface {
	ServeChat(w http.ResponseWriter, r *http.Request)
}

// chatCompletionsPath is the internal route the router recognises. Native
// ingress replays its translated body against it so auth, billing, filters,
// caching and fallback all observe exactly what an OpenAI client produces.
const chatCompletionsPath = "/v1/chat/completions"

// dialect renders the router's OpenAI-shaped output back into the wire format
// the calling SDK expects. Instances are per-request: streaming translation has
// to remember which content block is currently open.
type dialect interface {
	// StreamContentType is the media type of the translated stream.
	StreamContentType() string
	// Frame translates one OpenAI SSE `data:` payload. An empty result drops
	// the frame.
	Frame(data []byte) []byte
	// Finish emits whatever the dialect needs to close a stream.
	Finish() []byte
	// Body translates a complete, non-streamed chat completion.
	Body(body []byte) []byte
	// Error renders a gateway or upstream failure in the dialect's envelope.
	Error(status int, body []byte) []byte
}

// serveIngress runs one native-dialect request through the metered chat path.
func serveIngress(w http.ResponseWriter, req *http.Request, next chatHandler, body []byte, d dialect, stream bool) {
	inner := req.Clone(req.Context())
	inner.Method = http.MethodPost
	inner.URL = &url.URL{Path: chatCompletionsPath}
	inner.RequestURI = ""
	inner.Body = io.NopCloser(bytes.NewReader(body))
	inner.ContentLength = int64(len(body))
	inner.Header.Set("Content-Type", "application/json")

	iw := &ingressWriter{w: w, d: d, stream: stream}
	next.ServeChat(iw, inner)
	iw.finish()
}

// ingressWriter sits between the router and the client, translating the
// response as it goes past. Streaming is handled frame by frame so the caller
// still sees tokens as they are generated; everything else is buffered because
// the dialects reshape a completion as a whole rather than field by field.
type ingressWriter struct {
	w      http.ResponseWriter
	d      dialect
	stream bool

	status      int
	wroteHeader bool
	streaming   bool
	buf         bytes.Buffer
	pending     bytes.Buffer
}

func (iw *ingressWriter) Header() http.Header { return iw.w.Header() }

func (iw *ingressWriter) WriteHeader(code int) {
	if iw.wroteHeader {
		return
	}
	iw.wroteHeader = true
	iw.status = code

	// A streaming request still answers with a buffered JSON error when the
	// pipeline rejects it before a provider was reached, so the stream shape is
	// confirmed from what the router actually produced, not from the intent.
	ct := iw.w.Header().Get("Content-Type")
	iw.streaming = iw.stream && code < 400 && strings.HasPrefix(ct, "text/event-stream")
	if !iw.streaming {
		// Header is withheld until finish() knows the translated length.
		return
	}
	h := iw.w.Header()
	h.Set("Content-Type", iw.d.StreamContentType())
	h.Del("Content-Length")
	iw.w.WriteHeader(code)
}

func (iw *ingressWriter) Write(p []byte) (int, error) {
	if !iw.wroteHeader {
		iw.WriteHeader(http.StatusOK)
	}
	if !iw.streaming {
		return iw.buf.Write(p)
	}

	iw.pending.Write(p)
	for {
		line, err := iw.pending.ReadBytes('\n')
		if err != nil {
			// Partial line — hold it until the rest of the frame arrives.
			iw.pending.Reset()
			iw.pending.Write(line)
			break
		}
		iw.consume(line)
	}
	return len(p), nil
}

// Flush keeps the router's flusher assertion satisfied so translated frames
// leave the process as soon as they are produced.
func (iw *ingressWriter) Flush() {
	if f, ok := iw.w.(http.Flusher); ok {
		f.Flush()
	}
}

var sseDataPrefix = []byte("data:")

func (iw *ingressWriter) consume(line []byte) {
	trimmed := bytes.TrimSpace(line)
	if !bytes.HasPrefix(trimmed, sseDataPrefix) {
		return
	}
	payload := bytes.TrimSpace(trimmed[len(sseDataPrefix):])
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return
	}
	if out := iw.d.Frame(payload); len(out) > 0 {
		_, _ = iw.w.Write(out)
		iw.Flush()
	}
}

func (iw *ingressWriter) finish() {
	if iw.streaming {
		if tail := iw.d.Finish(); len(tail) > 0 {
			_, _ = iw.w.Write(tail)
		}
		iw.Flush()
		return
	}

	status := iw.status
	if status == 0 {
		status = http.StatusOK
	}
	var out []byte
	if status >= 400 {
		out = iw.d.Error(status, iw.buf.Bytes())
	} else {
		out = iw.d.Body(iw.buf.Bytes())
	}

	h := iw.w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Content-Length", strconv.Itoa(len(out)))
	iw.w.WriteHeader(status)
	_, _ = iw.w.Write(out)
}

// ── OpenAI canonical shapes ────────────────────────────────────────────────

type oaiRequest struct {
	Model             string        `json:"model"`
	Messages          []oaiMessage  `json:"messages"`
	MaxTokens         int           `json:"max_tokens,omitempty"`
	Temperature       *float64      `json:"temperature,omitempty"`
	TopP              *float64      `json:"top_p,omitempty"`
	Stop              []string      `json:"stop,omitempty"`
	Stream            bool          `json:"stream,omitempty"`
	StreamOptions     *oaiStreamOpt `json:"stream_options,omitempty"`
	Tools             []oaiTool     `json:"tools,omitempty"`
	ToolChoice        any           `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool         `json:"parallel_tool_calls,omitempty"`
	ResponseFormat    any           `json:"response_format,omitempty"`
	ReasoningEffort   string        `json:"reasoning_effort,omitempty"`
	User              string        `json:"user,omitempty"`
	N                 int           `json:"n,omitempty"`
}

type oaiStreamOpt struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}

type oaiImageURL struct {
	URL string `json:"url"`
}

type oaiToolCall struct {
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function oaiToolCallFunc `json:"function"`
}

type oaiToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type oaiTool struct {
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// oaiCompletion is the buffered response shape. content is decoded as a raw
// message because providers legitimately send null for a tool-only turn.
type oaiCompletion struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content          string        `json:"content"`
			ReasoningContent string        `json:"reasoning_content"`
			ToolCalls        []oaiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *oaiUsage `json:"usage"`
}

type oaiChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int             `json:"index"`
				ID       string          `json:"id"`
				Function oaiToolCallFunc `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *oaiUsage `json:"usage"`
}

// ── Anthropic wire shapes ──────────────────────────────────────────────────

type anthropicRequest struct {
	Model         string               `json:"model"`
	MaxTokens     int                  `json:"max_tokens"`
	Messages      []anthropicMessage   `json:"messages"`
	System        json.RawMessage      `json:"system"`
	Temperature   *float64             `json:"temperature"`
	TopP          *float64             `json:"top_p"`
	StopSequences []string             `json:"stop_sequences"`
	Stream        bool                 `json:"stream"`
	Tools         []anthropicTool      `json:"tools"`
	ToolChoice    *anthropicToolChoice `json:"tool_choice"`
	Thinking      *anthropicThinking   `json:"thinking"`
	Metadata      *struct {
		UserID string `json:"user_id"`
	} `json:"metadata"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicBlock struct {
	Type      string            `json:"type"`
	Text      string            `json:"text"`
	Source    *anthropicSource  `json:"source"`
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Input     json.RawMessage   `json:"input"`
	ToolUseID string            `json:"tool_use_id"`
	Content   json.RawMessage   `json:"content"`
	IsError   bool              `json:"is_error"`
	Thinking  string            `json:"thinking"`
	Signature string            `json:"signature"`
	CacheCtl  map[string]string `json:"cache_control"`
}

type anthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	URL       string `json:"url"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type                   string `json:"type"`
	Name                   string `json:"name"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

// MountAnthropicIngress registers the Anthropic Messages API on the proxy
// router. Claude Code, Cline and the anthropic-sdk-* family point
// ANTHROPIC_BASE_URL here and speak nothing else.
func MountAnthropicIngress(r chi.Router, chat chatHandler) {
	r.Post("/v1/messages", handleAnthropicMessages(chat))
	r.Post("/v1/messages/count_tokens", handleAnthropicCountTokens)
}

func handleAnthropicMessages(chat chatHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(req.Body, 8<<20))
		if err != nil {
			writeAnthropicError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		var in anthropicRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			writeAnthropicError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		out, err := anthropicToChat(&in)
		if err != nil {
			writeAnthropicError(w, http.StatusBadRequest, err.Error())
			return
		}
		body, err := json.Marshal(out)
		if err != nil {
			writeAnthropicError(w, http.StatusInternalServerError, err.Error())
			return
		}

		serveIngress(w, req, chat, body, &anthropicDialect{model: in.Model}, in.Stream)
	}
}

// handleAnthropicCountTokens answers locally. Claude Code only uses the count
// to budget its context window, and a round trip to a pooled account would
// spend a real request on an estimate the client treats as advisory.
func handleAnthropicCountTokens(w http.ResponseWriter, req *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(req.Body, 8<<20))
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var in anthropicRequest
	if err := json.Unmarshal(raw, &in); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var sb strings.Builder
	sb.WriteString(anthropicSystemText(in.System))
	for _, m := range in.Messages {
		blocks, err := decodeAnthropicContent(m.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				sb.WriteString(b.Text)
			case "tool_use":
				sb.WriteString(b.Name)
				sb.Write(b.Input)
			case "tool_result":
				sb.WriteString(flattenAnthropicText(b.Content))
			}
		}
	}
	jsonOK(w, map[string]int{"input_tokens": (sb.Len() + 3) / 4})
}

// ── Request translation: Anthropic → OpenAI ────────────────────────────────

func anthropicToChat(in *anthropicRequest) (*oaiRequest, error) {
	if in.Model == "" {
		return nil, fmt.Errorf("model: field required")
	}
	if in.MaxTokens <= 0 {
		return nil, fmt.Errorf("max_tokens: field required")
	}

	out := &oaiRequest{
		Model:       in.Model,
		MaxTokens:   in.MaxTokens,
		Temperature: in.Temperature,
		TopP:        in.TopP,
		Stop:        in.StopSequences,
		Stream:      in.Stream,
	}
	if in.Stream {
		// message_delta has to carry output_tokens, and the meter needs the same
		// numbers, so usage is requested explicitly rather than hoped for.
		out.StreamOptions = &oaiStreamOpt{IncludeUsage: true}
	}
	if in.Metadata != nil {
		out.User = in.Metadata.UserID
	}
	if in.Thinking != nil && in.Thinking.Type == "enabled" {
		out.ReasoningEffort = reasoningEffortFor(in.Thinking.BudgetTokens)
	}

	if sys := anthropicSystemText(in.System); sys != "" {
		out.Messages = append(out.Messages, oaiMessage{Role: "system", Content: sys})
	}

	for _, m := range in.Messages {
		blocks, err := decodeAnthropicContent(m.Content)
		if err != nil {
			return nil, err
		}

		// Anthropic carries tool results inside the following user turn; OpenAI
		// wants them as their own messages ahead of it, so they are peeled off
		// first and the remaining blocks form the user message.
		var rest []anthropicBlock
		for _, b := range blocks {
			if b.Type != "tool_result" {
				rest = append(rest, b)
				continue
			}
			out.Messages = append(out.Messages, oaiMessage{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
				Content:    flattenAnthropicText(b.Content),
			})
		}

		if m.Role == "assistant" {
			msg := oaiMessage{Role: "assistant"}
			for _, b := range rest {
				if b.Type != "tool_use" {
					continue
				}
				args := string(b.Input)
				if args == "" || args == "null" {
					args = "{}"
				}
				msg.ToolCalls = append(msg.ToolCalls, oaiToolCall{
					ID:       b.ID,
					Type:     "function",
					Function: oaiToolCallFunc{Name: b.Name, Arguments: args},
				})
			}
			msg.Content = anthropicBlocksToContent(rest)
			if msg.Content == nil && len(msg.ToolCalls) == 0 {
				continue
			}
			out.Messages = append(out.Messages, msg)
			continue
		}

		if content := anthropicBlocksToContent(rest); content != nil {
			out.Messages = append(out.Messages, oaiMessage{Role: "user", Content: content})
		}
	}

	if len(out.Messages) == 0 {
		return nil, fmt.Errorf("messages: at least one message is required")
	}

	for _, t := range in.Tools {
		// Server-side tools (web_search and friends) carry a type instead of a
		// schema; there is nothing to forward to an OpenAI-shaped upstream.
		if t.Name == "" || len(t.InputSchema) == 0 {
			continue
		}
		out.Tools = append(out.Tools, oaiTool{
			Type: "function",
			Function: oaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	if tc := in.ToolChoice; tc != nil {
		switch tc.Type {
		case "auto":
			out.ToolChoice = "auto"
		case "any":
			out.ToolChoice = "required"
		case "none":
			out.ToolChoice = "none"
		case "tool":
			out.ToolChoice = map[string]any{
				"type":     "function",
				"function": map[string]string{"name": tc.Name},
			}
		}
		if tc.DisableParallelToolUse {
			no := false
			out.ParallelToolCalls = &no
		}
	}

	return out, nil
}

// decodeAnthropicContent normalises the string-or-array content field. A bare
// string is Anthropic's shorthand for a single text block.
func decodeAnthropicContent(raw json.RawMessage) ([]anthropicBlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []anthropicBlock{{Type: "text", Text: s}}, nil
	}
	var blocks []anthropicBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("messages: content must be a string or a block array")
	}
	return blocks, nil
}

// anthropicBlocksToContent returns a plain string for text-only turns, since
// most of the pool accepts that shape far more reliably than the part array.
func anthropicBlocksToContent(blocks []anthropicBlock) any {
	var parts []oaiPart
	var sb strings.Builder
	multimodal := false

	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, oaiPart{Type: "text", Text: b.Text})
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(b.Text)
		case "image":
			if u := anthropicImageURL(b.Source); u != "" {
				parts = append(parts, oaiPart{Type: "image_url", ImageURL: &oaiImageURL{URL: u}})
				multimodal = true
			}
		}
	}

	if len(parts) == 0 {
		return nil
	}
	if !multimodal {
		return sb.String()
	}
	return parts
}

func anthropicImageURL(src *anthropicSource) string {
	if src == nil {
		return ""
	}
	switch src.Type {
	case "base64":
		if src.Data == "" {
			return ""
		}
		mt := src.MediaType
		if mt == "" {
			mt = "image/png"
		}
		return "data:" + mt + ";base64," + src.Data
	case "url":
		return src.URL
	}
	return ""
}

// anthropicSystemText flattens the string-or-block-array system parameter.
func anthropicSystemText(raw json.RawMessage) string {
	return flattenAnthropicText(raw)
}

func flattenAnthropicText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []anthropicBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Text == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(b.Text)
		}
		return sb.String()
	}
	return string(raw)
}

func reasoningEffortFor(budget int) string {
	switch {
	case budget <= 0:
		return ""
	case budget <= 2048:
		return "low"
	case budget <= 8192:
		return "medium"
	default:
		return "high"
	}
}

// ── Response translation: OpenAI → Anthropic ───────────────────────────────

// anthropicDialect holds the streaming state machine. OpenAI has no notion of
// content-block boundaries, so they are inferred: a block opens the first time
// its kind of delta appears and closes when another kind starts or the turn
// ends.
type anthropicDialect struct {
	model string
	id    string

	started    bool
	open       string // "", "text", "thinking", "tool"
	openIndex  int
	nextIndex  int
	toolBlocks map[int]int

	stopReason string
	in, out    int
}

func (d *anthropicDialect) StreamContentType() string { return "text/event-stream" }

func (d *anthropicDialect) Frame(data []byte) []byte {
	var chunk oaiChunk
	if json.Unmarshal(data, &chunk) != nil {
		return nil
	}

	out := d.start(&chunk)
	if u := chunk.Usage; u != nil {
		if u.PromptTokens > 0 {
			d.in = u.PromptTokens
		}
		if u.CompletionTokens > 0 {
			d.out = u.CompletionTokens
		}
	}
	if len(chunk.Choices) == 0 {
		return out
	}
	choice := chunk.Choices[0]

	if r := choice.Delta.ReasoningContent; r != "" {
		out = append(out, d.openBlock("thinking", map[string]any{"type": "thinking", "thinking": ""})...)
		out = append(out, sseFrame("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": d.openIndex,
			"delta": map[string]any{"type": "thinking_delta", "thinking": r},
		})...)
	}

	if c := choice.Delta.Content; c != "" {
		out = append(out, d.openBlock("text", map[string]any{"type": "text", "text": ""})...)
		out = append(out, sseFrame("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": d.openIndex,
			"delta": map[string]any{"type": "text_delta", "text": c},
		})...)
	}

	for _, tc := range choice.Delta.ToolCalls {
		idx, known := d.toolBlocks[tc.Index]
		if !known {
			out = append(out, d.closeBlock()...)
			idx = d.nextIndex
			d.nextIndex++
			if d.toolBlocks == nil {
				d.toolBlocks = make(map[int]int)
			}
			d.toolBlocks[tc.Index] = idx
			d.open, d.openIndex = "tool", idx

			id := tc.ID
			if id == "" {
				id = randomID("toolu_")
			}
			out = append(out, sseFrame("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]any{
					"type": "tool_use", "id": id, "name": tc.Function.Name, "input": map[string]any{},
				},
			})...)
		}
		// Argument fragments are forwarded verbatim; re-marshalling a partial
		// JSON string would corrupt it.
		if args := tc.Function.Arguments; args != "" {
			out = append(out, sseFrame("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": args},
			})...)
		}
	}

	if fr := choice.FinishReason; fr != nil && *fr != "" {
		d.stopReason = anthropicStopReason(*fr)
	}
	return out
}

func (d *anthropicDialect) Finish() []byte {
	out := d.start(nil)
	out = append(out, d.closeBlock()...)
	if d.stopReason == "" {
		d.stopReason = "end_turn"
	}
	out = append(out, sseFrame("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": d.stopReason, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": d.out},
	})...)
	// Anthropic has no [DONE] sentinel; message_stop is the terminator and an
	// extra frame after it is a client-visible protocol error.
	return append(out, sseFrame("message_stop", map[string]any{"type": "message_stop"})...)
}

func (d *anthropicDialect) start(chunk *oaiChunk) []byte {
	if d.started {
		return nil
	}
	d.started = true
	if chunk != nil {
		if chunk.Model != "" {
			d.model = chunk.Model
		}
		if chunk.ID != "" {
			d.id = anthropicMessageID(chunk.ID)
		}
		if chunk.Usage != nil && chunk.Usage.PromptTokens > 0 {
			d.in = chunk.Usage.PromptTokens
		}
	}
	if d.id == "" {
		d.id = randomID("msg_")
	}
	return sseFrame("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            d.id,
			"type":          "message",
			"role":          "assistant",
			"model":         d.model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": d.in, "output_tokens": 0},
		},
	})
}

func (d *anthropicDialect) openBlock(kind string, block map[string]any) []byte {
	if d.open == kind {
		return nil
	}
	out := d.closeBlock()
	idx := d.nextIndex
	d.nextIndex++
	d.open, d.openIndex = kind, idx
	return append(out, sseFrame("content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         idx,
		"content_block": block,
	})...)
}

func (d *anthropicDialect) closeBlock() []byte {
	if d.open == "" {
		return nil
	}
	d.open = ""
	return sseFrame("content_block_stop", map[string]any{
		"type": "content_block_stop", "index": d.openIndex,
	})
}

func (d *anthropicDialect) Body(body []byte) []byte {
	var comp oaiCompletion
	if err := json.Unmarshal(body, &comp); err != nil {
		return d.Error(http.StatusBadGateway, body)
	}

	model := comp.Model
	if model == "" {
		model = d.model
	}
	content := make([]map[string]any, 0, 2)
	stop := "end_turn"

	if len(comp.Choices) > 0 {
		choice := comp.Choices[0]
		if r := choice.Message.ReasoningContent; r != "" {
			content = append(content, map[string]any{"type": "thinking", "thinking": r})
		}
		if c := choice.Message.Content; c != "" {
			content = append(content, map[string]any{"type": "text", "text": c})
		}
		for _, tc := range choice.Message.ToolCalls {
			id := tc.ID
			if id == "" {
				id = randomID("toolu_")
			}
			content = append(content, map[string]any{
				"type": "tool_use", "id": id, "name": tc.Function.Name,
				"input": toolInput(tc.Function.Arguments),
			})
		}
		if choice.FinishReason != "" {
			stop = anthropicStopReason(choice.FinishReason)
		}
	}
	// The content array is never empty on the wire, even for a refusal.
	if len(content) == 0 {
		content = append(content, map[string]any{"type": "text", "text": ""})
	}

	var in, out int
	if comp.Usage != nil {
		in, out = comp.Usage.PromptTokens, comp.Usage.CompletionTokens
	}

	b, err := json.Marshal(map[string]any{
		"id":            anthropicMessageID(comp.ID),
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       content,
		"stop_reason":   stop,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens": in, "output_tokens": out,
			"cache_creation_input_tokens": 0, "cache_read_input_tokens": 0,
		},
	})
	if err != nil {
		return d.Error(http.StatusInternalServerError, nil)
	}
	return b
}

func (d *anthropicDialect) Error(status int, body []byte) []byte {
	b, _ := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    anthropicErrorType(status),
			"message": errorMessage(body),
		},
	})
	return b
}

func writeAnthropicError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": anthropicErrorType(status), "message": msg},
	})
}

func anthropicErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusPaymentRequired, http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusServiceUnavailable:
		return "overloaded_error"
	default:
		return "api_error"
	}
}

func anthropicStopReason(finish string) string {
	switch finish {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	default:
		return "end_turn"
	}
}

// anthropicMessageID keeps the correlation between the two ids so a chatcmpl id
// in the gateway log can be matched against what the client reported.
func anthropicMessageID(id string) string {
	if suffix := strings.TrimPrefix(id, "chatcmpl-"); suffix != "" && suffix != id {
		return "msg_" + suffix
	}
	if id != "" {
		return "msg_" + id
	}
	return randomID("msg_")
}

// ── shared helpers ─────────────────────────────────────────────────────────

func sseFrame(event string, payload any) []byte {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	out := make([]byte, 0, len(b)+len(event)+16)
	out = append(out, "event: "...)
	out = append(out, event...)
	out = append(out, '\n')
	out = append(out, "data: "...)
	out = append(out, b...)
	return append(out, '\n', '\n')
}

// toolInput parses OpenAI's stringified arguments into the object shape every
// native dialect expects. A truncated or malformed fragment degrades to an
// empty object rather than failing the whole response.
func toolInput(args string) any {
	if args == "" {
		return map[string]any{}
	}
	var v any
	if json.Unmarshal([]byte(args), &v) != nil {
		return map[string]any{}
	}
	if _, ok := v.(map[string]any); !ok {
		return map[string]any{}
	}
	return v
}

// errorMessage pulls a human-readable message out of either the gateway's own
// {"error":"..."} envelope or an upstream {"error":{"message":"..."}} one.
func errorMessage(body []byte) string {
	var probe struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &probe) == nil && len(probe.Error) > 0 {
		var s string
		if json.Unmarshal(probe.Error, &s) == nil && s != "" {
			return s
		}
		var obj struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(probe.Error, &obj) == nil && obj.Message != "" {
			return obj.Message
		}
	}
	if len(body) == 0 {
		return "upstream request failed"
	}
	const max = 512
	if len(body) > max {
		return string(body[:max])
	}
	return string(body)
}

func randomID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + hex.EncodeToString(b[:])
}
