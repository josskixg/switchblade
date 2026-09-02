// Package crypto — AES-256-GCM encrypt/decrypt for account passwords.
package crypto

// Encrypt encrypts with AES-256-GCM and returns base64-encoded ciphertext.
func Encrypt(plaintext, key string) string {
	if key == "" || plaintext == "" {
		return plaintext
	}
	out, err := EncryptAES([]byte(plaintext), []byte(key))
	if err != nil {
		return ""
	}
	return string(out)
}

// Decrypt decrypts AES-256-GCM base64-encoded ciphertext.
func Decrypt(ciphertext, key string) string {
	if key == "" || ciphertext == "" {
		return ciphertext
	}
	out, err := DecryptAES(ciphertext, []byte(key))
	if err != nil {
		return ""
	}
	return string(out)
}
