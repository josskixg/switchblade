package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

var testSecret = []byte("test-secret-key-0123456789abcdef")

func TestJWTRoundtrip(t *testing.T) {
	claims := JWTClaims{
		UserID:    "u-123",
		TenantID:  "tenant-a",
		Role:      "admin",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}

	token, err := SignJWT(claims, testSecret)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	got, err := VerifyJWT(token, testSecret)
	if err != nil {
		t.Fatalf("VerifyJWT: %v", err)
	}
	if got.UserID != claims.UserID || got.TenantID != claims.TenantID ||
		got.Role != claims.Role || got.ExpiresAt != claims.ExpiresAt {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, claims)
	}
}

func TestJWTExpired(t *testing.T) {
	claims := JWTClaims{UserID: "u-1", ExpiresAt: time.Now().Add(-time.Minute).Unix()}
	token, err := SignJWT(claims, testSecret)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}
	if _, err := VerifyJWT(token, testSecret); !errors.Is(err, ErrExpiredToken) {
		t.Errorf("VerifyJWT expired token: err = %v, want ErrExpiredToken", err)
	}
}

func TestJWTWrongSecret(t *testing.T) {
	claims := JWTClaims{UserID: "u-1", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	token, _ := SignJWT(claims, testSecret)
	if _, err := VerifyJWT(token, []byte("different-secret")); !errors.Is(err, ErrSignatureFail) {
		t.Errorf("VerifyJWT wrong secret: err = %v, want ErrSignatureFail", err)
	}
}

func TestJWTTamperedPayload(t *testing.T) {
	claims := JWTClaims{UserID: "u-1", Role: "user", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	token, _ := SignJWT(claims, testSecret)

	// Re-sign the payload with escalated role but keep the original signature.
	parts := splitToken(t, token)
	forged, _ := json.Marshal(JWTClaims{UserID: "u-1", Role: "admin", ExpiresAt: claims.ExpiresAt})
	forgedToken := parts[0] + "." + base64.RawURLEncoding.EncodeToString(forged) + "." + parts[2]

	if _, err := VerifyJWT(forgedToken, testSecret); !errors.Is(err, ErrSignatureFail) {
		t.Errorf("VerifyJWT tampered payload: err = %v, want ErrSignatureFail", err)
	}
}

func TestJWTMalformed(t *testing.T) {
	cases := []string{
		"",
		"not-a-jwt",
		"only.two",
		"a.b.c.d", // 4 segments
		"eyJhbGciOiJIUzI1NiJ9.!!!not-base64!!!.c2ln",
	}
	for _, token := range cases {
		if _, err := VerifyJWT(token, testSecret); err == nil {
			t.Errorf("VerifyJWT(%q) = nil error, want error", token)
		}
	}
}

func splitToken(t *testing.T, token string) []string {
	t.Helper()
	parts := make([]string, 0, 3)
	start := 0
	for i := 0; i <= len(token); i++ {
		if i == len(token) || token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	if len(parts) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(parts))
	}
	return parts
}
