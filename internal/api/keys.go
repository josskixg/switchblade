package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"switchblade/internal/db"
)

// MountKeysAPI registers API key management endpoints.
func MountKeysAPI(r chi.Router, database *db.DB) {
	r.Get("/api/keys", listKeys(database))
	r.Post("/api/keys", RequireJSONHandler(createKey(database)))
	r.Delete("/api/keys/{id}", deleteKey(database))
}

func listKeys(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, name, key_hash, created_at, last_used_at, enabled FROM api_keys ORDER BY id`,
		)
		if err != nil {
			log.Printf("[api] list keys failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		type keyRow struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			KeyHash    string `json:"key_hash"`
			CreatedAt  int64  `json:"created_at"`
			LastUsedAt int64  `json:"last_used_at"`
			Enabled    bool   `json:"enabled"`
		}
		var out []keyRow
		for rows.Next() {
			var k keyRow
			var enabled int
			if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.CreatedAt, &k.LastUsedAt, &enabled); err != nil {
				log.Printf("[api] key scan failed: %v", err)
				jsonError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			k.Enabled = enabled == 1
			out = append(out, k)
		}
		jsonOK(w, out)
	}
}

func createKey(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			jsonError(w, http.StatusBadRequest, "name required")
			return
		}

		// generate 32 random bytes → hex key
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			jsonError(w, http.StatusInternalServerError, "rng failure")
			return
		}
		key := hex.EncodeToString(raw)

		sum := sha256.Sum256([]byte(key))
		hash := hex.EncodeToString(sum[:])

		now := time.Now().Unix()
		res, err := database.Exec(
			`INSERT INTO api_keys (name, key_hash, created_at, last_used_at, enabled) VALUES (?, ?, ?, 0, 1)`,
			body.Name, hash, now,
		)
		if err != nil {
			log.Printf("[api] create key failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		id, _ := res.LastInsertId()
		w.WriteHeader(http.StatusCreated)
		// return the plaintext key once — caller must store it
		jsonOK(w, map[string]any{"id": id, "key": key, "name": body.Name})
	}
}

func deleteKey(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := database.Exec(`DELETE FROM api_keys WHERE id = ?`, id); err != nil {
			log.Printf("[api] delete key failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}
