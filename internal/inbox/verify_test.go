package inbox_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/inbox"
)

func TestSignAndVerifyOK(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"input":"hi"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := inbox.Sign(secret, ts, body)
	if err := inbox.Verify(secret, ts, body, sig, time.Now(), 300*time.Second); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyRejectsSkew(t *testing.T) {
	secret := "s"
	body := []byte(`{}`)
	ts := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	sig := inbox.Sign(secret, ts, body)
	if err := inbox.Verify(secret, ts, body, sig, time.Now(), 300*time.Second); err != inbox.ErrTimestampSkew {
		t.Fatalf("err=%v want skew", err)
	}
}
