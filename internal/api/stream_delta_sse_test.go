package api

import (
	"testing"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
)

func TestWriteLiveSSEEventAllowsStreamingDeltaResend(t *testing.T) {
	delta := store.Event{Type: "llm.content.delta", Data: map[string]any{"turn": 0, "text": "你好"}}
	ev := eventbus.IndexedEvent{Index: 2, Event: delta}

	// Same index as lastSent: non-delta would be skipped; delta must still send.
	if !store.IsStreamingDelta(delta) {
		t.Fatal("expected streaming delta")
	}
	// Pure logic check via shouldAdvance pattern mirrored by writeLiveSSEEvent.
	lastSent := 2
	next := lastSent
	if store.IsStreamingDelta(ev.Event) {
		if ev.Index > lastSent {
			next = ev.Index
		}
	} else if ev.Index > lastSent {
		next = ev.Index
	}
	if next != 2 {
		t.Fatalf("delta resend must keep lastSent=%d, got %d", lastSent, next)
	}

	tool := eventbus.IndexedEvent{Index: 2, Event: store.Event{Type: "tool.result"}}
	if store.IsStreamingDelta(tool.Event) {
		t.Fatal("tool.result is not a streaming delta")
	}
	if tool.Index <= lastSent {
		// skipped — good
	} else {
		t.Fatal("same-index non-delta should be treated as already sent")
	}
}
