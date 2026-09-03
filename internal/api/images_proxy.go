package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/db"
)

// newUUID returns a random 32-hex-char ID (no external dep).
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// MountImagesProxyAPI mounts OpenAI-compatible image generation endpoints on r.
// storageDir is where downloaded images are saved (created on first use).
func MountImagesProxyAPI(r chi.Router, database *db.DB, storageDir string) {
	r.Post("/v1/images/generations", RequireJSONHandler(proxyImageGeneration(database, storageDir)))
	r.Get("/v1/images/retrieve/{id}", retrieveProxiedImage(database))
}

func proxyImageGeneration(database *db.DB, storageDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt         string `json:"prompt"`
			N              int    `json:"n"`
			Size           string `json:"size"`
			Model          string `json:"model"`
			ResponseFormat string `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Prompt == "" {
			jsonError(w, http.StatusBadRequest, "prompt required")
			return
		}
		if req.N <= 0 {
			req.N = 1
		}
		if req.Size == "" {
			req.Size = "1024x1024"
		}

		// Pick first active image-capable account.
		row := database.QueryRow(`
			SELECT id, provider, tokens
			FROM accounts
			WHERE status = 'active' AND enabled = 1
			  AND provider IN ('canva','fireworks','stabilityai','fal')
			LIMIT 1`)
		var accID int64
		var provider, tokensJSON string
		if err := row.Scan(&accID, &provider, &tokensJSON); err != nil {
			jsonError(w, http.StatusServiceUnavailable, "no image provider available")
			return
		}

		// Extract bearer token from tokens JSON blob.
		var tokMap map[string]any
		bearerToken := ""
		if json.Unmarshal([]byte(tokensJSON), &tokMap) == nil {
			for _, k := range []string{"access_token", "token", "api_key"} {
				if v, ok := tokMap[k].(string); ok && v != "" {
					bearerToken = v
					break
				}
			}
		}

		// Build a minimal provider request body (OpenAI format — providers that
		// speak this schema will work; others need their own adapter, ponytail: add
		// per-provider adapters when a second provider needs non-OpenAI format).
		providerReqBody, _ := json.Marshal(map[string]any{
			"prompt": req.Prompt,
			"n":      req.N,
			"size":   req.Size,
			"model":  req.Model,
		})

		// Determine endpoint from provider.
		endpoint := providerImageEndpoint(provider)

		httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, nil)
		if err != nil {
			slog.Error("[api] build provider request failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		httpReq.Body = io.NopCloser(jsonReader(providerReqBody))
		httpReq.Header.Set("Content-Type", "application/json")
		if bearerToken != "" {
			httpReq.Header.Set("Authorization", "Bearer "+bearerToken)
		}

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			slog.Error("[api] provider request failed", "err", err)
			jsonError(w, http.StatusBadGateway, "image generation failed")
			return
		}
		defer resp.Body.Close()

		var provResp struct {
			Data []struct {
				URL     string `json:"url"`
				B64JSON string `json:"b64_json"`
			} `json:"data"`
		}
		respBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			jsonError(w, http.StatusBadGateway, fmt.Sprintf("provider %d: %s", resp.StatusCode, string(respBytes)))
			return
		}
		if err := json.Unmarshal(respBytes, &provResp); err != nil {
			slog.Error("[api] parse provider response failed", "err", err)
			jsonError(w, http.StatusBadGateway, "image generation failed")
			return
		}

		if err := os.MkdirAll(storageDir, 0o755); err != nil {
			slog.Error("[api] create storage dir failed", "err", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		type outItem struct {
			URL string `json:"url"`
		}
		var outItems []outItem

		for _, item := range provResp.Data {
			imgURL := item.URL
			if imgURL == "" {
				continue
			}

			// Download and save to disk.
			imgID := newUUID()
			localPath := filepath.Join(storageDir, imgID+".png")
			if err := downloadToFile(imgURL, localPath); err != nil {
				// ponytail: skip items that fail to download; caller still gets partial results
				continue
			}

			fi, _ := os.Stat(localPath)
			var fileSize int64
			if fi != nil {
				fileSize = fi.Size()
			}

			now := time.Now().Unix()
			var insertedID int64
			res, err := database.Exec(`
				INSERT INTO image_studio_results
					(prompt, type, aspect_ratio, n, urls, local_path, file_size, file_type, created_at)
				VALUES (?, 'image', ?, ?, '[]', ?, ?, 'image/png', ?)`,
				req.Prompt, req.Size, req.N, localPath, fileSize, now)
			if err == nil {
				insertedID, _ = res.LastInsertId()
			}

			outItems = append(outItems, outItem{
				URL: fmt.Sprintf("/v1/images/retrieve/%d", insertedID),
			})
		}

		if len(outItems) == 0 {
			jsonError(w, http.StatusBadGateway, "no images returned by provider")
			return
		}

		jsonOK(w, map[string]any{
			"created": time.Now().Unix(),
			"data":    outItems,
		})
	}
}

func retrieveProxiedImage(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		row := database.QueryRow(`SELECT local_path, file_type FROM image_studio_results WHERE id = ?`, id)
		var localPath *string
		var fileType *string
		if err := row.Scan(&localPath, &fileType); err != nil || localPath == nil || *localPath == "" {
			jsonError(w, http.StatusNotFound, "image not found")
			return
		}
		data, err := os.ReadFile(*localPath)
		if err != nil {
			jsonError(w, http.StatusNotFound, "file not readable")
			return
		}
		mime := "image/png"
		if fileType != nil && *fileType != "" {
			mime = *fileType
		}
		w.Header().Set("Content-Type", mime)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			slog.Warn("[images] write response", "err", err)
		}
	}
}

func providerImageEndpoint(provider string) string {
	switch provider {
	case "fireworks":
		return "https://api.fireworks.ai/inference/v1/images/generations"
	case "stabilityai":
		return "https://api.stability.ai/v1/generation/stable-diffusion-xl-1024-v1-0/text-to-image"
	case "fal":
		return "https://fal.run/fal-ai/flux/schnell"
	default: // canva and fallback
		return "https://api.canva.com/v1/images/generations"
	}
}

// downloadToFile fetches url and writes bytes to dst.
func downloadToFile(url, dst string) error {
	if err := IsSafeURL(url); err != nil {
		return fmt.Errorf("ssrf blocked: %w", err)
	}
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// jsonReader wraps a byte slice as an io.Reader (avoids bytes import).
type byteReader struct {
	b   []byte
	pos int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.pos:])
	r.pos += n
	return n, nil
}

func jsonReader(b []byte) io.Reader { return &byteReader{b: b} }
