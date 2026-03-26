package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

const keyLength = 32 // 32 bytes = 64 hex chars

// GenerateAPIKey creates a new API key and returns (rawKey, keyHash, keyPrefix).
// The raw key is shown to the user once; only the hash is stored.
func GenerateAPIKey() (rawKey, keyHash, keyPrefix string, err error) {
	buf := make([]byte, keyLength)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", "", "", fmt.Errorf("generating key: %w", err)
	}

	rawKey = "mk_" + hex.EncodeToString(buf) // mk_ prefix for easy identification
	keyHash = HashAPIKey(rawKey)
	keyPrefix = rawKey[:10] // mk_XXXXXXX for display

	return rawKey, keyHash, keyPrefix, nil
}

// HashAPIKey returns the SHA-256 hash of an API key.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
