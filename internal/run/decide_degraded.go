package run

import (
	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/store"
)

// emitDecideDegraded appends one unified decide.degraded event for a decision
// point that failed open. It is called only when a verdict is degraded or the
// raw implementation errored (err != nil on a point the production Chain would
// normally absorb). `kind` identifies the decision point; extra optional fields
// are merged into the event data. Emitting this never changes the point's
// fail-open behavior — it records only.
func (e *Engine) emitDecideDegraded(runID, kind string, extra map[string]any) {
	if e == nil || e.Store == nil || runID == "" || kind == "" {
		return
	}
	data := map[string]any{"kind": kind}
	for k, v := range extra {
		data[k] = v
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventDecideDegraded,
		Data: data,
	})
}

// degradedFromAsk reports whether a decision Answer/error pair represents a
// fail-open condition worth recording: either the Chain fell back to the
// hard-coded OnFail (Degraded) or a raw implementation returned an error
// (production *decide.Chain never errors; only fakes do).
func degradedFromAsk(ans decide.Answer, err error) bool {
	return err != nil || ans.Degraded
}
