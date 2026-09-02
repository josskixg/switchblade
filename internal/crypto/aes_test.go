package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptAES_RoundTrip(t *testing.T) {
	key := []byte("mysecretkey")
	plain := []byte("hello, world")
	enc, err := EncryptAES(plain, key)
	if err != nil {
		t.Fatalf("EncryptAES: %v", err)
	}
	dec, err := DecryptAES(string(enc), key)
	if err != nil {
		t.Fatalf("DecryptAES: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("want %q, got %q", plain, dec)
	}
}

func TestEncryptAES_DifferentNonce(t *testing.T) {
	key := []byte("mysecretkey")
	plain := []byte("same plaintext")
	enc1, _ := EncryptAES(plain, key)
	enc2, _ := EncryptAES(plain, key)
	if string(enc1) == string(enc2) {
		t.Fatal("encrypting same plaintext twice produced identical output (nonce not random)")
	}
}

func TestDecryptAES_WrongKey(t *testing.T) {
	key := []byte("correctkey")
	enc, _ := EncryptAES([]byte("secret"), key)
	_, err := DecryptAES(string(enc), []byte("wrongkey"))
	if err == nil {
		t.Fatal("expected error with wrong key, got nil")
	}
}

func TestDecryptAES_InvalidBase64(t *testing.T) {
	_, err := DecryptAES("not-valid-base64!!!", []byte("key"))
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestNewAESKey(t *testing.T) {
	k1, err := NewAESKey()
	if err != nil {
		t.Fatalf("NewAESKey: %v", err)
	}
	if len(k1) != 32 {
		t.Fatalf("want 32 bytes, got %d", len(k1))
	}
	k2, _ := NewAESKey()
	if bytes.Equal(k1, k2) {
		t.Fatal("two generated keys are identical (not random)")
	}
}

func TestNormalizeKey_Short(t *testing.T) {
	k := normalizeKey([]byte("short"))
	if len(k) != 32 {
		t.Fatalf("want 32, got %d", len(k))
	}
}

func TestNormalizeKey_Long(t *testing.T) {
	k := normalizeKey(make([]byte, 64))
	if len(k) != 32 {
		t.Fatalf("want 32, got %d", len(k))
	}
}
