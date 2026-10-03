package run

import "github.com/rebornace/baize/internal/runtimecfg"

// KnobReader provides the hot-reloadable engine knobs snapshot.
// *runtimecfg.Holder satisfies this interface; nil means "use YAML/struct
// defaults" (legacy behavior, zero changes to existing tests).
type KnobReader interface {
	Knobs() runtimecfg.Knobs
}

func (e *Engine) effectiveMaxSteps() int {
	if e.Settings != nil {
		if n := e.Settings.Knobs().MaxSteps; n > 0 {
			return n
		}
	}
	if e.MaxSteps > 0 {
		return e.MaxSteps
	}
	return 16
}

func (e *Engine) effectiveMaxMessages() int {
	if e.Settings != nil {
		if n := e.Settings.Knobs().MaxMessages; n > 0 {
			return n
		}
	}
	if e.MaxMessages > 0 {
		return e.MaxMessages
	}
	return 40
}
