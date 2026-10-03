package webhooksig

import (
	"strings"
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	secret := "topsecret"
	body := []byte(`{"hello":"world"}`)
	ts := "1700000000"
	sig := Sign(secret, ts, body)
	if !strings.HasPrefix(sig, SignaturePrefix) {
		t.Fatalf("sig missing prefix: %q", sig)
	}
	if err := Verify(secret, ts, body, sig, time.Unix(1700000000, 0), 300*time.Second); err != nil {
		t.Fatalf("verify round trip: %v", err)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	body := []byte("x")
	ts := "1700000000"
	sig := Sign("right", ts, body)
	if err := Verify("wrong", ts, body, sig, time.Unix(1700000000, 0), 300*time.Second); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestVerifyRejectsSkew(t *testing.T) {
	body := []byte("x")
	ts := "1700000000"
	sig := Sign("s", ts, body)
	// request time 1000s away from now, maxSkew 300s
	if err := Verify("s", ts, body, sig, time.Unix(1700001000, 0), 300*time.Second); err == nil {
		t.Fatal("expected skew error")
	}
}

func TestVerifyRejectsBadHeader(t *testing.T) {
	body := []byte("x")
	if err := Verify("s", "1700000000", body, "garbage", time.Unix(1700000000, 0), 300*time.Second); err == nil {
		t.Fatal("expected error for malformed header")
	}
	if err := Verify("s", "not-a-number", body, SignaturePrefix+"aa", time.Unix(1700000000, 0), 300*time.Second); err == nil {
		t.Fatal("expected error for bad timestamp")
	}
}
