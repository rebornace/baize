package authcred_test

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/authcred"
)

func TestResolveStaticExpandsEnv(t *testing.T) {
	t.Setenv("BAIZE_CONNECTOR_TOKEN", "tok-static")
	got, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeStatic,
		Static: authcred.Static{Headers: map[string]string{
			"Authorization": "Bearer ${BAIZE_CONNECTOR_TOKEN}",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["Authorization"] != "Bearer tok-static" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveStaticMissingEnv(t *testing.T) {
	_, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeStatic,
		Static: authcred.Static{Headers: map[string]string{
			"Authorization": "Bearer ${MISSING_TOKEN_XYZ}",
		}},
	})
	if !errors.Is(err, authcred.ErrInvalidAuth) {
		t.Fatalf("err=%v", err)
	}
}

func TestResolveVaultEnvAndFile(t *testing.T) {
	t.Setenv("VAULT_ENV_TOK", "from-env")
	p := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(p, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeVaultRef,
		VaultRef: authcred.VaultRef{Headers: map[string]string{
			"X-Env":  "env:VAULT_ENV_TOK",
			"X-File": "file:" + p,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["X-Env"] != "from-env" || got["X-File"] != "from-file" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveVaultFilePathTrimSpace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(p, []byte("trimmed-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeVaultRef,
		VaultRef: authcred.VaultRef{Headers: map[string]string{
			"X-SpaceBefore": "file: " + p,
			"X-SpaceAfter":  "file:" + p + " ",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["X-SpaceBefore"] != "trimmed-token" || got["X-SpaceAfter"] != "trimmed-token" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveVaultUnknownPrefix(t *testing.T) {
	_, err := authcred.ResolveDefaults(authcred.Config{
		Mode:     authcred.ModeVaultRef,
		VaultRef: authcred.VaultRef{Headers: map[string]string{"Authorization": "secret:x"}},
	})
	if !errors.Is(err, authcred.ErrInvalidAuth) {
		t.Fatalf("err=%v", err)
	}
}

func TestResolvePassthroughHasNoDefaults(t *testing.T) {
	got, err := authcred.ResolveDefaults(authcred.Config{Mode: authcred.ModePassthrough})
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestPickPassthroughWhitelist(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer IN")
	h.Set("Cookie", "secret=1")
	h.Set("X-Ignored", "nope")
	got := authcred.PickHeaders(h, []string{"Authorization"})
	if got["Authorization"] != "Bearer IN" || len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	empty := authcred.PickHeaders(h, nil) // nil → 缺省仅 Authorization
	if empty["Authorization"] != "Bearer IN" {
		t.Fatalf("default whitelist: %+v", empty)
	}
	none := authcred.PickHeaders(h, []string{}) // 显式空 → 无头
	if len(none) != 0 {
		t.Fatalf("explicit empty: %+v", none)
	}
}

func TestUnknownMode(t *testing.T) {
	_, err := authcred.ResolveDefaults(authcred.Config{Mode: "ldap"})
	if !errors.Is(err, authcred.ErrInvalidAuth) {
		t.Fatalf("err=%v", err)
	}
}
