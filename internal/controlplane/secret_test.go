package controlplane

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSecretEmpty(t *testing.T) {
	got, err := ResolveSecret("")
	if err != nil || got != "" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	got, err = ResolveSecret("   ")
	if err != nil || got != "" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolveSecretEnvUnsetIsEmpty(t *testing.T) {
	t.Setenv("BAIZE_OPERATOR_TOKEN_TEST_UNSET", "")
	_ = os.Unsetenv("BAIZE_OPERATOR_TOKEN_TEST_MISSING")
	got, err := ResolveSecret("env:BAIZE_OPERATOR_TOKEN_TEST_UNSET")
	if err != nil || got != "" {
		t.Fatalf("unset env must be empty, not error: %q %v", got, err)
	}
	got, err = ResolveSecret("env:BAIZE_OPERATOR_TOKEN_TEST_MISSING")
	if err != nil || got != "" {
		t.Fatalf("missing env must be empty, not error: %q %v", got, err)
	}
}

func TestResolveSecretEnvSet(t *testing.T) {
	t.Setenv("BAIZE_OPERATOR_TOKEN_TEST_SET", " op-secret ")
	got, err := ResolveSecret("env:BAIZE_OPERATOR_TOKEN_TEST_SET")
	if err != nil || got != "op-secret" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolveSecretPlaintext(t *testing.T) {
	got, err := ResolveSecret("plain-token")
	if err != nil || got != "plain-token" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolveSecretFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tok")
	if err := os.WriteFile(p, []byte(" file-secret \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveSecret("file:" + p)
	if err != nil || got != "file-secret" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	_, err = ResolveSecret("file:" + filepath.Join(dir, "missing"))
	if err == nil {
		t.Fatal("missing file must error")
	}
}
