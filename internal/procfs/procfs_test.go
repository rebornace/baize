package procfs_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/procfs"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
)

func TestBuildTimelineIncludesFollowOnRunsCapped(t *testing.T) {
	st := store.NewMemory()
	source, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "src", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, st, source.ID, "llm.tool_call", map[string]any{"name": "tool_src"})

	for i := 0; i < 6; i++ {
		time.Sleep(2 * time.Millisecond)
		r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "follow", ConversationID: "conv1"})
		if err != nil {
			t.Fatal(err)
		}
		mustAppend(t, st, r.ID, "llm.tool_call", map[string]any{"name": "tool_" + r.ID})
	}

	tl, err := procfs.BuildTimeline(st, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tl.ConversationID != "conv1" || tl.SourceRunID != source.ID {
		t.Fatalf("timeline meta: %+v", tl)
	}
	seen := map[string]bool{}
	for _, e := range tl.Entries {
		seen[e.RunID] = true
	}
	if len(seen) != procfs.MaxFollowOnRuns {
		t.Fatalf("want %d distinct runs in timeline, got %d (%v)", procfs.MaxFollowOnRuns, len(seen), seen)
	}
}

func TestBuildTimelineNoConversationSingleRun(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "solo"})
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, st, r.ID, "llm.tool_call", map[string]any{"name": "only_tool"})

	tl, err := procfs.BuildTimeline(st, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tl.ConversationID != "" {
		t.Fatalf("conversation=%q", tl.ConversationID)
	}
	seen := map[string]bool{}
	for _, e := range tl.Entries {
		seen[e.RunID] = true
	}
	if len(seen) != 1 || !seen[r.ID] {
		t.Fatalf("want single run %s, got %v", r.ID, seen)
	}
}

func TestBuildTimelineEmptyEventsError(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = procfs.BuildTimeline(st, r.ID)
	if !errors.Is(err, procfs.ErrNoEvents) {
		t.Fatalf("want ErrNoEvents, got %v", err)
	}
}

func TestBuildTimelineRunNotFound(t *testing.T) {
	st := store.NewMemory()
	_, err := procfs.BuildTimeline(st, "run_missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderDraftContainsToolsAndHITL(t *testing.T) {
	ts := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	tl := procfs.Timeline{
		SourceRunID: "run_abc123456",
		Entries: []procfs.TimelineEntry{
			{RunID: "run_abc123456", Type: "tool.result", Timestamp: ts, Summary: "list_x", ToolName: "list_x"},
			{RunID: "run_abc123456", Type: "hitl.rejected", Timestamp: ts.Add(time.Second), Summary: "hitl rejected"},
		},
	}
	d, err := procfs.RenderDraft(tl, "run_abc123456")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := skill.BuildSKILLMD(d)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "list_x") || !strings.Contains(s, "hitl") {
		t.Fatalf("draft=%s", s)
	}
	if !strings.Contains(s, "人审") && !strings.Contains(strings.ToLower(s), "review") {
		t.Fatal("want human-review warning")
	}
}

func TestSuggestedSkillIDValid(t *testing.T) {
	id := procfs.SuggestedSkillID("run_deadbeefcafe")
	if _, err := skill.BuildSKILLMD(skill.ProposeDraft{Name: id, Description: "d", Body: "b"}); err != nil {
		t.Fatal(err)
	}
}

func TestEntryFromEventHITLIncludesDecision(t *testing.T) {
	e := procfs.EntryFromEventForTest("run_x", store.Event{
		Type: "hitl.resumed",
		Data: map[string]any{"decision": "approve", "comment": "ok go"},
	})
	if !strings.Contains(e.Summary, "approve") {
		t.Fatalf("summary=%q", e.Summary)
	}
	if !strings.Contains(e.Summary, "ok go") {
		t.Fatalf("want comment in summary=%q", e.Summary)
	}
}

func TestEntryFromEventHumanOpened(t *testing.T) {
	e := procfs.EntryFromEventForTest("run_x", store.Event{
		Type: procfs.EventHumanSkillDraftOpened,
		Data: map[string]any{"source": "eval"},
	})
	if e.Type != procfs.EventHumanSkillDraftOpened {
		t.Fatalf("type=%q", e.Type)
	}
	if !strings.Contains(e.Summary, "eval") {
		t.Fatalf("summary=%q", e.Summary)
	}
}

func mustAppend(t *testing.T, st store.Store, runID string, typ string, data map[string]any) {
	t.Helper()
	if err := st.AppendEvent(runID, store.Event{
		Type:      typ,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}); err != nil {
		t.Fatal(err)
	}
}
