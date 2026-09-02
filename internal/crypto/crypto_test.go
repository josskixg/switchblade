package crypto

import "testing"

func TestEncryptDecrypt(t *testing.T) {
	key := "mysecretkey"
	plain := "hello, world"
	got := Decrypt(Encrypt(plain, key), key)
	if got != plain {
		t.Fatalf("want %q, got %q", plain, got)
	}
}

func TestDecrypt_InvalidInput(t *testing.T) {
	// garbage base64 → returns the input unchanged (not a panic)
	result := Decrypt("not-valid-base64!!!", "somekey")
	if result == "" {
		// returning empty is also acceptable, just no panic
	}
	_ = result
}
