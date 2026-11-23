package run

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

const unverifiedClaimOutput = "复制完成，新问卷 id 是 13。"

func countSystemNotes(msgs []conversation.Message, content string) int {
	n := 0
	for _, m := range msgs {
		if m.Role == conversation.RoleSystemNote && strings.Contains(m.Content, content) {
			n++
		}
	}
	return n
}

func TestUnverifiedWriteNoteOnNoToolClaim(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, ms, r := newHITLTestEngine(t, now)
	if err := st.UpdateRun(r.ID, store.StatusSucceeded, unverifiedClaimOutput, ""); err != nil {
		t.Fatal(err)
	}
	eng.recordTerminalMessage(r.ID)

	evs, _ := st.ListEvents(r.ID)
	ev := findEvent(evs, EventUnverifiedWrite)
	if ev == nil {
		t.Fatal("missing run.unverified_write event")
	}
	if ev.Data["reason"] != "no_tool_call" {
		t.Fatalf("reason=%v", ev.Data["reason"])
	}
	claims, _ := ev.Data["claims"].([]string)
	if len(claims) == 0 || !strings.Contains(claims[0], "复制完成") {
		t.Fatalf("claims=%v", ev.Data["claims"])
	}
	if _, ok := ev.Data["output"]; ok {
		t.Fatal("event must not carry the whole output")
	}

	msgs := ms.List("conv")
	if countSystemNotes(msgs, UnverifiedWriteNote) != 1 {
		t.Fatalf("want one unverified-write note, got %+v", msgs)
	}
	if msgs[len(msgs)-1].Role != conversation.RoleSystemNote {
		t.Fatalf("note must be last, got %+v", msgs[len(msgs)-1])
	}

	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded || got.Output != unverifiedClaimOutput {
		t.Fatalf("run changed: status=%s output=%q", got.Status, got.Output)
	}
}

func TestUnverifiedWriteSilentWhenToolCalled(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, ms, r := newHITLTestEngine(t, now)
	if err := st.AppendEvent(r.ID, store.Event{
		Type: EventLLMToolCall,
		Data: map[string]any{"id": "call_1", "name": "list_questionnaires"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(r.ID, store.StatusSucceeded, unverifiedClaimOutput, ""); err != nil {
		t.Fatal(err)
	}
	eng.recordTerminalMessage(r.ID)

	evs, _ := st.ListEvents(r.ID)
	if findEvent(evs, EventUnverifiedWrite) != nil {
		t.Fatal("a run with any tool call must not be flagged in v1")
	}
	if n := countSystemNotes(ms.List("conv"), UnverifiedWriteNote); n != 0 {
		t.Fatalf("unexpected unverified-write note count=%d", n)
	}
}

func TestUnverifiedWriteNoteIdempotent(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, ms, r := newHITLTestEngine(t, now)
	if err := st.UpdateRun(r.ID, store.StatusSucceeded, unverifiedClaimOutput, ""); err != nil {
		t.Fatal(err)
	}
	eng.recordTerminalMessage(r.ID)
	eng.recordTerminalMessage(r.ID)

	evs, _ := st.ListEvents(r.ID)
	n := 0
	for _, ev := range evs {
		if ev.Type == EventUnverifiedWrite {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("unverified-write events=%d want 1", n)
	}
	if got := countSystemNotes(ms.List("conv"), UnverifiedWriteNote); got != 1 {
		t.Fatalf("unverified-write notes=%d want 1", got)
	}
}

func TestUnverifiedWriteSkipsNonSucceeded(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, ms, r := newHITLTestEngine(t, now)
	if err := st.UpdateRun(r.ID, store.StatusFailed, unverifiedClaimOutput, "boom"); err != nil {
		t.Fatal(err)
	}
	eng.recordTerminalMessage(r.ID)

	evs, _ := st.ListEvents(r.ID)
	if findEvent(evs, EventUnverifiedWrite) != nil {
		t.Fatal("failed runs must not be flagged")
	}
	if n := countSystemNotes(ms.List("conv"), UnverifiedWriteNote); n != 0 {
		t.Fatalf("unexpected note count=%d", n)
	}
}

func TestUnverifiedWriteSkipsReadOnlyClaim(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, ms, r := newHITLTestEngine(t, now)
	if err := st.UpdateRun(r.ID, store.StatusSucceeded, "已查询到问卷 11。", ""); err != nil {
		t.Fatal(err)
	}
	eng.recordTerminalMessage(r.ID)

	evs, _ := st.ListEvents(r.ID)
	if findEvent(evs, EventUnverifiedWrite) != nil {
		t.Fatal("read-only claims must not be flagged")
	}
	if n := countSystemNotes(ms.List("conv"), UnverifiedWriteNote); n != 0 {
		t.Fatalf("unexpected note count=%d", n)
	}
}
