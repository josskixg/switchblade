package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// ── Gemini wire shapes ─────────────────────────────────────────────────────

type geminiRequest struct {
	Contents          []geminiContent   `json:"contents"`
	SystemInstruction *geminiContent    `json:"systemInstruction"`
	GenerationConfig  *geminiGenConfig  `json:"generationConfig"`
	Tools             []geminiTool      `json:"tools"`
	ToolConfig        *geminiToolConfig `json:"toolConfig"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *geminiBlob             `json:"inlineData,omitempty"`
	FileData         *geminiFileData         `json:"fileData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response,omitempty"`
}

type geminiGenConfig struct {
	MaxOutputTokens  int             `json:"maxOutputTokens"`
	Temperature      *float64        `json:"temperature"`
	TopP             *float64        `json:"topP"`
	StopSequences    []string        `json:"stopSequences"`
	CandidateCount   int             `json:"candidateCount"`
	ResponseMimeType string          `json:"responseMimeType"`
	ResponseSchema   json.RawMessage `json:"responseSchema"`
	ThinkingConfig   *struct {
		ThinkingBudget int `json:"thinkingBudget"`
	} `json:"thinkingConfig"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type geminiToolConfig struct {
	FunctionCallingConfig *struct {
		Mode string `json:"mode"`
	} `json:"functionCallingConfig"`
}

// MountGeminiIngress registers Google's generateContent surface. chi does not
// split a path segment on ':', so the method suffix arrives inside the model
// parameter and is peeled off by the handler.
func MountGeminiIngress(r chi.Router, chat chatHandler) {
	r.Post("/v1beta/models/{model}", handleGeminiGenerate(chat))
}

// GeminiAuthShim normalises the credentials Google's SDKs send — an
// x-goog-api-key header or a ?key= query parameter — onto the x-api-key header
// the gateway authenticator already reads, so Gemini clients authenticate
// without the auth middleware growing a second extraction path.
func GeminiAuthShim(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" && r.Header.Get("Authorization") == "" {
			key := r.Header.Get("x-goog-api-key")
			if key == "" {
				key = r.URL.Query().Get("key")
			}
			if key != "" {
				r.Header.Set("x-api-key", key)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func handleGeminiGenerate(chat chatHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		model, method := splitGeminiMethod(chi.URLParam(req, "model"))
		var stream bool
		switch method {
		case "generateContent":
		case "streamGenerateContent":
			stream = true
		default:
			writeGeminiError(w, http.StatusNotFound, fmt.Sprintf("method %q is not supported", method))
			return
		}
		if model == "" {
			writeGeminiError(w, http.StatusBadRequest, "model is required")
			return
		}

		raw, err := io.ReadAll(io.LimitReader(req.Body, 8<<20))
		if err != nil {
			writeGeminiError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		var in geminiRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			writeGeminiError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		out, err := geminiToChat(model, &in, stream)
		if err != nil {
			writeGeminiError(w, http.StatusBadRequest, err.Error())
			return
		}
		body, err := json.Marshal(out)
		if err != nil {
			writeGeminiError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Without alt=sse the streaming surface is a progressively written JSON
		// array rather than an event stream; every official SDK asks for sse.
		d := &geminiDialect{model: model, sse: req.URL.Query().Get("alt") == "sse"}
		serveIngress(w, req, chat, body, d, stream)
	}
}

// splitGeminiMethod separates "gemini-2.5-pro:generateContent" into its parts.
// Clients also send the fully qualified "models/gemini-2.5-pro" form.
func splitGeminiMethod(param string) (model, method string) {
	if i := strings.LastIndex(param, ":"); i >= 0 {
		model, method = param[:i], param[i+1:]
	} else {
		model = param
	}
	return strings.TrimPrefix(model, "models/"), method
}

// ── Request translation: Gemini → OpenAI ───────────────────────────────────

func geminiToChat(model string, in *geminiRequest, stream bool) (*oaiRequest, error) {
	out := &oaiRequest{Model: model, Stream: stream}
	if stream {
		out.StreamOptions = &oaiStreamOpt{IncludeUsage: true}
	}

	if sys := in.SystemInstruction; sys != nil {
		if text := geminiPartsText(sys.Parts); text != "" {
			out.Messages = append(out.Messages, oaiMessage{Role: "system", Content: text})
		}
	}

	// Gemini function responses reference the call by name, not by id, so ids
	// are synthesised on the way out and looked up again on the way back.
	callIDs := make(map[string]string)
	var seq int

	for _, c := range in.Contents {
		role := "user"
		if c.Role == "model" {
			role = "assistant"
		}

		var parts []oaiPart
		var toolCalls []oaiToolCall
		var responses []oaiMessage
		var text strings.Builder
		multimodal := false

		for _, p := range c.Parts {
			switch {
			case p.FunctionCall != nil:
				seq++
				id := fmt.Sprintf("call_%s_%d", sanitizeToolName(p.FunctionCall.Name), seq)
				callIDs[p.FunctionCall.Name] = id
				args := string(p.FunctionCall.Args)
				if args == "" || args == "null" {
					args = "{}"
				}
				toolCalls = append(toolCalls, oaiToolCall{
					ID:       id,
					Type:     "function",
					Function: oaiToolCallFunc{Name: p.FunctionCall.Name, Arguments: args},
				})
			case p.FunctionResponse != nil:
				content := string(p.FunctionResponse.Response)
				if content == "" {
					content = "{}"
				}
				responses = append(responses, oaiMessage{
					Role:       "tool",
					ToolCallID: callIDs[p.FunctionResponse.Name],
					Content:    content,
				})
			case p.InlineData != nil && p.InlineData.Data != "":
				mt := p.InlineData.MimeType
				if mt == "" {
					mt = "image/png"
				}
				parts = append(parts, oaiPart{
					Type:     "image_url",
					ImageURL: &oaiImageURL{URL: "data:" + mt + ";base64," + p.InlineData.Data},
				})
				multimodal = true
			case p.FileData != nil && p.FileData.FileURI != "":
				parts = append(parts, oaiPart{
					Type:     "image_url",
					ImageURL: &oaiImageURL{URL: p.FileData.FileURI},
				})
				multimodal = true
			case p.Text != "":
				parts = append(parts, oaiPart{Type: "text", Text: p.Text})
				if text.Len() > 0 {
					text.WriteString("\n")
				}
				text.WriteString(p.Text)
			}
		}

		out.Messages = append(out.Messages, responses...)

		var content any
		if len(parts) > 0 {
			if multimodal {
				content = parts
			} else {
				content = text.String()
			}
		}
		if content == nil && len(toolCalls) == 0 {
			continue
		}
		out.Messages = append(out.Messages, oaiMessage{Role: role, Content: content, ToolCalls: toolCalls})
	}

	if len(out.Messages) == 0 {
		return nil, fmt.Errorf("contents: at least one content entry is required")
	}

	if g := in.GenerationConfig; g != nil {
		out.MaxTokens = g.MaxOutputTokens
		out.Temperature = g.Temperature
		out.TopP = g.TopP
		out.Stop = g.StopSequences
		if g.CandidateCount > 1 {
			out.N = g.CandidateCount
		}
		if g.ThinkingConfig != nil {
			out.ReasoningEffort = reasoningEffortFor(g.ThinkingConfig.ThinkingBudget)
		}
		if g.ResponseMimeType == "application/json" {
			if len(g.ResponseSchema) > 0 {
				out.ResponseFormat = map[string]any{
					"type": "json_schema",
					"json_schema": map[string]any{
						"name":   "response",
						"schema": g.ResponseSchema,
					},
				}
			} else {
				out.ResponseFormat = map[string]string{"type": "json_object"}
			}
		}
	}

	for _, t := range in.Tools {
		for _, fn := range t.FunctionDeclarations {
			if fn.Name == "" {
				continue
			}
			out.Tools = append(out.Tools, oaiTool{
				Type:     "function",
				Function: oaiFunction(fn),
			})
		}
	}

	if tc := in.ToolConfig; tc != nil && tc.FunctionCallingConfig != nil {
		switch tc.FunctionCallingConfig.Mode {
		case "AUTO":
			out.ToolChoice = "auto"
		case "ANY":
			out.ToolChoice = "required"
		case "NONE":
			out.ToolChoice = "none"
		}
	}

	return out, nil
}

func geminiPartsText(parts []geminiPart) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Text == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(p.Text)
	}
	return sb.String()
}

// sanitizeToolName keeps synthesised call ids readable and free of characters
// that upstreams reject in a tool_call_id.
func sanitizeToolName(name string) string {
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
	}
	if sb.Len() == 0 {
		return "fn"
	}
	return sb.String()
}

// ── Response translation: OpenAI → Gemini ──────────────────────────────────

// geminiDialect accumulates the terminal chunk rather than emitting it as soon
// as finish_reason arrives: with stream_options.include_usage the token counts
// come in a trailing chunk, and Gemini reports usageMetadata on the last frame.
type geminiDialect struct {
	model string
	sse   bool

	wrote      bool
	finish     string
	usage      oaiUsage
	toolOrder  []int
	toolBuffer map[int]*geminiPendingCall
}

type geminiPendingCall struct {
	name string
	args strings.Builder
}

func (d *geminiDialect) StreamContentType() string {
	if d.sse {
		return "text/event-stream"
	}
	return "application/json"
}

func (d *geminiDialect) Frame(data []byte) []byte {
	var chunk oaiChunk
	if json.Unmarshal(data, &chunk) != nil {
		return nil
	}
	if chunk.Model != "" {
		d.model = chunk.Model
	}
	if u := chunk.Usage; u != nil {
		if u.PromptTokens > 0 {
			d.usage.PromptTokens = u.PromptTokens
		}
		if u.CompletionTokens > 0 {
			d.usage.CompletionTokens = u.CompletionTokens
		}
		if u.TotalTokens > 0 {
			d.usage.TotalTokens = u.TotalTokens
		}
	}
	if len(chunk.Choices) == 0 {
		return nil
	}
	choice := chunk.Choices[0]

	for _, tc := range choice.Delta.ToolCalls {
		if d.toolBuffer == nil {
			d.toolBuffer = make(map[int]*geminiPendingCall)
		}
		pending, ok := d.toolBuffer[tc.Index]
		if !ok {
			pending = &geminiPendingCall{}
			d.toolBuffer[tc.Index] = pending
			d.toolOrder = append(d.toolOrder, tc.Index)
		}
		if tc.Function.Name != "" {
			pending.name = tc.Function.Name
		}
		pending.args.WriteString(tc.Function.Arguments)
	}

	if fr := choice.FinishReason; fr != nil && *fr != "" {
		d.finish = *fr
	}

	// Gemini has no partial-argument frame, so a turn that only produced tool
	// call fragments stays silent until Finish assembles them.
	if choice.Delta.Content == "" {
		return nil
	}
	return d.emit(map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{
				"role":  "model",
				"parts": []geminiPart{{Text: choice.Delta.Content}},
			},
			"index": 0,
		}},
		"modelVersion": d.model,
	})
}

func (d *geminiDialect) Finish() []byte {
	parts := d.pendingParts()
	if len(parts) == 0 {
		parts = []geminiPart{{Text: ""}}
	}
	payload := map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": parts},
			"finishReason": geminiFinishReason(d.finish),
			"index":        0,
		}},
		"usageMetadata": geminiUsage(d.usage),
		"modelVersion":  d.model,
	}
	out := d.emit(payload)
	if d.sse {
		return out
	}
	if !d.wrote {
		return []byte("[]")
	}
	return append(out, ']')
}

func (d *geminiDialect) pendingParts() []geminiPart {
	parts := make([]geminiPart, 0, len(d.toolOrder))
	for _, idx := range d.toolOrder {
		pending := d.toolBuffer[idx]
		args, err := json.Marshal(toolInput(pending.args.String()))
		if err != nil {
			continue
		}
		parts = append(parts, geminiPart{
			FunctionCall: &geminiFunctionCall{Name: pending.name, Args: args},
		})
	}
	return parts
}

func (d *geminiDialect) emit(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	if d.sse {
		// Gemini's SSE carries no event: lines and no terminating sentinel.
		out := make([]byte, 0, len(b)+8)
		out = append(out, "data: "...)
		out = append(out, b...)
		return append(out, '\n', '\n')
	}
	var out []byte
	if d.wrote {
		out = append(out, ',')
	} else {
		out = append(out, '[')
	}
	d.wrote = true
	return append(out, b...)
}

func (d *geminiDialect) Body(body []byte) []byte {
	var comp oaiCompletion
	if err := json.Unmarshal(body, &comp); err != nil {
		return d.Error(http.StatusBadGateway, body)
	}
	if comp.Model != "" {
		d.model = comp.Model
	}

	parts := make([]geminiPart, 0, 2)
	finish := ""
	if len(comp.Choices) > 0 {
		choice := comp.Choices[0]
		if c := choice.Message.Content; c != "" {
			parts = append(parts, geminiPart{Text: c})
		}
		for _, tc := range choice.Message.ToolCalls {
			args, err := json.Marshal(toolInput(tc.Function.Arguments))
			if err != nil {
				continue
			}
			parts = append(parts, geminiPart{
				FunctionCall: &geminiFunctionCall{Name: tc.Function.Name, Args: args},
			})
		}
		finish = choice.FinishReason
	}
	if len(parts) == 0 {
		parts = []geminiPart{{Text: ""}}
	}

	usage := oaiUsage{}
	if comp.Usage != nil {
		usage = *comp.Usage
	}

	b, err := json.Marshal(map[string]any{
		"candidates": []any{map[string]any{
			"content":       map[string]any{"role": "model", "parts": parts},
			"finishReason":  geminiFinishReason(finish),
			"index":         0,
			"safetyRatings": []any{},
		}},
		"usageMetadata": geminiUsage(usage),
		"modelVersion":  d.model,
	})
	if err != nil {
		return d.Error(http.StatusInternalServerError, nil)
	}
	return b
}

func (d *geminiDialect) Error(status int, body []byte) []byte {
	b, _ := json.Marshal(geminiErrorEnvelope(status, errorMessage(body)))
	return b
}

func geminiUsage(u oaiUsage) map[string]int {
	total := u.TotalTokens
	if total == 0 {
		total = u.PromptTokens + u.CompletionTokens
	}
	return map[string]int{
		"promptTokenCount":     u.PromptTokens,
		"candidatesTokenCount": u.CompletionTokens,
		"totalTokenCount":      total,
	}
}

// geminiFinishReason maps OpenAI's reason across. Gemini signals a tool call by
// the presence of a functionCall part, not by a distinct reason, so a tool turn
// still terminates with STOP.
func geminiFinishReason(finish string) string {
	switch finish {
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	default:
		return "STOP"
	}
}

func writeGeminiError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(geminiErrorEnvelope(status, msg))
}

func geminiErrorEnvelope(status int, msg string) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": msg,
			"status":  geminiStatusCode(status),
		},
	}
}

func geminiStatusCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusPaymentRequired, http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	case http.StatusServiceUnavailable:
		return "UNAVAILABLE"
	default:
		return "INTERNAL"
	}
}
