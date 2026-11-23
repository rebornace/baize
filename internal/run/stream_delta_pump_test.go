package run

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestStreamDeltaPumpLatestWinsWithoutBlocking(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	pump := newStreamDeltaPump(st, r.ID, 0)
	for i := 1; i <= 200; i++ {
		pump.Content(strings.Repeat("x", i))
	}
	pump.Close()

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	var last string
	for _, ev := range evs {
		if ev.Type == EventLLMContentDelta {
			last, _ = ev.Data["text"].(string)
		}
	}
	if last != strings.Repeat("x", 200) {
		t.Fatalf("want final snapshot len=200, got len=%d", len(last))
	}
}

func TestStreamDeltaPumpFlushThinkAndContent(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	pump := newStreamDeltaPump(st, r.ID, 1)
	pump.Think("想")
	pump.Content("答")
	pump.Close()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		evs, _ := st.ListEvents(r.ID)
		var think, content string
		for _, ev := range evs {
			switch ev.Type {
			case EventLLMThinkingDelta:
				think, _ = ev.Data["text"].(string)
			case EventLLMContentDelta:
				content, _ = ev.Data["text"].(string)
			}
		}
		if think == "想" && content == "答" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("missing think/content deltas after Close")
}
