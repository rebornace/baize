package mcpexport

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	keyPrefix      = "bze_"
	keyRandomBytes = 32
	keyPrefixLen   = 8
)

// GenerateKey creates a new export API key plaintext, its sha256 hex hash, and an 8-char prefix.
func GenerateKey() (plaintext string, hash string, prefix string, err error) {
	b := make([]byte, keyRandomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate key entropy: %w", err)
	}
	plaintext = keyPrefix + hex.EncodeToString(b)
	hash = HashKey(plaintext)
	if len(plaintext) < keyPrefixLen {
		return "", "", "", fmt.Errorf("generated key too short")
	}
	prefix = plaintext[:keyPrefixLen]
	return plaintext, hash, prefix, nil
}

// HashKey returns the sha256 hex digest of a plaintext export key.
func HashKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// CheckKey reports whether plaintext matches the stored hash.
func CheckKey(plaintext, hash string) bool {
	if plaintext == "" || hash == "" {
		return false
	}
	return HashKey(plaintext) == hash
}
