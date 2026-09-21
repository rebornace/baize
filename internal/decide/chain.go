package decide

import "context"

// Chain is the single call entry point. It tries each implementation in
// order and returns the first success; if all abstain or fail it returns
// Question.OnFail with Degraded=true. Chain never returns an error: errors
// must be consumed by degradation, never propagate into the main flow.
type Chain struct {
	items []Ask
}

// NewChain builds a chain. Production order is typically Remote first, Rules
// last (Rules is the always-available deterministic backstop).
func NewChain(items ...Ask) *Chain {
	return &Chain{items: items}
}

// Enabled always reports true: a chain itself is usable. Whether a decision is
// actually consulted is gated externally by the hot decide knobs, so a chain
// can be wired unconditionally without activating the feature.
func (c *Chain) Enabled() bool { return true }

// Ask returns the first successful answer or the hard-coded OnFail verdict.
// It never returns an error by contract: every failure is consumed by
// degrading to OnFail, so callers cannot accidentally propagate a decision
// error into the main flow.
func (c *Chain) Ask(ctx context.Context, q Question) (Answer, error) {
	if c != nil {
		for _, it := range c.items {
			if it == nil || !it.Enabled() {
				continue
			}
			ans, err := it.Ask(ctx, q)
			if err != nil {
				// Both abstentions (ErrUnavailable) and real failures move on
				// to the next implementation.
				continue
			}
			if ans.Source == "" {
				ans.Source = "unknown"
			}
			return ans, nil
		}
	}
	return Answer{Verdict: q.OnFail, Source: SourceFallback, Degraded: true}, nil
}
