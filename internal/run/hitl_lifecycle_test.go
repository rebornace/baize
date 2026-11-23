package run

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func newHITLTestEngine(t *testing.T, now time.Time) (*Engine, store.Store, *conversation.MemoryStore, *store.Run) {
	t.Helper()
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a"})
	ms := conversation.NewMemoryStore()
	if err := ms.EnsureMeta(conversation.Meta{ID: "conv", OwnerID: "u", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Store:           st,
		Messages:        ms,
		HITLApprovalTTL: 24 * time.Hour,
		Now:             func() time.Time { return now },
	}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "in", ConversationID: "conv"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ms.Append("conv", conversation.Message{
		Role: conversation.RoleUser, Content: "in", RunID: r.ID,
	}); err != nil {
		t.Fatal(err)
	}
	return eng, st, ms, r
}

func markWaiting(t *testing.T, st store.Store, runID string, payload *store.HITLPayload) {
	t.Helper()
	if err := st.UpdateRun(runID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetHITL(runID, payload); err != nil {
		t.Fatal(err)
	}
}

func findEvent(evs []store.Event, typ string) *store.Event {
	for i := range evs {
		if evs[i].Type == typ {
			return &evs[i]
		}
	}
	return nil
}

func TestHITLExpirySettlesWaitingApproval(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, ms, r := newHITLTestEngine(t, now)
	markWaiting(t, st, r.ID, &store.HITLPayload{
		Prompt:       "approve write",
		ToolName:     "write",
		Kind:         store.HITLKindApproval,
		WaitingSince: now.Add(-25 * time.Hour),
	})

	expired, err := eng.ExpireHITL(r.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if !expired {
		t.Fatal("want expired")
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusCancelled || got.Error != HITLExpiredReason {
		t.Fatalf("status=%s error=%q", got.Status, got.Error)
	}
	if p, _ := st.GetHITL(r.ID); p != nil {
		t.Fatalf("hitl payload must be cleared: %+v", p)
	}
	evs, _ := st.ListEvents(r.ID)
	ev := findEvent(evs, EventHITLExpired)
	if ev == nil {
		t.Fatal("missing hitl.expired event")
	}
	if ev.Data["reason"] != "waiting_expired" {
		t.Fatalf("reason=%v", ev.Data["reason"])
	}
	if ev.Data["kind"] != store.HITLKindApproval {
		t.Fatalf("kind=%v", ev.Data["kind"])
	}
	if _, ok := ev.Data["prompt"]; ok {
		t.Fatal("event must not carry prompt")
	}
	if _, ok := ev.Data["arguments"]; ok {
		t.Fatal("event must not carry arguments")
	}
	if w, _ := ev.Data["waited_sec"].(int64); w != int64(25*time.Hour/time.Second) {
		t.Fatalf("waited_sec=%v", ev.Data["waited_sec"])
	}

	msgs := ms.List("conv")
	found := false
	for _, m := range msgs {
		if m.Role == conversation.RoleSystemNote && strings.Contains(m.Content, HITLExpiredNote) {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing expired system note: %+v", msgs)
	}
	if busy, _ := st.HasActiveRun("conv"); busy {
		t.Fatal("expired run must release the conversation")
	}
}

func TestHITLExpiryDisabledWhenTTLZero(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, _, r := newHITLTestEngine(t, now)
	eng.HITLApprovalTTL = 0
	markWaiting(t, st, r.ID, &store.HITLPayload{
		ToolName:     "write",
		WaitingSince: now.Add(-100 * 24 * time.Hour),
	})
	expired, err := eng.ExpireHITL(r.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if expired {
		t.Fatal("TTL 0 must disable expiry")
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%s want waiting_human", got.Status)
	}
}

func TestHITLExpirySkipsWaitingChild(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, _, r := newHITLTestEngine(t, now)
	markWaiting(t, st, r.ID, &store.HITLPayload{
		Kind:         store.HITLKindWaitingChild,
		ChildRunID:   "child",
		WaitingSince: now.Add(-100 * 24 * time.Hour),
	})
	expired, err := eng.ExpireHITL(r.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if expired {
		t.Fatal("waiting_child must not expire")
	}
	if got, _ := st.GetRun(r.ID); got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%s want waiting_human", got.Status)
	}
}

type seededEventsStore struct {
	store.Store
	seeded map[string][]store.Event
}

func (s *seededEventsStore) ListEvents(runID string) ([]store.Event, error) {
	base, err := s.Store.ListEvents(runID)
	if err != nil {
		return nil, err
	}
	out := append([]store.Event{}, s.seeded[runID]...)
	return append(out, base...), nil
}

func TestHITLWaitingSinceFallsBackToEventTimestamp(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	_, st, _, r := newHITLTestEngine(t, now)
	started := now.Add(-25 * time.Hour)
	if err := st.SetHITL(r.ID, &store.HITLPayload{ToolName: "write"}); err != nil {
		t.Fatal(err)
	}
	seeded := &seededEventsStore{
		Store: st,
		seeded: map[string][]store.Event{
			r.ID: {{Type: EventHITLWaiting, Timestamp: started}},
		},
	}
	if got := HITLWaitingSince(seeded, r); !got.Equal(started) {
		t.Fatalf("waiting since=%v want %v", got, started)
	}
}

func TestHITLWaitingSinceFallsBackToRunCreatedAt(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	_, st, _, r := newHITLTestEngine(t, now)
	if got := HITLWaitingSince(st, r); !got.Equal(r.CreatedAt) {
		t.Fatalf("waiting since=%v want created_at %v", got, r.CreatedAt)
	}
}

func TestHITLResumeAfterExpiryDoesNotInvokeTool(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, _, r := newHITLTestEngine(t, now)
	markWaiting(t, st, r.ID, &store.HITLPayload{
		Prompt:       "approve write",
		ToolName:     "write",
		Kind:         store.HITLKindApproval,
		Arguments:    map[string]any{"value": "x"},
		WaitingSince: now.Add(-25 * time.Hour),
	})
	err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true})
	if !errors.Is(err, ErrHITLExpired) {
		t.Fatalf("err=%v want ErrHITLExpired", err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusCancelled {
		t.Fatalf("status=%s want cancelled", got.Status)
	}
	evs, _ := st.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == "tool.result" || ev.Type == EventLLMToolCall {
			t.Fatalf("expired approval must not invoke the tool, saw %s", ev.Type)
		}
	}
}

func TestExpiredRunNotOverwrittenByWorkflowFailure(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, _, r := newHITLTestEngine(t, now)
	markWaiting(t, st, r.ID, &store.HITLPayload{
		ToolName:     "write",
		WaitingSince: now.Add(-25 * time.Hour),
	})
	if _, err := eng.ExpireHITL(r.ID, now); err != nil {
		t.Fatal(err)
	}
	_ = eng.finalizeFailedRun(r.ID, errors.New("context canceled"))
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusCancelled || got.Error != HITLExpiredReason {
		t.Fatalf("finalizeFailedRun overwrote expiry: status=%s error=%q", got.Status, got.Error)
	}
}

func TestHITLReconcileFailsInterruptedWorkflow(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, ms, r := newHITLTestEngine(t, now)
	markWaiting(t, st, r.ID, &store.HITLPayload{
		Prompt:       "workflow step b",
		ToolName:     "create_ticket",
		Kind:         store.HITLKindApproval,
		WaitingSince: now.Add(-1 * time.Hour),
	})
	if err := st.AppendEvent(r.ID, store.Event{
		Type: "workflow.started",
		Data: map[string]any{"skill": "wf", "steps": []string{"a", "b"}},
	}); err != nil {
		t.Fatal(err)
	}

	settled, err := eng.ReconcileHITL(now)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 1 {
		t.Fatalf("settled=%d want 1", settled)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusFailed || got.Error != HITLWorkflowRestartNote {
		t.Fatalf("status=%s error=%q", got.Status, got.Error)
	}
	if p, _ := st.GetHITL(r.ID); p != nil {
		t.Fatalf("hitl payload must be cleared: %+v", p)
	}
	evs, _ := st.ListEvents(r.ID)
	ev := findEvent(evs, EventHITLInterrupted)
	if ev == nil || ev.Data["reason"] != "workflow_restart" {
		t.Fatalf("missing hitl.interrupted: %+v", ev)
	}
	found := false
	for _, m := range ms.List("conv") {
		if m.Role == conversation.RoleSystemNote && strings.Contains(m.Content, HITLWorkflowRestartNote) {
			found = true
		}
	}
	if !found {
		t.Fatal("missing restart system note")
	}
}

func TestHITLReconcileKeepsResumableReActWait(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, _, r := newHITLTestEngine(t, now)
	markWaiting(t, st, r.ID, &store.HITLPayload{
		Prompt:       "approve write",
		ToolName:     "write",
		Kind:         store.HITLKindApproval,
		WaitingSince: now.Add(-1 * time.Hour),
	})
	settled, err := eng.ReconcileHITL(now)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 0 {
		t.Fatalf("settled=%d want 0", settled)
	}
	if got, _ := st.GetRun(r.ID); got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%s want waiting_human", got.Status)
	}
	if p, _ := st.GetHITL(r.ID); p == nil {
		t.Fatal("resumable payload must stay")
	}
}

func TestHITLReconcileSettlesMissingPayload(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, _, r := newHITLTestEngine(t, now)
	if err := st.UpdateRun(r.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	settled, err := eng.ReconcileHITL(now)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 1 {
		t.Fatalf("settled=%d want 1", settled)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusCancelled {
		t.Fatalf("status=%s want cancelled", got.Status)
	}
	evs, _ := st.ListEvents(r.ID)
	ev := findEvent(evs, EventHITLExpired)
	if ev == nil || ev.Data["reason"] != "waiting_payload_missing" {
		t.Fatalf("missing waiting_payload_missing event: %+v", ev)
	}
}

func TestHITLReconcileSkipsWaitingChild(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, _, r := newHITLTestEngine(t, now)
	markWaiting(t, st, r.ID, &store.HITLPayload{
		Kind:         store.HITLKindWaitingChild,
		ChildRunID:   "child",
		WaitingSince: now.Add(-100 * 24 * time.Hour),
	})
	if err := st.AppendEvent(r.ID, store.Event{
		Type: "workflow.started",
		Data: map[string]any{"skill": "wf", "steps": []string{"b"}},
	}); err != nil {
		t.Fatal(err)
	}
	settled, err := eng.ReconcileHITL(now)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 0 {
		t.Fatalf("waiting_child must be skipped, settled=%d", settled)
	}
	if got, _ := st.GetRun(r.ID); got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%s want waiting_human", got.Status)
	}
}

func TestHITLReconcileKeepsResumableWorkflow(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, _, r := newHITLTestEngine(t, now)
	markWaiting(t, st, r.ID, &store.HITLPayload{
		Prompt:       "workflow step b",
		ToolName:     "tb",
		Kind:         store.HITLKindApproval,
		WaitingSince: now.Add(-time.Hour),
		Workflow:     &store.WorkflowState{Skill: "demo", StepIndex: 1, StepID: "b"},
	})
	if err := st.AppendEvent(r.ID, store.Event{
		Type: "workflow.started",
		Data: map[string]any{"skill": "demo", "steps": []string{"a", "b"}},
	}); err != nil {
		t.Fatal(err)
	}
	settled, err := eng.ReconcileHITL(now)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 0 {
		t.Fatalf("settled=%d want 0", settled)
	}
	if got, _ := st.GetRun(r.ID); got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%s want waiting_human", got.Status)
	}
}

func TestHITLWorkflowExpiryClearsState(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	eng, st, _, r := newHITLTestEngine(t, now)
	markWaiting(t, st, r.ID, &store.HITLPayload{
		Prompt:       "workflow step b",
		ToolName:     "tb",
		Kind:         store.HITLKindApproval,
		WaitingSince: now.Add(-25 * time.Hour),
		Workflow:     &store.WorkflowState{Skill: "demo", StepIndex: 1, StepID: "b"},
	})
	expired, err := eng.ExpireHITL(r.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if !expired {
		t.Fatal("workflow wait must expire by TTL")
	}
	if got, _ := st.GetRun(r.ID); got.Status != store.StatusCancelled {
		t.Fatalf("status=%s want cancelled", got.Status)
	}
	if p, _ := st.GetHITL(r.ID); p != nil {
		t.Fatalf("workflow payload must be cleared: %+v", p)
	}
}
