package llm

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

const (
	// DefaultAPIMaxRetries is used when NewOpenAI / config leave retries unset.
	DefaultAPIMaxRetries = 3
	retryBaseBackoff     = 100 * time.Millisecond
	retryMaxBackoff      = 2 * time.Second
)

// RetryableHTTP reports whether a failed LLM HTTP attempt should be retried.
// Mirrors webhook.Retryable: transport errors, 429, and 5xx.
func RetryableHTTP(statusCode int, err error) bool {
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) {
			return true
		}
		return true
	}
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	if statusCode >= 500 {
		return true
	}
	return false
}

func retryBackoff(failedAttempts int) time.Duration {
	if failedAttempts <= 0 {
		return retryBaseBackoff
	}
	d := retryBaseBackoff
	for i := 1; i < failedAttempts; i++ {
		d *= 2
		if d >= retryMaxBackoff {
			return retryMaxBackoff
		}
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
