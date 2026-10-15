// Package runtimecfg holds hot-reloadable runtime settings (engine knobs and
// control-plane credentials) behind an atomic snapshot. Reads are lock-free;
// updates serialize on a mutex and atomically swap the snapshot pointer.
//
// A nil *Holder is safe to use and returns zero values; consumers treat that
// as "fall back to YAML/struct defaults".
package runtimecfg

import (
	"strings"
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
	MemoryEnabled         bool // account memory inject + tools gate
	MemoryAutoExtract     bool // post-run auto extract (also requires MemoryEnabled)
	// Decide
	DecideEnabled       bool // master switch for the decision layer
	DecideMemoryEnabled bool // DP-1: pre-extract worth-it judgment
	// DecideProfileID selects the model profile used for decision calls (a
	// cheap model, decoupled from the main model). Empty = no dedicated
	// decision model (the pick-many implementation stays inert).
	DecideProfileID string
	// DecideSystemOneBaseURL, when non-empty, enables the System One HTTP
	// backend (recommended: local Ollama at http://127.0.0.1:11434 with model
	// tev1; also Laya / Clef / self-hosted). Empty disables it so the chain
	// falls through to the chat profile / rules.
	DecideSystemOneBaseURL string
	DecideSystemOneAPIKey  string
	// DecideSystemOneModel is the System One model id. Empty uses the client
	// default (tev1) when a base URL is set.
	DecideSystemOneModel string
	// DP-2a: tool-candidate narrowing. When on and the tool count exceeds the
	// threshold, the narrowed set replaces the full tool list sent to the main
	// model (there is no separate observe-only mode).
	DecideToolRoutingEnabled bool // narrow tool candidates before the main model
	DecideToolThreshold      int  // only narrow when tool count exceeds this
	DecideToolPreTopK        int  // hybrid Tool-RAG retrieve width (Top-K)
	// DP-2b: enum constraint on the main-model call. When true (and DP-2a is
	// on), the narrowed tools are sent with tool_choice=required so the model
	// must select one of them.
	DecideToolChoiceEnabled bool
	// DP-3 (redirected): prune bulky accumulated tool results inside a single
	// run's ReAct loop so they stop growing the per-turn prompt. Only tool
	// results estimated above the threshold are judged; at most MaxJudged are
	// judged per turn; failures keep the full result (fail open).
	DecideToolPruneEnabled   bool // consult the layer whether a tool result is worth keeping
	DecideToolPruneThreshold int  // only judge tool results estimated above this
	DecideToolPruneMaxJudged int  // max tool results judged per turn
	// DP-4: cheap fallback for Auto model routing. Only when the deterministic
	// classifier lands on the ambiguous middle (standard) tier AND the turn is
	// at least MinRunes long does the layer arbitrate light vs power; short or
	// clearly-signed turns keep the zero-cost heuristic.
	DecideRouteEnabled  bool // consult the layer for light/power on ambiguous long turns
	DecideRouteMinRunes int  // minimum turn length (runes) before consulting
	// ContextProjectionEnabled lets the model edit a conversation-level prompt
	// overlay (pins / drop-in-run stubs / rewrite notes / force compact). Default
	// off. When on, the four context_* tools arm only after the prompt estimate
	// exceeds the same Compactor budget as rolling summaries.
	ContextProjectionEnabled bool
	// ActiveGoalEnabled injects the conversation goal stack into each turn
	// (independent of context projection). Default on; KV can override.
	ActiveGoalEnabled bool
	// ActiveGoalMaxDepth caps push depth; ActiveGoalMaxTextRunes caps frame text.
	ActiveGoalMaxDepth     int
	ActiveGoalMaxTextRunes int
}

// Credentials is the effective control-plane credential set.
type Credentials struct {
	OperatorToken string
	AdminToken    string
	Operators     []controlplane.Operator
}

// Observability is the effective log / OTel snapshot (hot-reloadable).
type Observability struct {
	LogFormat        string // text | json
	OtelOTLPEndpoint string // empty = OTel off
	OtelServiceName  string // default baize when empty after merge
}

// Snapshot is an immutable view of all hot-reloadable settings.
type Snapshot struct {
	Knobs         Knobs
	Creds         Credentials
	PublicBaseURL string // advertised Runtime root (OAuth / plugin callbacks)
	Observability Observability
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
	MemoryEnabled            *bool    `json:"memory_enabled,omitempty"`
	MemoryAutoExtract        *bool    `json:"memory_auto_extract,omitempty"`
	DecideEnabled            *bool    `json:"decide_enabled,omitempty"`
	DecideMemoryEnabled      *bool    `json:"decide_memory_enabled,omitempty"`
	DecideProfileID          *string  `json:"decide_profile_id,omitempty"`
	DecideSystemOneBaseURL   *string  `json:"decide_systemone_base_url,omitempty"`
	DecideSystemOneAPIKey    *string  `json:"decide_systemone_api_key,omitempty"`
	DecideSystemOneModel     *string  `json:"decide_systemone_model,omitempty"`
	DecideToolRoutingEnabled *bool    `json:"decide_tool_routing_enabled,omitempty"`
	DecideToolThreshold      *int     `json:"decide_tool_threshold,omitempty"`
	DecideToolPreTopK        *int     `json:"decide_tool_pre_topk,omitempty"`
	DecideToolChoiceEnabled  *bool    `json:"decide_tool_choice_enabled,omitempty"`
	DecideToolPruneEnabled   *bool    `json:"decide_tool_prune_enabled,omitempty"`
	DecideToolPruneThreshold *int     `json:"decide_tool_prune_threshold,omitempty"`
	DecideToolPruneMaxJudged *int     `json:"decide_tool_prune_max_judged,omitempty"`
	DecideRouteEnabled       *bool    `json:"decide_route_enabled,omitempty"`
	DecideRouteMinRunes      *int     `json:"decide_route_min_runes,omitempty"`
	ContextProjectionEnabled *bool    `json:"context_projection_enabled,omitempty"`
	ActiveGoalEnabled        *bool    `json:"active_goal_enabled,omitempty"`
	ActiveGoalMaxDepth       *int     `json:"active_goal_max_depth,omitempty"`
	ActiveGoalMaxTextRunes   *int     `json:"active_goal_max_text_runes,omitempty"`
}

// credsOverride holds the persisted KV delta for control-plane credentials.
// Empty OperatorToken/AdminToken means "not overridden" (use config baseline,
// break-glass). Operators are runtime-added (appended to baseline).
type credsOverride struct {
	OperatorToken string          `json:"operator_token,omitempty"`
	AdminToken    string          `json:"admin_token,omitempty"`
	Operators     []operatorEntry `json:"operators,omitempty"`
}

// obsOverride holds the persisted KV delta for observability. Pointer fields
// are nil when "not overridden". A non-nil OtelOTLPEndpoint (including "") is
// an explicit override ("" = force OTel off even if YAML has an endpoint).
type obsOverride struct {
	LogFormat        *string `json:"log_format,omitempty"`
	OtelOTLPEndpoint *string `json:"otel_otlp_endpoint,omitempty"`
	OtelServiceName  *string `json:"otel_service_name,omitempty"`
}

// Holder stores the config baseline plus the current atomic Snapshot. The
// baseline is never mutated at runtime; KV overrides (ko/co/po/oo) are layered
// on top to produce the effective snapshot.
type Holder struct {
	base Snapshot

	mu  sync.Mutex
	cur atomic.Pointer[Snapshot]

	ko knobsOverride
	co credsOverride
	// po is nil when PublicBaseURL is not overridden (use YAML baseline).
	// Non-nil (including pointer to "") means an explicit runtime override.
	po *string
	oo obsOverride
}

// New builds a Holder seeded from the config baseline.
func New(base Snapshot) *Holder {
	h := &Holder{base: normalizeObsBaseline(base)}
	snap := mergeSnapshot(h.base, knobsOverride{}, credsOverride{}, nil, obsOverride{})
	h.cur.Store(&snap)
	return h
}

// normalizeObsBaseline fills observability defaults on the YAML baseline.
func normalizeObsBaseline(base Snapshot) Snapshot {
	o := base.Observability
	f := strings.ToLower(strings.TrimSpace(o.LogFormat))
	if f != "json" {
		f = "text"
	}
	o.LogFormat = f
	o.OtelOTLPEndpoint = strings.TrimSpace(o.OtelOTLPEndpoint)
	if n := strings.TrimSpace(o.OtelServiceName); n != "" {
		o.OtelServiceName = n
	} else {
		o.OtelServiceName = "baize"
	}
	base.Observability = o
	return base
}

// ReplaceBaseline swaps the YAML/config baseline and re-merges the current KV
// overrides on top. Used by SIGHUP / POST settings/reload. Safe on a nil receiver.
func (h *Holder) ReplaceBaseline(base Snapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.base = base
	h.swapLocked()
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

// PublicBaseURL returns the effective advertised Runtime root URL.
func (h *Holder) PublicBaseURL() string { return h.Snapshot().PublicBaseURL }

// PublicBaseURLOverridden reports whether PublicBaseURL comes from a KV override.
func (h *Holder) PublicBaseURLOverridden() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.po != nil
}

// Observability returns the effective log / OTel settings.
func (h *Holder) Observability() Observability { return h.Snapshot().Observability }

// ObservabilityOverridden reports which observability fields come from KV.
func (h *Holder) ObservabilityOverridden() (logFormat, otelEndpoint, otelService bool) {
	if h == nil {
		return false, false, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.oo.LogFormat != nil, h.oo.OtelOTLPEndpoint != nil, h.oo.OtelServiceName != nil
}

// mergeSnapshot layers override pointers/values on top of the config baseline.
// Numeric knob overrides that are nil keep the baseline; credential token
// overrides that are empty keep the baseline; runtime operators are appended
// to the baseline operators. A non-nil po overrides PublicBaseURL (even to "").
// A non-nil oo field overrides the corresponding Observability value.
func mergeSnapshot(base Snapshot, ko knobsOverride, co credsOverride, po *string, oo obsOverride) Snapshot {
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
	if ko.MemoryEnabled != nil {
		k.MemoryEnabled = *ko.MemoryEnabled
	}
	if ko.MemoryAutoExtract != nil {
		k.MemoryAutoExtract = *ko.MemoryAutoExtract
	}
	if ko.DecideEnabled != nil {
		k.DecideEnabled = *ko.DecideEnabled
	}
	if ko.DecideMemoryEnabled != nil {
		k.DecideMemoryEnabled = *ko.DecideMemoryEnabled
	}
	if ko.DecideProfileID != nil {
		k.DecideProfileID = *ko.DecideProfileID
	}
	if ko.DecideSystemOneBaseURL != nil {
		k.DecideSystemOneBaseURL = *ko.DecideSystemOneBaseURL
	}
	if ko.DecideSystemOneAPIKey != nil {
		k.DecideSystemOneAPIKey = *ko.DecideSystemOneAPIKey
	}
	if ko.DecideSystemOneModel != nil {
		k.DecideSystemOneModel = *ko.DecideSystemOneModel
	}
	if ko.DecideToolRoutingEnabled != nil {
		k.DecideToolRoutingEnabled = *ko.DecideToolRoutingEnabled
	}
	if ko.DecideToolThreshold != nil {
		k.DecideToolThreshold = *ko.DecideToolThreshold
	}
	if ko.DecideToolPreTopK != nil {
		k.DecideToolPreTopK = *ko.DecideToolPreTopK
	}
	if ko.DecideToolChoiceEnabled != nil {
		k.DecideToolChoiceEnabled = *ko.DecideToolChoiceEnabled
	}
	if ko.DecideToolPruneEnabled != nil {
		k.DecideToolPruneEnabled = *ko.DecideToolPruneEnabled
	}
	if ko.DecideToolPruneThreshold != nil {
		k.DecideToolPruneThreshold = *ko.DecideToolPruneThreshold
	}
	if ko.DecideToolPruneMaxJudged != nil {
		k.DecideToolPruneMaxJudged = *ko.DecideToolPruneMaxJudged
	}
	if ko.DecideRouteEnabled != nil {
		k.DecideRouteEnabled = *ko.DecideRouteEnabled
	}
	if ko.DecideRouteMinRunes != nil {
		k.DecideRouteMinRunes = *ko.DecideRouteMinRunes
	}
	if ko.ContextProjectionEnabled != nil {
		k.ContextProjectionEnabled = *ko.ContextProjectionEnabled
	}
	if ko.ActiveGoalEnabled != nil {
		k.ActiveGoalEnabled = *ko.ActiveGoalEnabled
	}
	if ko.ActiveGoalMaxDepth != nil {
		k.ActiveGoalMaxDepth = *ko.ActiveGoalMaxDepth
	}
	if ko.ActiveGoalMaxTextRunes != nil {
		k.ActiveGoalMaxTextRunes = *ko.ActiveGoalMaxTextRunes
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

	if po != nil {
		s.PublicBaseURL = *po
	}

	o := s.Observability
	if oo.LogFormat != nil {
		o.LogFormat = *oo.LogFormat
	}
	if oo.OtelOTLPEndpoint != nil {
		o.OtelOTLPEndpoint = *oo.OtelOTLPEndpoint
	}
	if oo.OtelServiceName != nil {
		n := strings.TrimSpace(*oo.OtelServiceName)
		if n == "" {
			n = "baize"
		}
		o.OtelServiceName = n
	}
	s.Observability = o
	return s
}

// KnobsFieldFlags reports which knobs are overridden in the KV. Field tags
// are the wire keys consumed by the HTTP layer (task 6).
type KnobsFieldFlags struct {
	MaxMessages            bool `json:"max_messages"`
	MaxSteps               bool `json:"max_steps"`
	ToolTimeout            bool `json:"tool_timeout_seconds"`
	CompactionEnabled      bool `json:"compaction_enabled"`
	CompactThreshold       bool `json:"compact_threshold"`
	CompactReserveTokens   bool `json:"compact_reserve_tokens"`
	CompactKeepRecent      bool `json:"compact_keep_recent"`
	CompactSummaryTimeout  bool `json:"compact_summary_timeout_seconds"`
	MemoryEnabled          bool `json:"memory_enabled"`
	MemoryAutoExtract      bool `json:"memory_auto_extract"`
	DecideEnabled          bool `json:"decide_enabled"`
	DecideMemoryEnabled    bool `json:"decide_memory_enabled"`
	DecideProfileID        bool `json:"decide_profile_id"`
	DecideSystemOneBase    bool `json:"decide_systemone_base_url"`
	DecideSystemOneKey     bool `json:"decide_systemone_api_key"`
	DecideSystemOneModel   bool `json:"decide_systemone_model"`
	DecideToolRouting      bool `json:"decide_tool_routing_enabled"`
	DecideToolThreshold    bool `json:"decide_tool_threshold"`
	DecideToolPreTopK      bool `json:"decide_tool_pre_topk"`
	DecideToolChoice       bool `json:"decide_tool_choice_enabled"`
	DecideToolPrune        bool `json:"decide_tool_prune_enabled"`
	DecideToolPruneThresh  bool `json:"decide_tool_prune_threshold"`
	DecideToolPruneMax     bool `json:"decide_tool_prune_max_judged"`
	DecideRoute            bool `json:"decide_route_enabled"`
	DecideRouteMinRunes    bool `json:"decide_route_min_runes"`
	ContextProjection      bool `json:"context_projection_enabled"`
	ActiveGoalEnabled      bool `json:"active_goal_enabled"`
	ActiveGoalMaxDepth     bool `json:"active_goal_max_depth"`
	ActiveGoalMaxTextRunes bool `json:"active_goal_max_text_runes"`
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
			MaxMessages:            ko.MaxMessages != nil,
			MaxSteps:               ko.MaxSteps != nil,
			ToolTimeout:            ko.ToolTimeoutSeconds != nil,
			CompactionEnabled:      ko.CompactionEnabled != nil,
			CompactThreshold:       ko.CompactThreshold != nil,
			CompactReserveTokens:   ko.CompactReserveTokens != nil,
			CompactKeepRecent:      ko.CompactKeepRecent != nil,
			CompactSummaryTimeout:  ko.CompactSummaryTimeoutSec != nil,
			MemoryEnabled:          ko.MemoryEnabled != nil,
			MemoryAutoExtract:      ko.MemoryAutoExtract != nil,
			DecideEnabled:          ko.DecideEnabled != nil,
			DecideMemoryEnabled:    ko.DecideMemoryEnabled != nil,
			DecideProfileID:        ko.DecideProfileID != nil,
			DecideSystemOneBase:    ko.DecideSystemOneBaseURL != nil,
			DecideSystemOneKey:     ko.DecideSystemOneAPIKey != nil,
			DecideSystemOneModel:   ko.DecideSystemOneModel != nil,
			DecideToolRouting:      ko.DecideToolRoutingEnabled != nil,
			DecideToolThreshold:    ko.DecideToolThreshold != nil,
			DecideToolPreTopK:      ko.DecideToolPreTopK != nil,
			DecideToolChoice:       ko.DecideToolChoiceEnabled != nil,
			DecideToolPrune:        ko.DecideToolPruneEnabled != nil,
			DecideToolPruneThresh:  ko.DecideToolPruneThreshold != nil,
			DecideToolPruneMax:     ko.DecideToolPruneMaxJudged != nil,
			DecideRoute:            ko.DecideRouteEnabled != nil,
			DecideRouteMinRunes:    ko.DecideRouteMinRunes != nil,
			ContextProjection:      ko.ContextProjectionEnabled != nil,
			ActiveGoalEnabled:      ko.ActiveGoalEnabled != nil,
			ActiveGoalMaxDepth:     ko.ActiveGoalMaxDepth != nil,
			ActiveGoalMaxTextRunes: ko.ActiveGoalMaxTextRunes != nil,
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
