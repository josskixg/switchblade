package proxy

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"switchblade/internal/billing"
	"switchblade/internal/providers"
	"switchblade/internal/reqctx"
)

// The non-chat stubs are deliberately separate types: a provider that only
// implements Chat must fail the capability assertion, which a single stub with
// nil function fields could not express.

type embedStub struct {
	stubProvider
	embed func() (*providers.EmbeddingsResponse, error)
}

func (e *embedStub) Embeddings(context.Context, *providers.Account, *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	return e.embed()
}

type ttsStub struct {
	stubProvider
	speak func() (*providers.TTSResponse, error)
}

func (s *ttsStub) TTS(context.Context, *providers.Account, *providers.TTSRequest) (*providers.TTSResponse, error) {
	return s.speak()
}

type sttStub struct {
	stubProvider
	transcribe func() (*providers.STTResponse, error)
}

func (s *sttStub) STT(context.Context, *providers.Account, *providers.STTRequest) (*providers.STTResponse, error) {
	return s.transcribe()
}

// chatOnly owns every model but implements none of the optional service
// interfaces, so it exercises the "provider does not support X" path.
func chatOnly() *stubProvider {
	return &stubProvider{name: "chatonly", fn: func(*providers.ChatRequest) (*providers.ChatResponse, error) {
		return &providers.ChatResponse{StatusCode: 200}, nil
	}}
}

func jsonRequest(path, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	ctx := reqctx.WithTenant(r.Context(), "t1")
	ctx = context.WithValue(ctx, reqctx.KeyID, int64(7))
	return r.WithContext(ctx)
}

// sttRequest builds the multipart upload the transcription endpoint expects.
// withFile=false omits the file part so the missing-field path can be metered.
func sttRequest(model string, withFile bool) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if withFile {
		part, _ := mw.CreateFormFile("file", "clip.mp3")
		_, _ = part.Write([]byte("ID3fake-audio"))
	}
	_ = mw.WriteField("model", model)
	_ = mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	ctx := reqctx.WithTenant(r.Context(), "t1")
	ctx = context.WithValue(ctx, reqctx.KeyID, int64(7))
	return r.WithContext(ctx)
}

// TestServeNonChat_EveryExitPathIsMetered covers the three endpoints the billing
// gate admits alongside chat: an unrecorded request is free forever and invisible
// in the audit trail, so every exit path has to leave a row behind.
func TestServeNonChat_EveryExitPathIsMetered(t *testing.T) {
	tests := []struct {
		name       string
		provider   providers.Provider
		request    func() *http.Request
		serve      func(*Router, http.ResponseWriter, *http.Request)
		wantStatus int
		wantKind   string
		wantEvent  string
		wantModel  string
		wantUsage  billing.Usage
		wantErrMsg bool
	}{
		{
			name: "embeddings success",
			provider: &embedStub{
				stubProvider: stubProvider{name: "emb"},
				embed: func() (*providers.EmbeddingsResponse, error) {
					return &providers.EmbeddingsResponse{
						Object:     "list",
						Model:      "text-embed",
						StatusCode: 200,
						Usage:      providers.EmbeddingUsage{PromptTokens: 8, TotalTokens: 8},
					}, nil
				},
			},
			request:    func() *http.Request { return jsonRequest("/v1/embeddings", `{"model":"text-embed","input":"hi"}`) },
			serve:      (*Router).ServeEmbeddings,
			wantStatus: 200,
			wantKind:   "embeddings",
			wantEvent:  "success",
			wantModel:  "text-embed",
			wantUsage:  billing.Usage{PromptTokens: 8, TotalTokens: 8},
		},
		{
			name: "embeddings upstream failure keeps its status",
			provider: &embedStub{
				stubProvider: stubProvider{name: "emb"},
				embed: func() (*providers.EmbeddingsResponse, error) {
					return nil, &providers.ProviderError{Code: 429, Message: "rate limited", Provider: "emb"}
				},
			},
			request:    func() *http.Request { return jsonRequest("/v1/embeddings", `{"model":"text-embed"}`) },
			serve:      (*Router).ServeEmbeddings,
			wantStatus: 429,
			wantKind:   "embeddings",
			wantEvent:  "error",
			wantModel:  "text-embed",
			wantErrMsg: true,
		},
		{
			name:       "embeddings on a chat-only provider",
			provider:   chatOnly(),
			request:    func() *http.Request { return jsonRequest("/v1/embeddings", `{"model":"whatever"}`) },
			serve:      (*Router).ServeEmbeddings,
			wantStatus: http.StatusBadRequest,
			wantKind:   "embeddings",
			wantEvent:  "error",
			wantModel:  "whatever",
			wantErrMsg: true,
		},
		{
			name:       "embeddings malformed body",
			provider:   chatOnly(),
			request:    func() *http.Request { return jsonRequest("/v1/embeddings", `not json`) },
			serve:      (*Router).ServeEmbeddings,
			wantStatus: http.StatusBadRequest,
			wantKind:   "embeddings",
			wantEvent:  "error",
			wantErrMsg: true,
		},
		{
			name:       "embeddings with no provider",
			provider:   nil,
			request:    func() *http.Request { return jsonRequest("/v1/embeddings", `{"model":"nope"}`) },
			serve:      (*Router).ServeEmbeddings,
			wantStatus: http.StatusBadRequest,
			wantKind:   "embeddings",
			wantEvent:  "error",
			wantModel:  "nope",
			wantErrMsg: true,
		},
		{
			name: "tts success",
			provider: &ttsStub{
				stubProvider: stubProvider{name: "speech"},
				speak: func() (*providers.TTSResponse, error) {
					return &providers.TTSResponse{Audio: []byte("mp3"), ContentType: "audio/mpeg", StatusCode: 200}, nil
				},
			},
			request:    func() *http.Request { return jsonRequest("/v1/audio/speech", `{"model":"tts-1","input":"hello"}`) },
			serve:      (*Router).ServeTTS,
			wantStatus: 200,
			wantKind:   "tts",
			wantEvent:  "success",
			wantModel:  "tts-1",
		},
		{
			name: "tts upstream failure",
			provider: &ttsStub{
				stubProvider: stubProvider{name: "speech"},
				speak: func() (*providers.TTSResponse, error) {
					return nil, &providers.ProviderError{Code: 502, Message: "boom", Provider: "speech"}
				},
			},
			request:    func() *http.Request { return jsonRequest("/v1/audio/speech", `{"model":"tts-1"}`) },
			serve:      (*Router).ServeTTS,
			wantStatus: 502,
			wantKind:   "tts",
			wantEvent:  "error",
			wantModel:  "tts-1",
			wantErrMsg: true,
		},
		{
			name:       "tts on a chat-only provider",
			provider:   chatOnly(),
			request:    func() *http.Request { return jsonRequest("/v1/audio/speech", `{"model":"tts-1"}`) },
			serve:      (*Router).ServeTTS,
			wantStatus: http.StatusBadRequest,
			wantKind:   "tts",
			wantEvent:  "error",
			wantModel:  "tts-1",
			wantErrMsg: true,
		},
		{
			name: "stt success",
			provider: &sttStub{
				stubProvider: stubProvider{name: "whisper"},
				transcribe: func() (*providers.STTResponse, error) {
					return &providers.STTResponse{StatusCode: 200, Body: []byte(`{"text":"hello"}`)}, nil
				},
			},
			request:    func() *http.Request { return sttRequest("whisper-1", true) },
			serve:      (*Router).ServeSTT,
			wantStatus: 200,
			wantKind:   "stt",
			wantEvent:  "success",
			wantModel:  "whisper-1",
		},
		{
			name: "stt upstream failure",
			provider: &sttStub{
				stubProvider: stubProvider{name: "whisper"},
				transcribe: func() (*providers.STTResponse, error) {
					return nil, &providers.ProviderError{Code: 413, Message: "file too large", Provider: "whisper"}
				},
			},
			request:    func() *http.Request { return sttRequest("whisper-1", true) },
			serve:      (*Router).ServeSTT,
			wantStatus: 413,
			wantKind:   "stt",
			wantEvent:  "error",
			wantModel:  "whisper-1",
			wantErrMsg: true,
		},
		{
			name:       "stt without a file part",
			provider:   chatOnly(),
			request:    func() *http.Request { return sttRequest("whisper-1", false) },
			serve:      (*Router).ServeSTT,
			wantStatus: http.StatusBadRequest,
			wantKind:   "stt",
			wantEvent:  "error",
			wantErrMsg: true,
		},
		{
			name:       "stt with no provider",
			provider:   nil,
			request:    func() *http.Request { return sttRequest("nope", true) },
			serve:      (*Router).ServeSTT,
			wantStatus: http.StatusBadRequest,
			wantKind:   "stt",
			wantEvent:  "error",
			wantModel:  "nope",
			wantErrMsg: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meter := &capturingMeter{}
			w := httptest.NewRecorder()
			tt.serve(routerWith(tt.provider, meter), w, tt.request())

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if w.Header().Get("X-Request-Id") == "" {
				t.Error("X-Request-Id not set — the request cannot be correlated to its ledger entry")
			}

			ev := meter.only(t)
			if ev.ServiceKind != tt.wantKind {
				t.Errorf("ServiceKind = %q, want %q", ev.ServiceKind, tt.wantKind)
			}
			if ev.Status != tt.wantEvent {
				t.Errorf("Status = %q, want %q", ev.Status, tt.wantEvent)
			}
			if ev.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", ev.Model, tt.wantModel)
			}
			if ev.Usage != tt.wantUsage {
				t.Errorf("Usage = %+v, want %+v", ev.Usage, tt.wantUsage)
			}
			if got := ev.ErrMessage != ""; got != tt.wantErrMsg {
				t.Errorf("ErrMessage present = %v (%q), want %v", got, ev.ErrMessage, tt.wantErrMsg)
			}
			if ev.TenantID != "t1" || ev.APIKeyID != 7 {
				t.Errorf("tenant/key = %q/%d, want t1/7", ev.TenantID, ev.APIKeyID)
			}
			if ev.RequestID == "" || ev.RequestID != w.Header().Get("X-Request-Id") {
				t.Errorf("RequestID %q does not match the X-Request-Id header %q", ev.RequestID, w.Header().Get("X-Request-Id"))
			}
		})
	}
}

// TestServeNonChat_UnmeteredRouterStillServes proves the metering scaffolding is
// optional — a router built without a meter (tests, embedded use) must not
// depend on it.
func TestServeNonChat_UnmeteredRouterStillServes(t *testing.T) {
	p := &ttsStub{
		stubProvider: stubProvider{name: "speech"},
		speak: func() (*providers.TTSResponse, error) {
			return &providers.TTSResponse{Audio: []byte("mp3"), ContentType: "audio/mpeg", StatusCode: 200}, nil
		},
	}
	w := httptest.NewRecorder()
	routerWith(p, nil).ServeTTS(w, jsonRequest("/v1/audio/speech", `{"model":"tts-1"}`))

	if w.Code != 200 || w.Body.String() != "mp3" {
		t.Errorf("status/body = %d/%q, want 200/mp3", w.Code, w.Body.String())
	}
}
