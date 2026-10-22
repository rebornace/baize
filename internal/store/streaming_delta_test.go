package store_test

import (
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestMemoryUpsertStreamingDeltaReplacesSameTurn(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	idx0, err := st.UpsertStreamingDelta(r.ID, store.Event{
		Type: "llm.content.delta",
		Data: map[string]any{"turn": 0, "text": "你"},
	})
	if err != nil || idx0 != 0 {
		t.Fatalf("first idx=%d err=%v", idx0, err)
	}
	idx1, err := st.UpsertStreamingDelta(r.ID, store.Event{
		Type: "llm.content.delta",
		Data: map[string]any{"turn": 0, "text": "你好世界"},
	})
	if err != nil || idx1 != 0 {
		t.Fatalf("second idx=%d err=%v want 0", idx1, err)
	}
	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("len=%d want 1 (upsert not append)", len(evs))
	}
	if evs[0].Data["text"] != "你好世界" {
		t.Fatalf("text=%v", evs[0].Data["text"])
	}
}

func TestMemoryUpsertStreamingDeltaKeepsThinkAndContentSeparate(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.UpsertStreamingDelta(r.ID, store.Event{
		Type: "llm.thinking.delta", Data: map[string]any{"turn": 0, "text": "想"},
	})
	_, _ = st.UpsertStreamingDelta(r.ID, store.Event{
		Type: "llm.content.delta", Data: map[string]any{"turn": 0, "text": "答"},
	})
	_, _ = st.UpsertStreamingDelta(r.ID, store.Event{
		Type: "llm.thinking.delta", Data: map[string]any{"turn": 0, "text": "想完"},
	})
	evs, _ := st.ListEvents(r.ID)
	if len(evs) != 2 {
		t.Fatalf("len=%d want 2", len(evs))
	}
	if evs[0].Type != "llm.thinking.delta" || evs[0].Data["text"] != "想完" {
		t.Fatalf("think=%v", evs[0])
	}
	if evs[1].Type != "llm.content.delta" || evs[1].Data["text"] != "答" {
		t.Fatalf("content=%v", evs[1])
	}
}

func TestSQLUpsertStreamingDeltaReplacesSameTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delta.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertStreamingDelta(r.ID, store.Event{
		Type: "llm.content.delta", Data: map[string]any{"turn": 1, "text": "a"},
	}); err != nil {
		t.Fatal(err)
	}
	idx, err := st.UpsertStreamingDelta(r.ID, store.Event{
		Type: "llm.content.delta", Data: map[string]any{"turn": 1, "text": "abc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || idx != 0 {
		t.Fatalf("len=%d idx=%d", len(evs), idx)
	}
	if evs[0].Data["text"] != "abc" {
		t.Fatalf("text=%v", evs[0].Data["text"])
	}
}
