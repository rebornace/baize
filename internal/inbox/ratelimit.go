package inbox

import (
	"sync"
	"time"
)

// DefaultRateLimit is the per-channel inbound request budget per minute.
const DefaultRateLimit = 120

// DefaultRateWindow is the sliding window for inbound rate limiting.
const DefaultRateWindow = time.Minute

// RateLimiter enforces a per-channel sliding-window request budget.
type RateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	requests map[string][]time.Time
}

// NewRateLimiter returns a limiter allowing limit requests per window per channel.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = DefaultRateLimit
	}
	if window <= 0 {
		window = DefaultRateWindow
	}
	return &RateLimiter{
		limit:    limit,
		window:   window,
		requests: make(map[string][]time.Time),
	}
}

// Allow reports whether channelID may accept one more request at the current time.
func (rl *RateLimiter) Allow(channelID string) bool {
	return rl.allowAt(channelID, time.Now())
}

func (rl *RateLimiter) allowAt(channelID string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := now.Add(-rl.window)
	ts := rl.requests[channelID]
	kept := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.limit {
		rl.requests[channelID] = kept
		return false
	}
	kept = append(kept, now)
	rl.requests[channelID] = kept
	return true
}
