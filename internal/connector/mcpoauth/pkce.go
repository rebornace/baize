package mcpoauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GeneratePKCE returns a RFC 7636 code_verifier and S256 code_challenge (base64url, no padding).
func GeneratePKCE() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("pkce random: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	if len(verifier) < 43 {
		return "", "", fmt.Errorf("pkce verifier too short")
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}
