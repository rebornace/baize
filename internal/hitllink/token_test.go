package hitllink

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIssueVerifyRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Unix(1_700_000_000, 0).UTC()
	wait := now.Add(-2 * time.Minute)
	token, exp, err := Issue(secret, "run_1", wait, 24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if exp.Unix() != wait.Add(24*time.Hour).Unix() {
		t.Fatalf("exp=%v", exp)
	}
	if got := len(strings.Split(token, ".")); got != 3 {
		t.Fatalf("token parts=%d token=%q", got, token)
	}
	claims, err := Verify(secret, token, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if claims.RunID != "run_1" {
		t.Fatalf("run=%q", claims.RunID)
	}
	if claims.WaitUnixNano != wait.UnixNano() {
		t.Fatalf("wait=%d want %d", claims.WaitUnixNano, wait.UnixNano())
	}
	if claims.ExpiresAt.Unix() != exp.Unix() {
		t.Fatalf("claims exp=%v want %v", claims.ExpiresAt, exp)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	token, _, err := Issue([]byte("right"), "run_1", now, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify([]byte("wrong"), token, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err=%v want ErrBadSignature", err)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	token, _, err := Issue([]byte("s"), "run_1", now, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify([]byte("s"), token, now.Add(time.Hour)); !errors.Is(err, ErrExpired) {
		t.Fatalf("err=%v want ErrExpired", err)
	}
}

func TestVerifyRejectsTamperedToken(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	token, _, err := Issue([]byte("s"), "run_1", now, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	parts[1] = parts[1] + "A"
	tampered := strings.Join(parts, ".")
	if _, err := Verify([]byte("s"), tampered, now); err == nil {
		t.Fatal("tampered payload must fail")
	}
	if _, err := Verify([]byte("s"), token+"x", now); err == nil {
		t.Fatal("tampered signature must fail")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	for _, token := range []string{"", "v1", "v1.only", "v1.a.b.c", "v2.a.b"} {
		if _, err := Verify([]byte("s"), token, now); !errors.Is(err, ErrMalformed) {
			t.Fatalf("token=%q err=%v want ErrMalformed", token, err)
		}
	}
}

func TestIssueRejectsBadInput(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	if _, _, err := Issue(nil, "run_1", now, time.Hour, now); err == nil {
		t.Fatal("empty secret must fail")
	}
	if _, _, err := Issue([]byte("s"), "", now, time.Hour, now); err == nil {
		t.Fatal("empty run must fail")
	}
	if _, _, err := Issue([]byte("s"), "run_1", time.Time{}, time.Hour, now); err == nil {
		t.Fatal("zero wait must fail")
	}
	if _, _, err := Issue([]byte("s"), "run_1", now, 0, now); err == nil {
		t.Fatal("non-positive ttl must fail")
	}
	if _, _, err := Issue([]byte("s"), "run_1", now, time.Hour, now.Add(2*time.Hour)); err == nil {
		t.Fatal("already-expired wait must fail")
	}
}

func TestFormatURL(t *testing.T) {
	if got := FormatURL("https://baize.example/", "tok"); got != "https://baize.example/v0/hitl/approvals/tok" {
		t.Fatalf("url=%q", got)
	}
	if got := FormatURL("  ", "tok"); got != "" {
		t.Fatalf("empty base url=%q", got)
	}
	if got := FormatURL("https://baize.example", ""); got != "" {
		t.Fatalf("empty token url=%q", got)
	}
}
