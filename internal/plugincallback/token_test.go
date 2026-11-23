package plugincallback

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

const testSecret = "super-secret-key"

func TestIssueVerifyRoundTrip(t *testing.T) {
	secret := []byte(testSecret)
	runID := "run_abc"
	ttl := time.Hour
	before := time.Now()

	token, exp, err := Issue(secret, runID, ttl)
	if err != nil {
		t.Fatalf("Issue err: %v", err)
	}
	wantMin := before.Add(ttl - time.Second)
	wantMax := time.Now().Add(ttl + time.Second)
	if exp.Before(wantMin) || exp.After(wantMax) {
		t.Fatalf("exp=%v want within (%v,%v)", exp, wantMin, wantMax)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if err := Verify(secret, runID, token, time.Now()); err != nil {
		t.Fatalf("Verify err: %v", err)
	}
}

func TestVerifyExpired(t *testing.T) {
	secret := []byte(testSecret)
	runID := "run_expired"
	ttl := time.Minute
	token, exp, err := Issue(secret, runID, ttl)
	if err != nil {
		t.Fatalf("Issue err: %v", err)
	}
	// 1 second past expiry
	if err := Verify(secret, runID, token, exp.Add(time.Second)); err == nil {
		t.Fatal("expected expired error, got nil")
	}
}

func TestVerifyTamperedToken(t *testing.T) {
	secret := []byte(testSecret)
	runID := "run_tamper"
	ttl := time.Hour
	now := time.Unix(1_700_000_000, 0)
	token, _, err := Issue(secret, runID, ttl)
	if err != nil {
		t.Fatalf("Issue err: %v", err)
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("token parts=%d", len(parts))
	}
	// Flip a char in the middle of the signature. The last char of a
	// RawURLEncoding string may carry only padding bits (ignored on decode),
	// so flipping it can be a no-op; a middle char always carries real bits.
	sig := parts[1]
	if len(sig) < 2 {
		t.Fatalf("signature too short: %q", sig)
	}
	mid := len(sig) / 2
	c := sig[mid]
	var flipped byte
	if c == 'A' {
		flipped = 'B'
	} else {
		flipped = 'A'
	}
	tampered := parts[0] + "." + sig[:mid] + string(flipped) + sig[mid+1:]
	if err := Verify(secret, runID, tampered, now); err == nil {
		t.Fatal("expected tamper error, got nil")
	}
}

func TestVerifyWrongRunID(t *testing.T) {
	secret := []byte(testSecret)
	ttl := time.Hour
	now := time.Unix(1_700_000_000, 0)
	token, _, err := Issue(secret, "run_a", ttl)
	if err != nil {
		t.Fatalf("Issue err: %v", err)
	}
	if err := Verify(secret, "run_b", token, now); err == nil {
		t.Fatal("expected wrong-run error, got nil")
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	runID := "run_secret"
	ttl := time.Hour
	now := time.Unix(1_700_000_000, 0)
	token, _, err := Issue([]byte("secret-a"), runID, ttl)
	if err != nil {
		t.Fatalf("Issue err: %v", err)
	}
	if err := Verify([]byte("secret-b"), runID, token, now); err == nil {
		t.Fatal("expected wrong-secret error, got nil")
	}
}

func TestVerifyMalformedToken(t *testing.T) {
	secret := []byte(testSecret)
	now := time.Unix(1_700_000_000, 0)
	for _, bad := range []string{"", "no-separator", "notb64.notb64", "////.////"} {
		if err := Verify(secret, "run_x", bad, now); err == nil {
			t.Fatalf("expected err for %q, got nil", bad)
		}
	}
}

func TestVerifyExpMismatch(t *testing.T) {
	// Forge a token whose exp differs from the bound exp used in HMAC.
	secret := []byte(testSecret)
	runID := "run_exp_mismatch"
	now := time.Unix(1_700_000_000, 0)
	token, _, err := Issue(secret, runID, time.Hour)
	if err != nil {
		t.Fatalf("Issue err: %v", err)
	}
	parts := strings.SplitN(token, ".", 2)
	// Re-encode a different exp but keep original signature
	fakeExp := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", now.Add(2*time.Hour).Unix())))
	forced := fakeExp + "." + parts[1]
	if err := Verify(secret, runID, forced, now); err == nil {
		t.Fatal("expected exp-mismatch error, got nil")
	}
}
