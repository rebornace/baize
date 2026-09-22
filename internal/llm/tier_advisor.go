package llm

import "context"

// TierAdvisor optionally arbitrates light vs power (DP-4) when the
// deterministic classifier lands on the ambiguous standard tier. It is defined
// here rather than importing internal/decide so the dependency graph stays
// acyclic (decide already imports llm); bootstrap supplies an adapter over the
// decide layer.
//
// AdviseTier returns the chosen tier (store.AutoTierLight / AutoTierPower) and
// ok=true. Any abstention, error, or unrecognized tier returns ok=false, in
// which case the caller keeps the original classifier result (standard).
type TierAdvisor interface {
	AdviseTier(ctx context.Context, text string) (tier string, ok bool)
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
