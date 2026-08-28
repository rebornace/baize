package webhook

import (
	"errors"
	"net"
	"strconv"
	"time"
)

const maxBackoff = 60 * time.Second

// Retryable reports whether a failed POST should be retried with backoff.
func Retryable(statusCode int, err error) bool {
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) {
			return true
		}
		return true
	}
	if statusCode == 429 {
		return true
	}
	if statusCode >= 500 {
		return true
	}
	return false
}

// Backoff returns the wait duration after attempt failures (1-based attempt count).
func Backoff(failedAttempts int) time.Duration {
	if failedAttempts <= 0 {
		return time.Second
	}
	d := time.Second
	for i := 1; i < failedAttempts; i++ {
		d *= 2
		if d >= maxBackoff {
			return maxBackoff
		}
	}
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func formatDeliveryError(statusCode int, err error) string {
	if err != nil {
		return err.Error()
	}
	if statusCode > 0 {
		return ErrDeliveryFailed.Error() + " (HTTP " + strconv.Itoa(statusCode) + ")"
	}
	return ErrDeliveryFailed.Error()
}
