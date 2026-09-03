// JWT sign/verify — pure stdlib HS256 implementation.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// JWTClaims holds the payload for a JWT token.
type JWTClaims struct {
	UserID    string `json:"user_id"`
	TenantID  string `json:"tenant_id"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

var (
	ErrInvalidToken  = errors.New("invalid token")
	ErrExpiredToken  = errors.New("token expired")
	ErrSignatureFail = errors.New("signature verification failed")
)

// base64url encodes bytes without padding.
func b64url(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// SignJWT creates a HS256 JWT token.
func SignJWT(claims JWTClaims, secret []byte) (string, error) {
	hdr := jwtHeader{Alg: "HS256", Typ: "JWT"}
	hdrJSON, err := json.Marshal(hdr)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := b64url(hdrJSON) + "." + b64url(payload)
	sig := hmacSHA256([]byte(signingInput), secret)
	return signingInput + "." + b64url(sig), nil
}

// VerifyJWT validates and returns claims from a JWT token.
func VerifyJWT(token string, secret []byte) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Reject unexpected algs (alg confusion / "none").
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var hdr jwtHeader
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil || hdr.Alg != "HS256" {
		return nil, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := hmacSHA256([]byte(signingInput), secret)
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, ErrSignatureFail
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if claims.ExpiresAt == 0 || claims.ExpiresAt < time.Now().Unix() {
		return nil, ErrExpiredToken
	}
	if claims.UserID == "" || claims.TenantID == "" {
		return nil, ErrInvalidToken
	}

	return &claims, nil
}

func hmacSHA256(data, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}
