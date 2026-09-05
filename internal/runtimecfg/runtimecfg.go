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
	MaxMessages          int
	MaxSteps             int
	ToolTimeout          time.Duration
	CompactionEnabled    bool
	CompactThreshold     float64
	CompactReserveTokens int
	CompactKeepRecent    int
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
	MaxMessages          *int     `json:"max_messages,omitempty"`
	MaxSteps             *int     `json:"max_steps,omitempty"`
	ToolTimeoutSeconds   *int     `json:"tool_timeout_seconds,omitempty"`
	CompactionEnabled    *bool    `json:"compaction_enabled,omitempty"`
	CompactThreshold     *float64 `json:"compact_threshold,omitempty"`
	CompactReserveTokens *int     `json:"compact_reserve_tokens,omitempty"`
	CompactKeepRecent    *int     `json:"compact_keep_recent,omitempty"`
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
