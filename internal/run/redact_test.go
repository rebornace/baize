package run

import (
	"strings"
	"testing"
)

func TestRedactSensitiveText(t *testing.T) {
	cases := []string{
		`request failed: Authorization: Bearer abcdefghijklmnop`,
		`{"api_key":"sk-1234567890abcdef","ok":false}`,
		`token=supersecret&user=bob`,
		`password: hunter2`,
	}
	for _, in := range cases {
		got := redactSensitiveText(in, 0)
		for _, secret := range []string{"abcdefghijklmnop", "sk-1234567890abcdef", "supersecret", "hunter2"} {
			if strings.Contains(got, secret) {
				t.Fatalf("redaction leaked %q: %s", secret, got)
			}
		}
		if !strings.Contains(got, "***") {
			t.Fatalf("redaction marker missing: %s", got)
		}
	}
	if got := redactSensitiveText("plain error", 0); got != "plain error" {
		t.Fatalf("plain text changed: %q", got)
	}
}
