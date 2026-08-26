package plugincallback

import "time"

const (
	// DefaultBudget is the per-run callback budget per window (spec §3.4).
	DefaultBudget = 100
	// DefaultWindow is the rolling window length for the per-run limiter.
	DefaultWindow = time.Hour
)
