package secret

import (
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}
	return key
}

func TestEncryptDecrypt(t *testing.T) {
	enc, err := NewEncryptor(testKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	plaintext := []byte(`{"api_key":"sk-test123","org_id":"org-456"}`)

	ciphertext, nonce, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	if string(ciphertext) == string(plaintext) {
		t.Error("ciphertext should differ from plaintext")
	}

	decrypted, err := enc.Decrypt(ciphertext, nonce)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	enc1, _ := NewEncryptor(testKey(t))
	enc2, _ := NewEncryptor(testKey(t))

	ciphertext, nonce, _ := enc1.Encrypt([]byte("secret data"))

	_, err := enc2.Decrypt(ciphertext, nonce)
	if err == nil {
		t.Error("Decrypt() with wrong key should fail")
	}
}

func TestNewEncryptor_InvalidKey(t *testing.T) {
	_, err := NewEncryptor("not-hex")
	if err == nil {
		t.Error("NewEncryptor() with invalid hex should fail")
	}

	_, err = NewEncryptor("aabbccdd") // too short
	if err == nil {
		t.Error("NewEncryptor() with short key should fail")
	}
}

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}
	if len(key) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("key length = %d, want 64", len(key))
	}
}
