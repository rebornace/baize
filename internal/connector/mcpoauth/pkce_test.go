package mcpoauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	v, c, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 43 {
		t.Fatalf("verifier length %d, want 43 (32 random bytes, base64url)", len(v))
	}
	if len(v) > 128 {
		t.Fatalf("verifier length %d exceeds RFC 7636 max 128", len(v))
	}
	sum := sha256.Sum256([]byte(v))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if c != want {
		t.Fatalf("challenge %q, want %q", c, want)
	}
}
