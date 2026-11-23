package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStoreOverlay(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "minimal.yaml")
	if err := os.WriteFile(base, []byte("listen: :8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteStoreOverlay(base, StoreOverlay{
		Driver: "postgres",
		DSN:    "postgres://u:p@localhost/baize",
	}); err != nil {
		t.Fatal(err)
	}
	overlay := OverlayWritePath(base)
	b, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "postgres") {
		t.Fatalf("overlay=%q", string(b))
	}
}

func TestRedactDSN(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "url form masks userinfo password",
			in:   "postgres://user:secret@localhost:5432/baize?sslmode=disable",
			want: "postgres://user:***@localhost:5432/baize?sslmode=disable",
		},
		{
			name: "url form without password unchanged",
			in:   "postgres://user@localhost:5432/baize",
			want: "postgres://user@localhost:5432/baize",
		},
		{
			name: "keyword form masks unquoted password",
			in:   "host=localhost user=app password=s3cret dbname=baize",
			want: "host=localhost user=app password=*** dbname=baize",
		},
		{
			name: "keyword form masks quoted password with spaces",
			in:   "host=localhost password='my s3cret!' dbname=baize",
			want: "host=localhost password=*** dbname=baize",
		},
		{
			name: "keyword form masks double-quoted escaped password",
			in:   `host=localhost password="p""w" user=app`,
			want: "host=localhost password=*** user=app",
		},
		{
			name: "keyword form masks password even when first",
			in:   "password=topsecret host=localhost",
			want: "password=*** host=localhost",
		},
		{
			name: "keyword form case-insensitive key",
			in:   "host=localhost PASSWORD=topsecret",
			want: "host=localhost PASSWORD=***",
		},
		{
			name: "url form masks password in query string",
			in:   "postgres://localhost:5432/baize?password=topsecret&sslmode=disable",
			want: "postgres://localhost:5432/baize?password=***&sslmode=disable",
		},
		{
			name: "empty stays empty",
			in:   "",
			want: "",
		},
		{
			name: "non-dsn text unchanged",
			in:   "hello world",
			want: "hello world",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactDSN(tc.in)
			if got != tc.want {
				t.Fatalf("RedactDSN(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if tc.in != "" && strings.Contains(tc.in, "secret") && strings.Contains(got, "secret") {
				t.Fatalf("redacted output leaked credential: %q", got)
			}
		})
	}
}
