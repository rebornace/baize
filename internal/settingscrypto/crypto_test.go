package settingscrypto_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/settingscrypto"
)

func TestSealOpenRoundTrip(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	key, err := settingscrypto.KeyFromEnv()
	if err != nil || key == nil {
		t.Fatalf("KeyFromEnv: %v", err)
	}
	sealed, err := settingscrypto.Seal(key, "sk-secret-1234")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "bz1:") || sealed == "sk-secret-1234" {
		t.Fatalf("sealed=%q", sealed)
	}
	plain, err := settingscrypto.Open(key, sealed)
	if err != nil || plain != "sk-secret-1234" {
		t.Fatalf("open=%q err=%v", plain, err)
	}
}

func TestOpenPlaintextPassthrough(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	key, _ := settingscrypto.KeyFromEnv()
	got, err := settingscrypto.Open(key, "already-plain")
	if err != nil || got != "already-plain" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestSealWithoutKey(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "")
	key, err := settingscrypto.KeyFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if key != nil {
		t.Fatal("empty env must yield nil key")
	}
	_, err = settingscrypto.Seal(nil, "x")
	if !errors.Is(err, settingscrypto.ErrNoKey) {
		t.Fatalf("want ErrNoKey, got %v", err)
	}
}

func TestOpenCiphertextWithoutKey(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	key, _ := settingscrypto.KeyFromEnv()
	sealed, _ := settingscrypto.Seal(key, "secret")
	t.Setenv("BAIZE_SETTINGS_KEY", "")
	_, err := settingscrypto.Open(nil, sealed)
	if !errors.Is(err, settingscrypto.ErrCiphertext) {
		t.Fatalf("want ErrCiphertext, got %v", err)
	}
}
