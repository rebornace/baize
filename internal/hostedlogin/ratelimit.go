package hostedlogin

import (
	"sync"
	"time"
)

// DefaultRateLimit is the per-key request budget per window for hosted pages.
const DefaultRateLimit = 30

// DefaultRateWindow is the sliding window for hosted login rate limiting.
const DefaultRateWindow = time.Minute

// RateLimiter enforces a per-key sliding-window budget (IP and/or session).
type RateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	requests map[string][]time.Time
}

// NewRateLimiter returns a limiter allowing limit requests per window per key.
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

// Allow reports whether key may accept one more request now.
func (rl *RateLimiter) Allow(key string) bool {
	if rl == nil {
		return true
	}
	return rl.allowAt(key, time.Now())
}

func (rl *RateLimiter) allowAt(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := now.Add(-rl.window)
	ts := rl.requests[key]
	kept := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.limit {
		rl.requests[key] = kept
		return false
	}
	kept = append(kept, now)
	rl.requests[key] = kept
	return true
}

// AllowHosted checks both IP and session budgets. Empty parts are skipped.
func (rl *RateLimiter) AllowHosted(ip, sessionID string) bool {
	if rl == nil {
		return true
	}
	if ip != "" && !rl.Allow("ip:"+ip) {
		return false
	}
	if sessionID != "" && !rl.Allow("sess:"+sessionID) {
		return false
	}
	return true
}
