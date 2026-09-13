package runtimecfg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// KnobsPatch is a partial engine-knob update (nil fields = leave unchanged).
type KnobsPatch struct {
	MaxMessages                  *int     `json:"max_messages,omitempty"`
	MaxSteps                     *int     `json:"max_steps,omitempty"`
	ToolTimeoutSeconds           *int     `json:"tool_timeout_seconds,omitempty"`
	CompactionEnabled            *bool    `json:"compaction_enabled,omitempty"`
	CompactThreshold             *float64 `json:"compact_threshold,omitempty"`
	CompactReserveTokens         *int     `json:"compact_reserve_tokens,omitempty"`
	KeepRecent                   *int     `json:"compact_keep_recent,omitempty"`
	CompactSummaryTimeoutSeconds *int     `json:"compact_summary_timeout_seconds,omitempty"`
}

// OperatorInput is one add_operator entry {id, token}.
type OperatorInput struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// CredsPatch is a partial control-plane credential update.
type CredsPatch struct {
	OperatorToken   string          `json:"operator_token,omitempty"`
	AdminToken      string          `json:"admin_token,omitempty"`
	AddOperators    []OperatorInput `json:"add_operators,omitempty"`
	RemoveOperators []string        `json:"remove_operators,omitempty"`
	Reset           bool            `json:"reset,omitempty"`
}

// persisted is the on-disk KV shape (delta only; absent = baseline).
type persisted struct {
	Knobs knobsOverride `json:"knobs,omitempty"`
	Creds credsOverride `json:"creds,omitempty"`
}

// Exported error sentinels so the API layer can map them to HTTP status codes
// (ErrBadRange/ErrBadRequest -> 400, ErrConflict -> 409) via errors.Is.
var (
	ErrBadRange   = errors.New("value out of allowed range")
	ErrConflict   = errors.New("operator id already exists")
	ErrBadRequest = errors.New("invalid credential patch")
)

// HTTPStatus maps a runtimecfg error to an HTTP status code. Unknown errors 500.
func HTTPStatus(err error) int {
	switch {
	case errors.Is(err, ErrConflict):
		return 409
	case errors.Is(err, ErrBadRange), errors.Is(err, ErrBadRequest):
		return 400
	default:
		return 500
	}
}

// ValidateKnobs checks field-level ranges. Returns a descriptive error.
func (h *Holder) ValidateKnobs(p KnobsPatch) error {
	if p.MaxMessages != nil && (*p.MaxMessages < 1 || *p.MaxMessages > 500) {
		return fmt.Errorf("%w: max_messages must be 1-500", ErrBadRange)
	}
	if p.MaxSteps != nil && (*p.MaxSteps < 1 || *p.MaxSteps > 100) {
		return fmt.Errorf("%w: max_steps must be 1-100", ErrBadRange)
	}
	if p.ToolTimeoutSeconds != nil && (*p.ToolTimeoutSeconds < 1 || *p.ToolTimeoutSeconds > 600) {
		return fmt.Errorf("%w: tool_timeout_seconds must be 1-600", ErrBadRange)
	}
	if p.CompactThreshold != nil && (*p.CompactThreshold < 0.1 || *p.CompactThreshold > 0.95) {
		return fmt.Errorf("%w: compact_threshold must be 0.1-0.95", ErrBadRange)
	}
	if p.CompactReserveTokens != nil && (*p.CompactReserveTokens < 256 || *p.CompactReserveTokens > 100000) {
		return fmt.Errorf("%w: compact_reserve_tokens must be 256-100000", ErrBadRange)
	}
	if p.KeepRecent != nil && (*p.KeepRecent < 0 || *p.KeepRecent > 100) {
		return fmt.Errorf("%w: compact_keep_recent must be 0-100", ErrBadRange)
	}
	if p.CompactSummaryTimeoutSeconds != nil &&
		(*p.CompactSummaryTimeoutSeconds < 1 || *p.CompactSummaryTimeoutSeconds > 600) {
		return fmt.Errorf("%w: compact_summary_timeout_seconds must be 1-600", ErrBadRange)
	}
	return nil
}

// KnobsOverride returns a copy of the current knob delta (for GET effective/overridden).
func (h *Holder) KnobsOverride() knobsOverride {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ko
}

// ApplyKnobs validates, merges, persists, and atomically swaps the snapshot.
func (h *Holder) ApplyKnobs(ctx context.Context, st store.Store, p KnobsPatch) error {
	if err := h.ValidateKnobs(p); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	next := h.ko
	if p.MaxMessages != nil {
		next.MaxMessages = p.MaxMessages
	}
	if p.MaxSteps != nil {
		next.MaxSteps = p.MaxSteps
	}
	if p.ToolTimeoutSeconds != nil {
		next.ToolTimeoutSeconds = p.ToolTimeoutSeconds
	}
	if p.CompactionEnabled != nil {
		next.CompactionEnabled = p.CompactionEnabled
	}
	if p.CompactThreshold != nil {
		next.CompactThreshold = p.CompactThreshold
	}
	if p.CompactReserveTokens != nil {
		next.CompactReserveTokens = p.CompactReserveTokens
	}
	if p.KeepRecent != nil {
		next.CompactKeepRecent = p.KeepRecent
	}
	if p.CompactSummaryTimeoutSeconds != nil {
		next.CompactSummaryTimeoutSec = p.CompactSummaryTimeoutSeconds
	}
	if err := h.persistLocked(ctx, st, next, h.co); err != nil {
		return err
	}
	h.ko = next
	h.swapLocked()
	return nil
}

// ApplyCreds validates, merges, persists, and swaps. The change must not leave
// a previously-configured gate with no valid token (lockout guard).
func (h *Holder) ApplyCreds(ctx context.Context, st store.Store, p CredsPatch) error {
	if p.Reset && (p.OperatorToken != "" || p.AdminToken != "" ||
		len(p.AddOperators) > 0 || len(p.RemoveOperators) > 0) {
		return fmt.Errorf("%w: reset cannot combine with other fields", ErrBadRequest)
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	next := h.co
	if p.Reset {
		next = credsOverride{}
	} else {
		if p.OperatorToken != "" {
			next.OperatorToken = p.OperatorToken
		}
		if p.AdminToken != "" {
			next.AdminToken = p.AdminToken
		}
		// remove (runtime only). Rebuild the slice once: any id that is a
		// config-baseline operator is rejected; any id absent from the runtime
		// set is rejected. A fresh slice (not in-place truncation) avoids
		// aliasing the backing array.
		if len(p.RemoveOperators) > 0 {
			removing := map[string]bool{}
			for _, id := range p.RemoveOperators {
				if h.hasBaseOperator(id) {
					return fmt.Errorf("%w: operator %q is from config and cannot be removed", ErrBadRequest, id)
				}
				removing[id] = true
			}
			kept := make([]operatorEntry, 0, len(next.Operators))
			for _, e := range next.Operators {
				if removing[e.ID] {
					continue
				}
				kept = append(kept, e)
			}
			if len(kept) != len(next.Operators)-len(p.RemoveOperators) {
				return fmt.Errorf("%w: one or more operators to remove not found in runtime set", ErrBadRequest)
			}
			next.Operators = kept
		}
		// add (no duplicate against effective set = base + runtime)
		for _, in := range p.AddOperators {
			if in.ID == "" || in.Token == "" {
				return fmt.Errorf("%w: add_operators entries require id and token", ErrBadRequest)
			}
			if h.effectiveHasOperator(next, in.ID) {
				return fmt.Errorf("%w: %q", ErrConflict, in.ID)
			}
			next.Operators = append(next.Operators, operatorEntry{ID: in.ID, Token: in.Token})
		}
	}

	// Lockout guard: if baseline configured a gate, the result must still have
	// at least one valid credential slot.
	effective := mergeSnapshot(h.base, h.ko, next).Creds
	if credentialsConfigured(h.base.Creds) && !credentialsConfigured(effective) {
		return fmt.Errorf("%w: refusing to clear all credentials (would lock out the gate)", ErrBadRequest)
	}

	if err := h.persistLocked(ctx, st, h.ko, next); err != nil {
		return err
	}
	h.co = next
	h.swapLocked()
	return nil
}

func (h *Holder) hasBaseOperator(id string) bool {
	for _, op := range h.base.Creds.Operators {
		if op.ID == id {
			return true
		}
	}
	return false
}

func (h *Holder) effectiveHasOperator(next credsOverride, id string) bool {
	if h.hasBaseOperator(id) {
		return true
	}
	for _, e := range next.Operators {
		if e.ID == id {
			return true
		}
	}
	return false
}

func (h *Holder) swapLocked() {
	snap := mergeSnapshot(h.base, h.ko, h.co)
	h.cur.Store(&snap)
}

// persistLocked writes the merged delta to the KV. Caller holds h.mu.
func (h *Holder) persistLocked(ctx context.Context, st store.Store, ko knobsOverride, co credsOverride) error {
	if st == nil {
		return nil // tests / no-store: swap in-memory only
	}
	raw, err := json.Marshal(persisted{Knobs: ko, Creds: co})
	if err != nil {
		return err
	}
	return st.UpsertSetting(store.SettingKeyRuntimeSettings, raw)
}

// Load reads the KV delta and applies it on top of the baseline. Corrupt JSON
// logs a warning and keeps the baseline (never blocks startup).
func (h *Holder) Load(ctx context.Context, st store.Store) error {
	if st == nil {
		return nil
	}
	raw, ok, err := st.GetSetting(store.SettingKeyRuntimeSettings)
	if err != nil || !ok || len(raw) == 0 {
		return err
	}
	var p persisted
	if err := json.Unmarshal(raw, &p); err != nil {
		log.Printf("runtimecfg: ignoring corrupt runtime_settings KV: %v", err)
		return nil
	}
	h.mu.Lock()
	h.ko = p.Knobs
	h.co = p.Creds
	h.swapLocked()
	h.mu.Unlock()
	return nil
}

// StartRefresh periodically re-reads the KV so changes made on another replica
// propagate. It blocks until ctx is cancelled; run it in a goroutine. Failures
// keep the last good snapshot.
func (h *Holder) StartRefresh(ctx context.Context, st store.Store, interval time.Duration) {
	if h == nil || st == nil || interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := h.Load(ctx, st); err != nil {
				log.Printf("runtimecfg: refresh failed (keeping last snapshot): %v", err)
			}
		}
	}
}
