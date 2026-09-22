// Package decide is baize's pluggable, degradable decision layer. It moves
// high-frequency "ask a generative model to write an answer to get a single
// decision" calls onto a thin layer that returns only enum verdicts.
//
// Hard constraints (see docs/decisions/BAIZE-JEV-SPEC.md):
//   - It never depends on an external service: a remote model is just one
//     optional implementation; without it the system keeps working.
//   - It never exposes numeric probabilities (baize calls OpenAI-compatible
//     APIs and cannot read calibrated logits).
//   - The failure direction of each decision point is hard-coded by that
//     point (Question.OnFail), never flipped by configuration.
package decide

import (
	"context"
	"errors"
)

// Verdict is the only binary decision shape: an enum, never a probability.
type Verdict string

const (
	VerdictYes Verdict = "yes"
	VerdictNo  Verdict = "no"
)

// Source labels identify where an answer came from (observability only).
const (
	SourceRules    = "rules"
	SourceRemote   = "remote"
	SourceFallback = "fallback"
)

// Kind identifiers for the decision points.
const (
	// KindMemoryExtract asks whether a turn carries facts worth extracting
	// (DP-1). OnFail is VerdictYes: if the layer is down, extract anyway.
	KindMemoryExtract = "memory_worth_extracting"
	// KindToolCandidates asks which tools are worth keeping in the prompt for
	// a run (DP-2a). It is a pick-many question over Question.Options; chosen
	// names come back in Answer.Values. OnFail is VerdictYes (keep all tools)
	// so a layer outage preserves today's behavior.
	KindToolCandidates = "tool_candidates"
	// KindPruneToolResult asks whether one bulky tool result already produced
	// in this run is worth keeping verbatim in later turns (DP-3, redirected).
	// Binary verdict; the engine probes the result content (trimmed). OnFail
	// is VerdictYes: if the layer is down, keep the full result rather than
	// silently drop information.
	KindPruneToolResult = "tool_result_worth_keeping"
	// KindSystemTargets asks which backend systems (connectors) a turn needs
	// before tools within them are considered (two-level routing, multi-system
	// support). It is a pick-many question whose Options are connector IDs and
	// chosen IDs come back in Answer.Values. OnFail is VerdictYes (keep every
	// system) so a layer outage degrades to the flat-catalog behavior.
	KindSystemTargets = "system_targets"
	// KindRouteTier arbitrates light vs power (DP-4) when the deterministic
	// router lands on the ambiguous standard tier. It is a pick-ONE question
	// whose Options are [light, power]; the chosen tier comes back in
	// Answer.Value. The call site supplies no OnFail verdict: on any error it
	// explicitly keeps ClassifyTask's original standard tier.
	KindRouteTier = "route_tier"
)

// ErrUnavailable reports that an implementation currently abstains or is
// unavailable. The Chain consumes it and moves to the next implementation;
// it never propagates to callers.
var ErrUnavailable = errors.New("decide: unavailable")

// Question describes one judgment. OnFail is hard-coded by the call site
// (never configurable) because the failure direction is a safety property:
// memory extraction fails open to Yes (rather spend a call than lose a
// memory), etc.
type Question struct {
	// Kind identifies the decision point (one of the Kind* constants).
	Kind string
	// Context is the (already trimmed) context shown to the implementation.
	Context string
	// Options, when non-empty, makes this a pick-one-or-more question; chosen
	// items are returned in Answer.Values. Empty means a binary verdict.
	Options []string
	// Descriptions optionally annotates each Option with a short human-readable
	// explanation (e.g. connector id -> what that backend system is) shown to
	// the model. Nil implementations may ignore it.
	Descriptions map[string]string
	// OnFail is the verdict used when every implementation abstains/fails.
	OnFail Verdict
	// TraceID correlates the judgment with a run / conversation.
	TraceID string
}

// Answer returns an enum plus provenance, never a numeric probability.
type Answer struct {
	Verdict Verdict
	// Value holds the single chosen option for a pick-one question.
	Value string
	// Values holds the chosen options for a pick-many question.
	Values []string
	// Source is one of the Source* constants.
	Source string
	// Degraded reports that the answer fell back to Question.OnFail.
	Degraded bool
}

// Ask is the abstraction of a single decision implementation. An
// implementation must return ErrUnavailable (not a wrapped generic error)
// when it abstains so the Chain can degrade.
type Ask interface {
	Enabled() bool
	Ask(ctx context.Context, q Question) (Answer, error)
}
