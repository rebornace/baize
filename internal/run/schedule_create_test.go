package run

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/schedule"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func scheduleTestNow() time.Time {
	return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
}

func scheduleCreateArgs() map[string]any {
	return map[string]any{
		"kind":        "once",
		"instruction": "喝水",
		"timezone":    "UTC",
		"fire_at":     scheduleTestNow().Add(time.Hour).Format(time.RFC3339),
	}
}

type runScheduleMeta struct {
	store conversation.MetaStore
}

func (a runScheduleMeta) GetMeta(id string) (string, error) {
	m, err := a.store.GetMeta(id)
	if err != nil {
		return "", err
	}
	return m.OwnerID, nil
}

func registerScheduleCreate(reg *tool.Registry, sched schedule.Store, meta conversation.MetaStore) {
	nowFn := func() time.Time { return scheduleTestNow() }
	tm := schedule.CreateToolMeta(sched, runScheduleMeta{store: meta}, nowFn)
	reg.RegisterSpecApproved(tm.Spec, tm.Invoker, true)
	_ = reg.RegisterArgsPrecheck(schedule.CreateName, schedule.ValidateCreateArgs)
}

type scheduleScriptLLM struct{ calls int }

func (s *scheduleScriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	for _, m := range messages {
		if m.Role == llm.RoleTool {
			return llm.Message{Role: llm.RoleAssistant, Content: "已设置"}, nil
		}
	}
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
		{ID: "c1", Name: schedule.CreateName, Arguments: scheduleCreateArgs()},
	}}, nil
}

func (s *scheduleScriptLLM) SupportsVision() bool { return false }

func setupScheduleRun(t *testing.T) (*store.Memory, schedule.Store, *conversation.MemoryStore, *tool.Registry) {
	t.Helper()
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	sched := schedule.NewMemoryStore()
	meta := conversation.NewMemoryStore()
	if err := meta.EnsureMeta(conversation.Meta{
		ID: "conv1", OwnerID: "alice", Source: "ui", UpdatedAt: scheduleTestNow(),
	}); err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	registerScheduleCreate(reg, sched, meta)
	return st, sched, meta, reg
}

func TestScheduleCreateRejectedInsertsNothing(t *testing.T) {
	st, sched, _, reg := setupScheduleRun(t)
	ag := agent.Def{ID: "a", System: "sys"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "定时", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	gate := NewGate()
	eng := &Engine{
		Store: st, LLM: &scheduleScriptLLM{}, Tools: reg, Gate: gate,
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	errCh := make(chan error, 1)
	go func() { errCh <- eng.Execute(ctx, r.ID, ag, r.Input) }()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := resumeAfterWait(t, eng, errCh, r.ID, Decision{Approve: false}); err != nil && !errors.Is(err, ErrHITLRejected) {
		t.Fatalf("ContinueFromHITL: %v", err)
	}
	items, _ := sched.List("alice")
	if len(items) != 0 {
		t.Fatalf("want empty store, got %d", len(items))
	}
}

func TestScheduleCreateApprovedInsertsInstruction(t *testing.T) {
	st, sched, _, reg := setupScheduleRun(t)
	ag := agent.Def{ID: "a", System: "sys"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "定时", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	gate := NewGate()
	eng := &Engine{
		Store: st, LLM: &scheduleScriptLLM{}, Tools: reg, Gate: gate,
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	errCh := make(chan error, 1)
	go func() { errCh <- eng.Execute(ctx, r.ID, ag, r.Input) }()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := resumeAfterWait(t, eng, errCh, r.ID, Decision{Approve: true}); err != nil && !errors.Is(err, ErrHITLRejected) {
		t.Fatalf("ContinueFromHITL: %v", err)
	}
	items, _ := sched.List("alice")
	if len(items) != 1 {
		t.Fatalf("len=%d want 1", len(items))
	}
	if items[0].Instruction != "喝水" || items[0].Status != schedule.StatusActive {
		t.Fatalf("item=%+v", items[0])
	}
}

func TestScheduleCreateActiveCapAfterApprove(t *testing.T) {
	st, sched, _, reg := setupScheduleRun(t)
	now := scheduleTestNow()
	for i := 0; i < schedule.MaxActivePerOwner; i++ {
		if _, err := sched.Insert(schedule.Item{
			OwnerID: "alice", ConversationID: "conv1", Instruction: "existing",
			Kind: "daily", TimeOfDay: "09:00", Timezone: "UTC",
			Status: schedule.StatusActive, NextRunAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	ag := agent.Def{ID: "a", System: "sys"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "定时", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	gate := NewGate()
	eng := &Engine{
		Store: st, LLM: &scheduleScriptLLM{}, Tools: reg, Gate: gate,
		Now: func() time.Time { return now },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	errCh := make(chan error, 1)
	go func() { errCh <- eng.Execute(ctx, r.ID, ag, r.Input) }()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := resumeAfterWait(t, eng, errCh, r.ID, Decision{Approve: true}); err != nil && !errors.Is(err, ErrHITLRejected) {
		t.Fatalf("ContinueFromHITL: %v", err)
	}
	items, _ := sched.List("alice")
	if len(items) != schedule.MaxActivePerOwner {
		t.Fatalf("count=%d want %d", len(items), schedule.MaxActivePerOwner)
	}
	evs, _ := st.ListEvents(r.ID)
	var sawCapErr bool
	for _, ev := range evs {
		if ev.Type != EventToolResult || ev.Data["is_error"] != true {
			continue
		}
		content, _ := ev.Data["content"].(map[string]any)
		errStr, _ := content["error"].(string)
		if errStr != "" && errStr == schedule.ErrActiveCap.Error() {
			sawCapErr = true
		}
	}
	if !sawCapErr {
		t.Fatalf("expected active cap tool error in events")
	}
}

func TestScheduleCreatePastFireAtNoHITL(t *testing.T) {
	st, sched, _, reg := setupScheduleRun(t)
	now := scheduleTestNow()
	pastArgs := map[string]any{
		"kind": "once", "instruction": "喝水", "timezone": "UTC",
		"fire_at": now.Add(-time.Hour).Format(time.RFC3339),
	}
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		for _, m := range msgs {
			if m.Role == llm.RoleTool {
				return llm.Message{Role: llm.RoleAssistant, Content: "重试"}
			}
		}
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: schedule.CreateName, Arguments: pastArgs},
		}}
	}}
	ag := agent.Def{ID: "a", System: "sys"}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "定时", ConversationID: "conv1"})
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: reg, Gate: NewGate(), MaxSteps: 4,
		Now: func() time.Time { return now },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	if err := eng.Execute(ctx, r.ID, ag, r.Input); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status == store.StatusWaitingHuman {
		t.Fatal("must not enter waiting_human for invalid args")
	}
	items, _ := sched.List("alice")
	if len(items) != 0 {
		t.Fatalf("store must stay empty, got %d", len(items))
	}
}

func TestScheduleCreateCancelledBeforeApproveInsertsNothing(t *testing.T) {
	st, sched, meta, _ := setupScheduleRun(t)
	var invokes atomic.Int32
	nowFn := func() time.Time { return scheduleTestNow() }
	tm := schedule.CreateToolMeta(sched, runScheduleMeta{store: meta}, nowFn)
	inner := tm.Invoker
	reg := tool.NewRegistry()
	reg.RegisterSpecApproved(tm.Spec, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		invokes.Add(1)
		return inner(ctx, args)
	}, true)
	_ = reg.RegisterArgsPrecheck(schedule.CreateName, schedule.ValidateCreateArgs)

	ag := agent.Def{ID: "a", System: "sys"}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "定时", ConversationID: "conv1"})
	eng := &Engine{
		Store: st, LLM: &scheduleScriptLLM{}, Tools: reg, Gate: NewGate(),
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	done := make(chan error, 1)
	go func() { done <- eng.Execute(ctx, r.ID, ag, r.Input) }()
	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := eng.Cancel(r.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	if invokes.Load() != 0 {
		t.Fatalf("invoke count=%d want 0", invokes.Load())
	}
	items, _ := sched.List("alice")
	if len(items) != 0 {
		t.Fatalf("want empty store, got %d", len(items))
	}
}
