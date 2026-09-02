// Package proxy orchestrates the full request pipeline.
package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"switchblade/internal/billing"
	"switchblade/internal/providers"
	"switchblade/internal/proxy/compression"
)

// UsageMeter records billable requests. The router depends on the behaviour
// rather than *billing.Meter so tests can substitute a recorder.
type UsageMeter interface {
	Record(billing.Event)
}

// Router ties a Registry + per-provider AccountPool together.
type Router struct {
	registry    *providers.Registry
	pools       map[string]*AccountPool // provider name → pool (legacy shared-only wiring)
	poolSrc     PoolSource              // tenant-scoped pools; takes precedence over pools
	hub         interface{ Broadcast(event, data string) }
	filters     *Filters          // PUDIDIL content filter (nil = disabled)
	cache       *ResponseCache    // response cache (nil = disabled)
	modelMapper *ModelMapper      // model alias resolver (nil = disabled)
	fallback    *FallbackExecutor // fallback executor (nil = single-attempt)
	combos      *ComboStore       // fallback chain resolver (nil = no chains)
	meter       UsageMeter        // usage metering (nil = unmetered)
}

// RouterOptions holds optional pipeline components. Zero-value = disabled.
type RouterOptions struct {
	Filters     *Filters
	Cache       *ResponseCache
	ModelMapper *ModelMapper
	Fallback    *FallbackExecutor
	Combos      *ComboStore
	Meter       UsageMeter
	// Pools resolves the account pool per (tenant, provider) at request time.
	// When set it wins over the static pools map passed to NewRouter, which
	// cannot keep tenants apart.
	Pools PoolSource
}

// NewRouter creates a Router. pools is optional — providers without a pool
// (e.g. BYOK) will pick the account from the request context instead.
func NewRouter(registry *providers.Registry, pools map[string]*AccountPool, hub interface{ Broadcast(event, data string) }, opts RouterOptions) *Router {
	if pools == nil {
		pools = make(map[string]*AccountPool)
	}
	return &Router{
		registry:    registry,
		pools:       pools,
		poolSrc:     opts.Pools,
		hub:         hub,
		filters:     opts.Filters,
		cache:       opts.Cache,
		modelMapper: opts.ModelMapper,
		fallback:    opts.Fallback,
		combos:      opts.Combos,
		meter:       opts.Meter,
	}
}

// poolFor resolves the pool a request may lease from. A nil return means the
// provider is unpooled (BYOK) and takes credentials from the request instead.
func (r *Router) poolFor(tenant, provider string) *AccountPool {
	if r.poolSrc != nil {
		return r.poolSrc.For(tenant, provider)
	}
	return r.pools[provider]
}

// ChatRequest is the parsed inbound body (superset of providers.ChatRequest).
type inboundRequest struct {
	Model string `json:"model"`
}

// ServeChat handles POST /v1/chat/completions.
func (r *Router) ServeChat(w http.ResponseWriter, req *http.Request) {
	c := r.begin(w, req, "chat")
	defer c.done()

	body, err := io.ReadAll(io.LimitReader(req.Body, 8<<20)) // 8MB max
	if err != nil {
		c.fail(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var inbound inboundRequest
	if err := json.Unmarshal(body, &inbound); err != nil {
		c.fail(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	model := inbound.Model
	c.ev.Model = model

	// Stage 1: Model alias resolution.
	if r.modelMapper != nil {
		model = r.modelMapper.Resolve(model)
		c.ev.Model = model
	}

	// Resolve providerName for provider-specific jailbreak prompt mapping
	providerName := ""
	targetModel := model
	if r.combos != nil {
		if ch := r.combos.FindChain(model); len(ch) > 0 {
			targetModel = ch[0]
		}
	}
	if p := r.registry.Route(targetModel); p != nil {
		providerName = p.Name()
		c.ev.Provider = providerName
	}

	// Stage 2: PUDIDIL content filter — strip identity markers + custom rules.
	if r.filters != nil {
		body = r.filters.ApplyToMessages(body, providerName)
	}

	// Stage 3: Token compression — 6-stage pipeline (non-fatal).
	body, compStats := compression.Compress(body)
	if compStats.Total > 0 {
		log.Printf("[chat] compression saved %d bytes for model %q", compStats.Total, model)
	}

	// Parse stream flag.
	stream := false
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) == nil {
		if s, ok := raw["stream"]; ok {
			_ = json.Unmarshal(s, &stream)
		}
	}

	// OpenAI-dialect backends omit the usage block on streams unless
	// stream_options.include_usage is set, and a stream scraped at zero usage
	// bills nothing — so the proxy asks for the frame when the client did not.
	injectedUsage := false
	if stream && openAIStreamDialect(providerName) {
		body, injectedUsage = ensureUsageReporting(body, raw)
	}

	// Stage 4: Response cache check (skip streaming).
	cacheKey := cacheKeyFor(c.ev.TenantID, providerName, model, body)
	if !stream && r.cache != nil {
		if entry, ok := r.cache.Get(cacheKey); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Switchblade-Cache", "HIT")
			w.WriteHeader(entry.Status)
			_, _ = io.Copy(w, bytes.NewReader(entry.Body))
			// A cache hit costs nothing upstream, so it is logged with its token
			// counts but flagged so the meter does not charge for it. That does
			// mean replays within the TTL are free — an accepted trade, because
			// the key is tenant-scoped: a tenant can only replay a completion it
			// already paid for, and only until the entry expires.
			c.ev.Status = "success"
			c.ev.Cached = true
			c.ev.Usage = extractUsage(entry.Body)
			return
		}
	}

	// Build provider request.
	chatReq := &providers.ChatRequest{
		Model:  model,
		Body:   body,
		Stream: stream,
	}

	// Stage 5: Execute — fallback chain if configured, else single-attempt.
	var resp *providers.ChatResponse

	if r.fallback != nil {
		// Build fallback chain: combo if exists, else just [model].
		chain := []string{model}
		if r.combos != nil {
			if ch := r.combos.FindChain(model); len(ch) > 0 {
				chain = ch
			}
		}
		var out *FallbackOutcome
		out, err = r.fallback.Execute(c.ctx, chatReq, chain)
		if out != nil {
			resp = out.Response
			providerName = out.Provider
			c.ev.Provider = out.Provider
			c.ev.FallbackChain = out.Attempted
			if out.Account != nil {
				c.ev.AccountID, c.ev.AccountEmail = out.Account.ID, out.Account.Email
			}
		}
	} else {
		// Legacy single-attempt path.
		provider := c.route(w, model)
		if provider == nil {
			return
		}
		providerName = provider.Name()
		if !c.lease(w, provider) {
			return
		}
		resp, err = provider.Chat(c.ctx, c.acc, chatReq)
	}

	if err != nil {
		closeBody(resp)
		c.failUpstream(w, err, http.StatusBadGateway)
		return
	}

	// Response headers shared by both transports.
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if providerName != "" {
		w.Header().Set("X-Switchblade-Provider", providerName)
	}

	// Stage 6a: streaming passthrough. Bytes reach the client as the provider
	// emits them instead of landing in one batch after the whole generation
	// completes; usage is scraped from the frames as they go past.
	if resp.BodyStream != nil {
		defer resp.BodyStream.Close()
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "text/event-stream")
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no") // stop nginx re-buffering the stream
		w.WriteHeader(resp.StatusCode)

		c.streamStarted()
		usage, written, streamErr := pipeStream(w, resp.BodyStream, streamOpts{
			dropUsageFrame: injectedUsage,
			idle:           providerTimeout(),
			kill:           c.cancel,
		})
		if usage.TotalTokens == 0 && written > 0 {
			usage = estimateUsage(len(body), written)
		}
		c.ev.Usage = usage
		if streamErr != nil {
			// Not fed back to the pool: a client hanging up mid-stream is
			// indistinguishable here from an upstream fault, and evicting a healthy
			// account every time someone presses Ctrl-C drains the pool.
			c.ev.ErrMessage = streamErr.Error()
		} else {
			c.ev.Status = "success"
		}
		r.broadcast(model, providerName, resp.StatusCode, usage)
		return
	}

	// Stage 6b: buffered response.
	if !stream && r.cache != nil && resp.StatusCode < 400 {
		r.cache.Set(cacheKey, resp.Body, resp.StatusCode)
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, bytes.NewReader(resp.Body))

	if resp.StatusCode >= 400 {
		c.ev.ErrMessage = truncateBody(resp.Body)
	}
	c.succeed(resp.StatusCode, extractUsage(resp.Body))
}

// openAIStreamDialect reports whether provider accepts OpenAI's
// stream_options request field. Anthropic, Canva and Kiro speak their own
// request shapes and reject unknown top-level fields; every other registered
// backend is reached through an OpenAI-compatible /chat/completions endpoint.
func openAIStreamDialect(provider string) bool {
	switch provider {
	case "", "anthropic", "canva", "kiro":
		return false
	}
	return true
}

// ensureUsageReporting sets stream_options.include_usage on an OpenAI-dialect
// streaming body and reports whether the proxy added it on its own behalf —
// a frame the client did not opt into is stripped again on the way out.
func ensureUsageReporting(body []byte, raw map[string]json.RawMessage) ([]byte, bool) {
	if raw == nil {
		return body, false
	}
	if opts, ok := raw["stream_options"]; ok {
		var so struct {
			IncludeUsage bool `json:"include_usage"`
		}
		if json.Unmarshal(opts, &so) == nil && so.IncludeUsage {
			return body, false
		}
	}
	raw["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	out, err := json.Marshal(raw)
	if err != nil {
		return body, false
	}
	return out, true
}

// broadcast pushes a completed request to dashboard SSE listeners.
func (r *Router) broadcast(model, provider string, status int, u billing.Usage) {
	if r.hub == nil || status >= 400 {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"model":             model,
		"provider":          provider,
		"status":            status,
		"prompt_tokens":     u.PromptTokens,
		"completion_tokens": u.CompletionTokens,
		"total_tokens":      u.TotalTokens,
	})
	r.hub.Broadcast("request_complete", string(payload))
}

// truncateBody caps an upstream error body before it goes into the audit log.
func truncateBody(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max])
	}
	return string(b)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ServeEmbeddings handles POST /v1/embeddings.
func (r *Router) ServeEmbeddings(w http.ResponseWriter, req *http.Request) {
	c := r.begin(w, req, "embeddings")
	defer c.done()

	body, err := io.ReadAll(io.LimitReader(req.Body, 8<<20))
	if err != nil {
		c.fail(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var inbound providers.EmbeddingsRequest
	if err := json.Unmarshal(body, &inbound); err != nil {
		c.fail(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	c.ev.Model = inbound.Model

	provider := c.route(w, inbound.Model)
	if provider == nil {
		return
	}
	embedder, ok := provider.(providers.Embedder)
	if !ok {
		c.fail(w, http.StatusBadRequest, fmt.Sprintf("provider %q does not support embeddings", provider.Name()))
		return
	}
	if !c.lease(w, provider) {
		return
	}

	resp, err := embedder.Embeddings(c.ctx, c.acc, &inbound)
	if err != nil {
		c.failUpstream(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_ = json.NewEncoder(w).Encode(resp)

	c.succeed(resp.StatusCode, billing.Usage{
		PromptTokens: resp.Usage.PromptTokens,
		TotalTokens:  resp.Usage.TotalTokens,
	})
}

// ServeTTS handles POST /v1/audio/speech.
func (r *Router) ServeTTS(w http.ResponseWriter, req *http.Request) {
	c := r.begin(w, req, "tts")
	defer c.done()

	body, err := io.ReadAll(io.LimitReader(req.Body, 8<<20))
	if err != nil {
		c.fail(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var inbound providers.TTSRequest
	if err := json.Unmarshal(body, &inbound); err != nil {
		c.fail(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	c.ev.Model = inbound.Model

	provider := c.route(w, inbound.Model)
	if provider == nil {
		return
	}
	ttsProvider, ok := provider.(providers.TTSProvider)
	if !ok {
		c.fail(w, http.StatusBadRequest, fmt.Sprintf("provider %q does not support TTS", provider.Name()))
		return
	}
	if !c.lease(w, provider) {
		return
	}

	resp, err := ttsProvider.TTS(c.ctx, c.acc, &inbound)
	if err != nil {
		c.failUpstream(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", resp.ContentType)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Audio)

	// Speech is billed per character of input rather than per token, which the
	// rate card cannot express yet, so the request is audited at zero usage.
	c.succeed(resp.StatusCode, billing.Usage{})
}

// ServeSTT handles POST /v1/audio/transcriptions.
func (r *Router) ServeSTT(w http.ResponseWriter, req *http.Request) {
	c := r.begin(w, req, "stt")
	defer c.done()

	// Parse multipart form (max 25MB)
	if err := req.ParseMultipartForm(25 << 20); err != nil {
		c.fail(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	// Extract file
	file, header, err := req.FormFile("file")
	if err != nil {
		c.fail(w, http.StatusBadRequest, "missing or invalid 'file' field")
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		c.fail(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	// Build STT request
	sttReq := &providers.STTRequest{
		File:           fileBytes,
		Filename:       header.Filename,
		Model:          req.FormValue("model"),
		Language:       req.FormValue("language"),
		Prompt:         req.FormValue("prompt"),
		ResponseFormat: req.FormValue("response_format"),
	}
	c.ev.Model = sttReq.Model

	provider := c.route(w, sttReq.Model)
	if provider == nil {
		return
	}
	sttProvider, ok := provider.(providers.STTProvider)
	if !ok {
		c.fail(w, http.StatusBadRequest, fmt.Sprintf("provider %q does not support STT", provider.Name()))
		return
	}
	if !c.lease(w, provider) {
		return
	}

	resp, err := sttProvider.STT(c.ctx, c.acc, sttReq)
	if err != nil {
		c.failUpstream(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)

	// Transcription is billed per second of audio, a unit the rate card cannot
	// express yet; scraping the body still picks up a usage block from the
	// providers that volunteer one.
	c.succeed(resp.StatusCode, extractUsage(resp.Body))
}
