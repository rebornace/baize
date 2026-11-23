package store

import "strings"

// StreamingDeltaTypes are cumulative text snapshots emitted during LLM streaming.
var StreamingDeltaTypes = map[string]struct{}{
	"llm.content.delta":  {},
	"llm.thinking.delta": {},
}

// IsStreamingDelta reports whether ev is a coalesceable streaming text snapshot.
func IsStreamingDelta(ev Event) bool {
	_, ok := StreamingDeltaTypes[strings.TrimSpace(ev.Type)]
	return ok
}

// EventTurn reads the optional turn field from event data (default 0).
func EventTurn(data map[string]any) int {
	if data == nil {
		return 0
	}
	switch v := data["turn"].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// FindStreamingDeltaIndex returns the last index of the same type+turn, or -1.
func FindStreamingDeltaIndex(evs []Event, typ string, turn int) int {
	typ = strings.TrimSpace(typ)
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type != typ {
			continue
		}
		if EventTurn(evs[i].Data) == turn {
			return i
		}
	}
	return -1
}
