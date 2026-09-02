// Package crypto — AES-256-GCM encrypt/decrypt for sensitive secrets.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// normalizeKey ensures key is exactly 32 bytes.
// Shorter → zero-padded. Longer → SHA-256 hash.
func normalizeKey(key []byte) []byte {
	if len(key) == 32 {
		return key
	}
	if len(key) > 32 {
		h := sha256.Sum256(key)
		return h[:]
	}
	out := make([]byte, 32)
	copy(out, key)
	return out
}

// EncryptAES encrypts with AES-256-GCM.
// Returns base64(nonce || ciphertext).
func EncryptAES(plaintext []byte, key []byte) ([]byte, error) {
	k := normalizeKey(key)
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := gcm.Seal(nonce, nonce, plaintext, nil)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(out)))
	base64.StdEncoding.Encode(encoded, out)
	return encoded, nil
}

// DecryptAES decrypts base64(nonce || ciphertext) with AES-256-GCM.
func DecryptAES(ciphertextB64 string, key []byte) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, err
	}
	k := normalizeKey(key)
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// NewAESKey generates a random 32-byte AES-256 key.
func NewAESKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}
