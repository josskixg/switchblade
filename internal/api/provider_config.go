package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
)

// MountProviderConfigAPI mounts provider config CRUD routes on r.
func MountProviderConfigAPI(r chi.Router, database *db.DB) {
	r.Get("/api/provider-config", listProviderConfigs(database))
	r.Post("/api/provider-config", RequireJSONHandler(upsertProviderConfig(database)))
	r.Delete("/api/provider-config/{provider}", deleteProviderConfig(database))
	r.Get("/api/provider-config/defaults", getProviderDefaults())
	r.Post("/api/provider-config/seed", RequireJSONHandler(seedProviderDefaults(database)))
}

func listProviderConfigs(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT provider, base_url, models, enabled, extra, updated_at FROM provider_config ORDER BY provider`)
		if err != nil {
			slog.Error("[api] list provider configs failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var configs []map[string]any
		for rows.Next() {
			var provider, baseURL, models, extra string
			var enabled int
			var updatedAt int64
			if err := rows.Scan(&provider, &baseURL, &models, &enabled, &extra, &updatedAt); err != nil {
				continue
			}
			configs = append(configs, map[string]any{
				"provider":   provider,
				"base_url":   baseURL,
				"models":     json.RawMessage(models),
				"enabled":    enabled == 1,
				"extra":      json.RawMessage(extra),
				"updated_at": updatedAt,
			})
		}
		jsonOK(w, configs)
	}
}

func upsertProviderConfig(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Provider string          `json:"provider"`
			BaseURL  string          `json:"base_url"`
			Models   json.RawMessage `json:"models"`
			Enabled  *bool           `json:"enabled"`
			Extra    json.RawMessage `json:"extra"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Provider == "" || body.BaseURL == "" {
			jsonError(w, http.StatusBadRequest, "provider and base_url required")
			return
		}
		if err := ValidateBaseURL(body.BaseURL); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		models := string(body.Models)
		if models == "" || models == "null" {
			models = "[]"
		}
		extra := string(body.Extra)
		if extra == "" || extra == "null" {
			extra = "{}"
		}
		enabled := 1
		if body.Enabled != nil && !*body.Enabled {
			enabled = 0
		}
		now := time.Now().Unix()
		_, err := database.Exec(
			`INSERT OR REPLACE INTO provider_config (provider, base_url, models, enabled, extra, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			body.Provider, body.BaseURL, models, enabled, extra, now)
		if err != nil {
			slog.Error("[api] upsert provider config failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "ok", "provider": body.Provider})
	}
}

func deleteProviderConfig(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := chi.URLParam(r, "provider")
		if _, err := database.Exec(`DELETE FROM provider_config WHERE provider = ?`, provider); err != nil {
			slog.Error("[api] delete provider config failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted", "provider": provider})
	}
}

func getProviderDefaults() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, providerDefaultURLs)
	}
}

func seedProviderDefaults(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().Unix()
		inserted := 0
		for provider, baseURL := range providerDefaultURLs {
			res, err := database.Exec(
				`INSERT OR IGNORE INTO provider_config (provider, base_url, models, enabled, extra, updated_at)
				 VALUES (?, ?, '[]', 1, '{}', ?)`,
				provider, baseURL, now)
			if err != nil {
				continue
			}
			if n, _ := res.RowsAffected(); n > 0 {
				inserted++
			}
		}
		jsonOK(w, map[string]any{"seeded": inserted, "total": len(providerDefaultURLs)})
	}
}

// providerDefaultURLs maps every known provider name to its default base URL.
var providerDefaultURLs = map[string]string{
	// OpenAI-compatible
	"openai":        "https://api.openai.com/v1",
	"azure-openai":  "https://{resource}.openai.azure.com/openai/deployments/{deployment}",
	"anthropic":     "https://api.anthropic.com/v1",
	"cohere":        "https://api.cohere.ai/v1",
	"mistral":       "https://api.mistral.ai/v1",
	"groq":          "https://api.groq.com/openai/v1",
	"together":      "https://api.together.xyz/v1",
	"fireworks":     "https://api.fireworks.ai/inference/v1",
	"deepseek":      "https://api.deepseek.com/v1",
	"perplexity":    "https://api.perplexity.ai",
	"cerebras":      "https://api.cerebras.ai/v1",
	"sambanova":     "https://api.sambanova.ai/v1",
	"novita":        "https://api.novita.ai/v3/openai",
	"lepton":        "https://llama3-1-405b.lepton.run/api/v1",
	"hyperbolic":    "https://api.hyperbolic.xyz/v1",
	"xai":           "https://api.x.ai/v1",
	"google":        "https://generativelanguage.googleapis.com/v1beta",
	"google-vertex": "https://us-central1-aiplatform.googleapis.com/v1",
	"bedrock":       "https://bedrock-runtime.us-east-1.amazonaws.com",
	"cloudflare":    "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1",
	"replicate":     "https://api.replicate.com/v1",
	"huggingface":   "https://api-inference.huggingface.co/models",
	"ollama":        "http://localhost:11434/api",
	"lmstudio":      "http://localhost:1234/v1",
	"localai":       "http://localhost:8080/v1",
	"vllm":          "http://localhost:8000/v1",
	"textgenwebui":  "http://localhost:5000/v1",
	"koboldcpp":     "http://localhost:5001/api",
	"llamacpp":      "http://localhost:8080",
	"jan":           "http://localhost:1337/v1",
	"oobabooga":     "http://localhost:5000/v1",
	// Image providers
	"stabilityai": "https://api.stability.ai/v1",
	"fal":         "https://fal.run",
	"canva":       "https://api.canva.com/v1",
	"ideogram":    "https://api.ideogram.ai/generate",
	"midjourney":  "https://api.userapi.ai/midjourney/v2",
	"leonardo":    "https://cloud.leonardo.ai/api/rest/v1",
	"clipdrop":    "https://clipdrop-api.co",
	"getimg":      "https://api.getimg.ai/v1",
	"segmind":     "https://api.segmind.com/v1",
	// Code / dev tools
	"kiro":           "https://api.kiro.dev/v1",
	"kiro-pro":       "https://api.kiro.dev/v1",
	"codebuddy":      "https://api.codebuddy.ai/v1",
	"codex":          "https://api.openai.com/v1",
	"qoder":          "https://api.qoder.ai/v1",
	"mimo":           "https://api.mimo.ai/v1",
	"github-copilot": "https://api.githubcopilot.com",
	"tabnine":        "https://api.tabnine.com/v1",
	"codeium":        "https://web-backend.codeium.com/exa.language_server_pb.LanguageServerService",
	"sourcegraph":    "https://sourcegraph.com/.api",
	"cursor":         "https://api2.cursor.sh",
	"continue":       "http://localhost:65130",
	// Embedded/edge
	"aws-bedrock": "https://bedrock-runtime.us-east-1.amazonaws.com",
	"sagemaker":   "https://runtime.sagemaker.us-east-1.amazonaws.com",
	"azure-ml":    "https://{workspace}.{region}.inference.ml.azure.com",
	// Audio/speech
	"elevenlabs": "https://api.elevenlabs.io/v1",
	"openai-tts": "https://api.openai.com/v1/audio/speech",
	"playht":     "https://api.play.ht/api/v2",
	"assemblyai": "https://api.assemblyai.com/v2",
	"deepgram":   "https://api.deepgram.com/v1",
	"whisper":    "https://api.openai.com/v1/audio/transcriptions",
	// Video
	"runway": "https://api.runwayml.com/v1",
	"sora":   "https://api.openai.com/v1/video",
	"pika":   "https://api.pika.art/v1",
	"kling":  "https://api.klingai.com/v1",
	"luma":   "https://api.lumalabs.ai/dream-machine/v1a",
	// Embeddings / vector
	"pinecone": "https://api.pinecone.io",
	"weaviate": "http://localhost:8080/v1",
	"qdrant":   "http://localhost:6333",
	"milvus":   "http://localhost:19530",
	"chroma":   "http://localhost:8000/api/v1",
	// Misc
	"byok":         "https://api.openai.com/v1",
	"openrouter":   "https://openrouter.ai/api/v1",
	"anyscale":     "https://api.endpoints.anyscale.com/v1",
	"octoai":       "https://text.octoai.run/v1",
	"scale":        "https://api.scale.com/v1",
	"aleph-alpha":  "https://api.aleph-alpha.com",
	"cohere-coral": "https://api.cohere.ai/v1/chat",
	"nlpcloud":     "https://api.nlpcloud.io/v1",
	"ai21":         "https://api.ai21.com/studio/v1",
	"gooseai":      "https://api.goose.ai/v1",
	"forefront":    "https://api.forefront.ai/v1",
	"banana":       "https://api.banana.dev/start/v4",
	"steamship":    "https://api.steamship.com/api/v1",
	"lighton":      "https://api.lighton.ai/muse/v1",
	"textsynth":    "https://textsynth.com/v1",
	"deepai":       "https://api.deepai.org/api",
	"writesonic":   "https://api.writesonic.com/v2",
	"rytr":         "https://api.rytr.me/v1",
	"wordtune":     "https://api.wordtune.com/v1",
	"jasper":       "https://api.jasper.ai/v1",
	"copy-ai":      "https://api.copy.ai/api/v1",
	"sudowrite":    "https://sudowrite.com/api",
	"aiseo":        "https://api.aiseo.ai/v1",
	"longshot":     "https://api.longshot.ai/custom/api/instruct",
	"anyword":      "https://api.anyword.com/api/v1",
	// Extended GPU clouds & alternate endpoints
	"neuralspace":     "https://api.neuralspace.ai/v1",
	"runpod":          "https://api.runpod.ai/v1",
	"lambda":          "https://api.lambdalabs.com/v1",
	"vastai":          "https://a.vast.ai",
	"tensordock":      "https://client.tensordock.com/api/v1",
	"paperspace":      "https://api.paperspace.io/v1",
	"modal-labs":      "https://api.modal.com/v1",
	"hyperbolic-api":  "https://api.hyperbolic.xyz/v1",
	"cerebras-cloud":  "https://api.cerebras.ai/v1",
	"groq-cloud":      "https://api.groq.com/openai/v1",
	"perplexity-api":  "https://api.perplexity.ai",
	"openrouter-ai":   "https://openrouter.ai/api/v1",
	"fireworks-ai":    "https://api.fireworks.ai/inference/v1",
	"together-ai":     "https://api.together.xyz/v1",
	"anthropic-api":   "https://api.anthropic.com/v1",
	"mistral-ai":      "https://api.mistral.ai/v1",
	"cohere-api":      "https://api.cohere.ai/v1",
	"deepseek-ai":     "https://api.deepseek.com/v1",
	"xai-api":         "https://api.x.ai/v1",
	"sambanova-ai":    "https://api.sambanova.ai/v1",
	"novita-ai":       "https://api.novita.ai/v3/openai",
	"lepton-ai":       "https://api.lepton.ai/v1",
	"stability-ai":    "https://api.stability.ai/v1",
	"eleven-labs":     "https://api.elevenlabs.io/v1",
	"playht-api":      "https://api.play.ht/api/v2",
	"assembly-ai":     "https://api.assemblyai.com/v2",
	"deepgram-api":    "https://api.deepgram.com/v1",
	"pinecone-db":     "https://api.pinecone.io",
	"weaviate-db":     "http://localhost:8080/v1",
	"qdrant-db":       "http://localhost:6333",
	"milvus-db":       "http://localhost:19530",
	"chroma-db":       "http://localhost:8000/api/v1",
	"octo-ai":         "https://text.octoai.run/v1",
	"anyscale-api":    "https://api.endpoints.anyscale.com/v1",
	"aleph-alpha-api": "https://api.aleph-alpha.com",
	"ai21-api":        "https://api.ai21.com/studio/v1",
	"goose-ai":        "https://api.goose.ai/v1",
	"writesonic-api":  "https://api.writesonic.com/v2",
	"jasper-ai":       "https://api.jasper.ai/v1",
	"copy-ai-api":     "https://api.copy.ai/api/v1",
	"sudowrite-api":   "https://sudowrite.com/api",
	"aiseo-api":       "https://api.aiseo.ai/v1",
	"anyword-api":     "https://api.anyword.com/api/v1",
}
