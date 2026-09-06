// Package runtimecfg holds hot-reloadable runtime settings (engine knobs and
// control-plane credentials) behind an atomic snapshot. Reads are lock-free;
// updates serialize on a mutex and atomically swap the snapshot pointer.
//
// A nil *Holder is safe to use and returns zero values; consumers treat that
// as "fall back to YAML/struct defaults".
package runtimecfg

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
)

// Knobs is the effective engine-tuning snapshot.
type Knobs struct {
	MaxMessages           int
	MaxSteps              int
	ToolTimeout           time.Duration
	CompactionEnabled     bool
	CompactThreshold      float64
	CompactReserveTokens  int
	CompactKeepRecent     int
	CompactSummaryTimeout time.Duration
}

// Credentials is the effective control-plane credential set.
type Credentials struct {
	OperatorToken string
	AdminToken    string
	Operators     []controlplane.Operator
}

// Snapshot is an immutable view of all hot-reloadable settings.
type Snapshot struct {
	Knobs Knobs
	Creds Credentials
}

// operatorEntry is a runtime-added named operator (id + plaintext token).
type operatorEntry struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// knobsOverride holds the persisted KV delta for engine knobs. Pointer fields
// are nil when "not overridden" (keep baseline); a non-nil pointer (even to 0
// for compaction off) is an explicit override.
type knobsOverride struct {
	MaxMessages              *int     `json:"max_messages,omitempty"`
	MaxSteps                 *int     `json:"max_steps,omitempty"`
	ToolTimeoutSeconds       *int     `json:"tool_timeout_seconds,omitempty"`
	CompactionEnabled        *bool    `json:"compaction_enabled,omitempty"`
	CompactThreshold         *float64 `json:"compact_threshold,omitempty"`
	CompactReserveTokens     *int     `json:"compact_reserve_tokens,omitempty"`
	CompactKeepRecent        *int     `json:"compact_keep_recent,omitempty"`
	CompactSummaryTimeoutSec *int     `json:"compact_summary_timeout_seconds,omitempty"`
}

// credsOverride holds the persisted KV delta for control-plane credentials.
// Empty OperatorToken/AdminToken means "not overridden" (use config baseline,
// break-glass). Operators are runtime-added (appended to baseline).
type credsOverride struct {
	OperatorToken string          `json:"operator_token,omitempty"`
	AdminToken    string          `json:"admin_token,omitempty"`
	Operators     []operatorEntry `json:"operators,omitempty"`
}

// Holder stores the config baseline plus the current atomic Snapshot. The
// baseline is never mutated at runtime; KV overrides (ko/co) are layered on
// top to produce the effective snapshot.
type Holder struct {
	base Snapshot

	mu  sync.Mutex
	cur atomic.Pointer[Snapshot]

	ko knobsOverride
	co credsOverride
}

// New builds a Holder seeded from the config baseline.
func New(base Snapshot) *Holder {
	h := &Holder{base: base}
	snap := mergeSnapshot(base, knobsOverride{}, credsOverride{})
	h.cur.Store(&snap)
	return h
}

// Snapshot returns the current effective snapshot. Safe on a nil receiver.
func (h *Holder) Snapshot() Snapshot {
	if h == nil {
		return Snapshot{}
	}
	if p := h.cur.Load(); p != nil {
		return *p
	}
	return h.base
}

// Knobs returns the effective engine knobs.
func (h *Holder) Knobs() Knobs { return h.Snapshot().Knobs }

// Credentials returns the effective control-plane credentials.
func (h *Holder) Credentials() Credentials { return h.Snapshot().Creds }

// mergeSnapshot layers override pointers/values on top of the config baseline.
// Numeric knob overrides that are nil keep the baseline; credential token
// overrides that are empty keep the baseline; runtime operators are appended
// to the baseline operators.
func mergeSnapshot(base Snapshot, ko knobsOverride, co credsOverride) Snapshot {
	s := base
	k := s.Knobs
	if ko.MaxMessages != nil {
		k.MaxMessages = *ko.MaxMessages
	}
	if ko.MaxSteps != nil {
		k.MaxSteps = *ko.MaxSteps
	}
	if ko.ToolTimeoutSeconds != nil {
		k.ToolTimeout = time.Duration(*ko.ToolTimeoutSeconds) * time.Second
	}
	if ko.CompactionEnabled != nil {
		k.CompactionEnabled = *ko.CompactionEnabled
	}
	if ko.CompactThreshold != nil {
		k.CompactThreshold = *ko.CompactThreshold
	}
	if ko.CompactReserveTokens != nil {
		k.CompactReserveTokens = *ko.CompactReserveTokens
	}
	if ko.CompactKeepRecent != nil {
		k.CompactKeepRecent = *ko.CompactKeepRecent
	}
	if ko.CompactSummaryTimeoutSec != nil {
		k.CompactSummaryTimeout = time.Duration(*ko.CompactSummaryTimeoutSec) * time.Second
	}
	s.Knobs = k

	c := s.Creds
	if co.OperatorToken != "" {
		c.OperatorToken = co.OperatorToken
	}
	if co.AdminToken != "" {
		c.AdminToken = co.AdminToken
	}
	ops := make([]controlplane.Operator, 0, len(base.Creds.Operators)+len(co.Operators))
	ops = append(ops, base.Creds.Operators...)
	for _, e := range co.Operators {
		ops = append(ops, controlplane.Operator{ID: e.ID, Token: e.Token})
	}
	c.Operators = ops
	s.Creds = c
	return s
}

// KnobsFieldFlags reports which knobs are overridden in the KV. Field tags
// are the wire keys consumed by the HTTP layer (task 6).
type KnobsFieldFlags struct {
	MaxMessages           bool `json:"max_messages"`
	MaxSteps              bool `json:"max_steps"`
	ToolTimeout           bool `json:"tool_timeout_seconds"`
	CompactionEnabled     bool `json:"compaction_enabled"`
	CompactThreshold      bool `json:"compact_threshold"`
	CompactReserveTokens  bool `json:"compact_reserve_tokens"`
	CompactKeepRecent     bool `json:"compact_keep_recent"`
	CompactSummaryTimeout bool `json:"compact_summary_timeout_seconds"`
}

// KnobsView is the GET /settings/runtime body: effective values + override flags.
type KnobsView struct {
	Effective  Knobs           `json:"effective"`
	Overridden KnobsFieldFlags `json:"overridden"`
}

// KnobsView returns the effective engine knobs alongside per-field override
// flags (true when the value comes from a KV override rather than the config
// baseline). Safe on a nil receiver.
func (h *Holder) KnobsView() KnobsView {
	if h == nil {
		return KnobsView{}
	}
	ko := h.KnobsOverride()
	return KnobsView{
		Effective: h.Knobs(),
		Overridden: KnobsFieldFlags{
			MaxMessages:           ko.MaxMessages != nil,
			MaxSteps:              ko.MaxSteps != nil,
			ToolTimeout:           ko.ToolTimeoutSeconds != nil,
			CompactionEnabled:     ko.CompactionEnabled != nil,
			CompactThreshold:      ko.CompactThreshold != nil,
			CompactReserveTokens:  ko.CompactReserveTokens != nil,
			CompactKeepRecent:     ko.CompactKeepRecent != nil,
			CompactSummaryTimeout: ko.CompactSummaryTimeoutSec != nil,
		},
	}
}

// OperatorView is a masked operator entry (id + source, never the token).
type OperatorView struct {
	ID     string `json:"id"`
	Source string `json:"source"` // "config" | "runtime"
}

// CredsView is the GET /settings/credentials body (no tokens, ever).
type CredsView struct {
	Source      string         `json:"source"` // "config" | "override"
	OperatorSet bool           `json:"operator_set"`
	AdminSet    bool           `json:"admin_set"`
	Operators   []OperatorView `json:"operators"`
}

// CredentialsView returns a token-free view of the effective credentials:
// which credential slots are set, whether any runtime override exists, and
// each operator's origin ("config" for baseline, "runtime" for KV-added).
// It never contains token material. Safe on a nil receiver.
func (h *Holder) CredentialsView() CredsView {
	if h == nil {
		return CredsView{Source: "config", Operators: []OperatorView{}}
	}
	c := h.Credentials()
	h.mu.Lock()
	hasOverride := h.co.OperatorToken != "" || h.co.AdminToken != "" || len(h.co.Operators) > 0
	runtimeOps := map[string]bool{}
	for _, e := range h.co.Operators {
		runtimeOps[e.ID] = true
	}
	h.mu.Unlock()

	ops := make([]OperatorView, 0, len(c.Operators))
	for _, op := range c.Operators {
		src := "config"
		if runtimeOps[op.ID] {
			src = "runtime"
		}
		ops = append(ops, OperatorView{ID: op.ID, Source: src})
	}
	source := "config"
	if hasOverride {
		source = "override"
	}
	return CredsView{
		Source:      source,
		OperatorSet: c.OperatorToken != "",
		AdminSet:    c.AdminToken != "",
		Operators:   ops,
	}
}

func credentialsConfigured(c Credentials) bool {
	if c.OperatorToken != "" || c.AdminToken != "" {
		return true
	}
	for _, op := range c.Operators {
		if op.Token != "" {
			return true
		}
	}
	return false
}
