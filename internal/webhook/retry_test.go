package webhook

import (
	"net"
	"testing"
	"time"
)

func TestRetryable(t *testing.T) {
	t.Parallel()
	if !Retryable(503, nil) {
		t.Fatal("503 should be retryable")
	}
	if !Retryable(429, nil) {
		t.Fatal("429 should be retryable")
	}
	if !Retryable(0, &net.OpError{Op: "dial", Err: net.ErrClosed}) {
		t.Fatal("network error should be retryable")
	}
	if Retryable(400, nil) {
		t.Fatal("400 should not be retryable")
	}
	if Retryable(404, nil) {
		t.Fatal("404 should not be retryable")
	}
}

func TestBackoff(t *testing.T) {
	t.Parallel()
	cases := []struct {
		failed int
		want   time.Duration
	}{
		{0, time.Second},
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{10, maxBackoff},
	}
	for _, tc := range cases {
		if got := Backoff(tc.failed); got != tc.want {
			t.Fatalf("Backoff(%d)=%v want %v", tc.failed, got, tc.want)
		}
	}
}
