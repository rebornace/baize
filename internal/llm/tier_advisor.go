package llm

import "context"

// TierAdvisor optionally arbitrates light vs power (DP-4) when the
// deterministic classifier lands on the ambiguous standard tier. It is defined
// here rather than importing internal/decide so the dependency graph stays
// acyclic (decide already imports llm); bootstrap supplies an adapter over the
// decide layer.
//
// TierAdviceOutcome distinguishes the advisor's three results so the caller
// knows when to record a unified decide.degraded event:
type TierAdviceOutcome int

const (
	// TierAdviceSkipped means the point was never eligible (switches off or
	// turn shorter than the floor) — the classifier tier stands, no event.
	TierAdviceSkipped TierAdviceOutcome = iota
	// TierAdviceDegraded means it WAS consulted but the layer failed open
	// (error / degraded / unrecognized tier) — caller keeps standard and this
	// is a decision-layer degradation worth recording.
	TierAdviceDegraded
	// TierAdviceOK means a usable tier was returned in Tier.
	TierAdviceOK
)

// TierAdvice is the structured DP-4 result. Tier is set when Outcome is
// TierAdviceOK (store.AutoTierLight / AutoTierPower).
type TierAdvice struct {
	Outcome TierAdviceOutcome
	Tier    string
}

// AdviseTier returns the chosen tier (store.AutoTierLight / AutoTierPower) with
// Outcome=TierAdviceOK. When the point is ineligible it returns TierAdviceSkipped;
// when it was consulted but the layer failed open it returns TierAdviceDegraded.
// In both non-OK cases the caller keeps the original classifier result.
type TierAdvisor interface {
	AdviseTier(ctx context.Context, text string) TierAdvice
}

// ResolveOption configures ResolveModel. The functional-options shape keeps
// the existing ResolveModel call sites (and their tests) unchanged while the
// advisor is wired only in production.
type ResolveOption func(*resolveConfig)

type resolveConfig struct {
	advisor TierAdvisor
	ctx     context.Context
}

// WithContext supplies the request context used when consulting the advisor.
// Production passes the run-creation request context; when unset the advisor
// call falls back to context.Background().
func WithContext(ctx context.Context) ResolveOption {
	return func(c *resolveConfig) { c.ctx = ctx }
}

// WithTierAdvisor enables the DP-4 fallback. The advisor implementation owns
// all gating (enabled flags + turn-length floor) so the trigger policy stays
// hot-reloadable with the runtime knobs.
func WithTierAdvisor(advisor TierAdvisor) ResolveOption {
	return func(c *resolveConfig) {
		c.advisor = advisor
	}
}
