package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"switchblade/internal/providers"
)

// knownModels maps provider name → model IDs it serves.
// ponytail: static map, no DB round-trip needed for a model list.
var knownModels = map[string][]string{
	"kiro":         {"kiro-chat", "kiro-fast"},
	"kiro-pro":     {"kiro-pro-chat", "kiro-pro-fast"},
	"codebuddy":    {"codebuddy-chat"},
	"canva":        {"canva-assistant"},
	"codex":        {"codex-chat", "codex-mini"},
	"qoder":        {"qoder-chat"},
	"byok":         {"byok-proxy"},
	"mimo":         {"mimo-chat"},
	"anthropic":    {"claude-3-5-sonnet-20241022", "claude-3-5-haiku-20241022", "claude-3-opus-20240229"},
	"openai":       {"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-3.5-turbo"},
	"googleai":     {"gemini-1.5-pro", "gemini-1.5-flash", "gemini-2.0-flash"},
	"deepseek":     {"deepseek-chat", "deepseek-coder", "deepseek-reasoner"},
	"groq":         {"llama-3.3-70b-versatile", "llama-3.1-8b-instant", "mixtral-8x7b-32768"},
	"mistral":      {"mistral-large-latest", "mistral-small-latest", "codestral-latest"},
	"cohere":       {"command-r-plus", "command-r", "command"},
	"together":     {"meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo", "mistralai/Mixtral-8x7B-Instruct-v0.1"},
	"openrouter":   {"openrouter/auto"},
	"fireworks":    {"accounts/fireworks/models/llama-v3p1-70b-instruct"},
	"cerebras":     {"llama3.1-70b", "llama3.1-8b"},
	"hyperbolic":   {"meta-llama/Meta-Llama-3.1-70B-Instruct"},
	"sambanova":    {"Meta-Llama-3.1-70B-Instruct"},
	"novita":       {"meta-llama/llama-3.1-70b-instruct"},
	"perplexity":   {"llama-3.1-sonar-large-128k-online"},
	"huggingface":  {"meta-llama/Meta-Llama-3-8B-Instruct"},
	"cloudflareai": {"@cf/meta/llama-3.1-8b-instruct"},
	"githubmodels": {"gpt-4o", "meta-llama-3.1-70b-instruct"},
	"ai21":         {"jamba-1.5-large", "jamba-1.5-mini"},
	"alibaba":      {"qwen-turbo", "qwen-plus", "qwen-max"},
}

// MountModelsAPI registers GET /v1/models returning an OpenAI-compatible list.
func MountModelsAPI(r chi.Router, registry *providers.Registry) {
	r.Get("/v1/models", listModels(registry))
}

// MountDashboardModelsAPI exposes the same contract on the dashboard router.
func MountDashboardModelsAPI(r chi.Router, registry *providers.Registry) {
	r.Get("/api/models", listModels(registry))
}

func listModels(registry *providers.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type modelObj struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		}
		data := make([]modelObj, 0)
		for _, p := range registry.All() {
			models := knownModels[p.Name()]
			if registry.ConfigStore != nil {
				if configured, ok := registry.ConfigStore.Models(p.Name()); ok {
					if !registry.ConfigStore.Get(p.Name()).Enabled {
						continue
					}
					models = configured
				}
			}
			for _, m := range models {
				data = append(data, modelObj{ID: m, Object: "model", OwnedBy: p.Name()})
			}
		}
		jsonOK(w, map[string]any{"object": "list", "data": data})
	}
}
