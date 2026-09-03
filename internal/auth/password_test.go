package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestHashPasswordBcryptRoundtrip(t *testing.T) {
	hash, err := HashPassword("s3cret-pass!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("HashPassword produced non-bcrypt hash: %q", hash)
	}
	if !VerifyPassword("s3cret-pass!", hash) {
		t.Error("VerifyPassword(correct) = false, want true")
	}
	if VerifyPassword("wrong-pass", hash) {
		t.Error("VerifyPassword(wrong) = true, want false")
	}
}

func TestVerifyPasswordLegacySHA256(t *testing.T) {
	// Legacy storage format: "<salt>:<hex(sha256(salt+password))>".
	salt := "legacy-salt-123"
	sum := sha256.Sum256([]byte(salt + "old-password"))
	stored := salt + ":" + hex.EncodeToString(sum[:])

	if !VerifyPassword("old-password", stored) {
		t.Error("VerifyPassword(legacy correct) = false, want true")
	}
	if VerifyPassword("new-password", stored) {
		t.Error("VerifyPassword(legacy wrong) = true, want false")
	}
}

func TestVerifyPasswordGarbageStored(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$2c$invalidbcrypt", // bcrypt prefix, malformed body
		"no-colon-here",     // legacy parser needs exactly one colon-separated hex hash
		"salt:not-hex",      // legacy hash portion must be hex
		"a:b:c",             // salt containing a colon misparses -> must not panic/verify
	}
	for _, stored := range cases {
		if VerifyPassword("whatever", stored) {
			t.Errorf("VerifyPassword with stored=%q = true, want false", stored)
		}
	}
}

func TestGenerateToken(t *testing.T) {
	tok := GenerateToken(16)
	if len(tok) != 32 { // 16 bytes -> 32 hex chars
		t.Errorf("GenerateToken(16) length = %d, want 32", len(tok))
	}
	for _, r := range tok {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Errorf("GenerateToken produced non-hex char %q", r)
		}
	}
	if tok == GenerateToken(16) {
		t.Error("GenerateToken returned identical value twice")
	}
}
