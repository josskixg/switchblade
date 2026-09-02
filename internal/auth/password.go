// Package auth provides password hashing and JWT utilities for Switchblade.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the password (cost 10).
// New passwords are always bcrypt.
func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// VerifyPassword checks a password against a stored hash.
// Supports bcrypt ($2a$/$2b$) and legacy SHA-256 (salt:hash) formats.
func VerifyPassword(password, stored string) bool {
	// Bcrypt format (new)
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
	}
	// Legacy SHA-256 fallback (salt:hash)
	parts := splitHex(stored)
	if len(parts) != 2 {
		return false
	}
	salt, want := parts[0], parts[1]
	hash := sha256.Sum256(append([]byte(salt), []byte(password)...))
	computed := hex.EncodeToString(hash[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(want)) == 1
}

// GenerateToken returns n random bytes as hex.
func GenerateToken(length int) string {
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return hex.EncodeToString(raw)
}

// splitHex decodes a hex string and returns the raw bytes.
func splitHex(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return nil
}
