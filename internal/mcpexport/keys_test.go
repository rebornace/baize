package mcpexport_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
)

func TestGenerateKeyFormat(t *testing.T) {
	plaintext, hash, prefix, err := mcpexport.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if !strings.HasPrefix(plaintext, "bze_") {
		t.Fatalf("plaintext=%q", plaintext)
	}
	if len(prefix) != 8 || prefix != plaintext[:8] {
		t.Fatalf("prefix=%q plaintext=%q", prefix, plaintext)
	}
	if len(hash) != 64 {
		t.Fatalf("hash len=%d", len(hash))
	}
	if !mcpexport.CheckKey(plaintext, hash) {
		t.Fatal("CheckKey should match generated hash")
	}
	if mcpexport.CheckKey(plaintext, "deadbeef") {
		t.Fatal("CheckKey should reject wrong hash")
	}
}

func TestHashKeyDeterministic(t *testing.T) {
	const sample = "bze_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got := mcpexport.HashKey(sample)
	if got == "" || got == sample {
		t.Fatalf("HashKey=%q", got)
	}
	if !mcpexport.CheckKey(sample, got) {
		t.Fatal("CheckKey should match HashKey output")
	}
}
