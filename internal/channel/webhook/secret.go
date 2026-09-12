package webhook

import (
	"crypto/rand"
	"encoding/hex"
)

// generateSecret returns a 32-byte random HMAC secret, hex-encoded, used for
// autostart adapters where baize and the child process share an ephemeral key.
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
