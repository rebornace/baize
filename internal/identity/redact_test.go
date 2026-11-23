package identity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactTextBearerAndKeyValue(t *testing.T) {
	cases := []struct {
		in     string
		secret string
	}{
		{`upstream said Authorization: Bearer abcdefghijklmnop`, "abcdefghijklmnop"},
		{`{"api_key":"sk-1234567890abcdef","ok":false}`, "sk-1234567890abcdef"},
		{`token=supersecret&user=bob`, "supersecret"},
		{`password: hunter2`, "hunter2"},
	}
	for _, tc := range cases {
		got := RedactText(tc.in)
		if strings.Contains(got, tc.secret) {
			t.Fatalf("leaked %q: %s", tc.secret, got)
		}
		if !strings.Contains(got, "***") {
			t.Fatalf("marker missing: %s", got)
		}
	}
	if got := RedactText("plain error"); got != "plain error" {
		t.Fatalf("plain text changed: %q", got)
	}
}

func TestRedactSensitiveNestedTokens(t *testing.T) {
	in := map[string]any{
		"email":       "a@x.com",
		"accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.sig",
		"data": map[string]any{
			"token": "secret-jwt",
			"ok":    true,
		},
		"items": []any{
			map[string]any{"access_token": "abc", "n": 1},
		},
	}
	got, ok := RedactSensitive(in).(map[string]any)
	if !ok {
		t.Fatalf("type=%T", RedactSensitive(in))
	}
	if got["accessToken"] != "[redacted]" {
		t.Fatalf("accessToken=%v", got["accessToken"])
	}
	if got["email"] != "a@x.com" {
		t.Fatalf("email mutated: %v", got["email"])
	}
	data := got["data"].(map[string]any)
	if data["token"] != "[redacted]" || data["ok"] != true {
		t.Fatalf("data=%v", data)
	}
	item := got["items"].([]any)[0].(map[string]any)
	if item["access_token"] != "[redacted]" || item["n"] != 1 {
		t.Fatalf("items[0]=%v", item)
	}
	// Original untouched.
	if in["accessToken"] == "[redacted]" {
		t.Fatal("input mutated")
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "eyJ") || strings.Contains(string(raw), "secret-jwt") {
		t.Fatalf("leaked token in %s", raw)
	}
}
