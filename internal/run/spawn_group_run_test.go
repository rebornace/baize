package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workflow"
)

type scopedKnobs struct {
	on   bool
	wake bool
}

func (s scopedKnobs) Knobs() runtimecfg.Knobs {
	return runtimecfg.Knobs{
		ScopedSubrunEnabled: s.on, SubagentAsyncWakeEnabled: s.wake, MaxSteps: 16,
	}
}

func TestSpawnGroupRunRequiresFlag(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "p", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, Settings: scopedKnobs{on: false}}
	eng.SetConnectorGroups([]config.ConnectorGroup{{ID: "finance", Connectors: []string{"expense"}}})
	ctx := identity.WithToolCallID(identity.WithRunID(context.Background(), parent.ID), "tc1")
	out, isErr, err := eng.spawnGroupRun(ctx, map[string]any{"group_id": "finance", "goal": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !isErr || out["error"] == nil {
		t.Fatalf("want error content, got %+v", out)
	}
}

func TestSpawnGroupRunCreatesChildAndWaits(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "p", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	var dispatched []middleware.Job
	eng := &Engine{
		Store: st, Tools: reg, Settings: scopedKnobs{on: true},
		DispatchJob: func(job middleware.Job) { dispatched = append(dispatched, job) },
	}
	eng.SetConnectorGroups([]config.ConnectorGroup{{ID: "finance", Connectors: []string{"expense", "invoice"}}})
	ctx := identity.WithToolCallID(identity.WithRunID(context.Background(), parent.ID), "tc1")
	_, _, err = eng.spawnGroupRun(ctx, map[string]any{
		"group_id": "finance", "goal": "查报销", "mode": "readonly",
	})
	if !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("err=%v want ErrWaitingHuman", err)
	}
	got, _ := st.GetRun(parent.ID)
	if got.Status != store.StatusWaitingHuman {
		t.Fatalf("parent status=%s", got.Status)
	}
	hitl, _ := st.GetHITL(parent.ID)
	if hitl == nil || hitl.Kind != store.HITLKindWaitingChild || hitl.ChildRunID == "" {
		t.Fatalf("hitl=%+v", hitl)
	}
	kids, _ := st.ListChildRuns(parent.ID)
	if len(kids) != 1 || kids[0].Depth != 1 || kids[0].GroupID != "finance" {
		t.Fatalf("kids=%+v", kids)
	}
	if len(dispatched) != 1 || dispatched[0].RunID != kids[0].ID {
		t.Fatalf("dispatched=%+v", dispatched)
	}
}

func TestSpawnGroupRunRejectsNested(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	child, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: "p1", GroupID: "finance",
	})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, Settings: scopedKnobs{on: true}}
	eng.SetConnectorGroups([]config.ConnectorGroup{{ID: "finance", Connectors: []string{"expense"}}})
	ctx := identity.WithRunID(context.Background(), child.ID)
	out, isErr, err := eng.spawnGroupRun(ctx, map[string]any{"group_id": "finance", "goal": "x"})
	if err != nil || !isErr {
		t.Fatalf("want soft error, err=%v out=%v", err, out)
	}
	if !strings.Contains(out["error"].(string), "depth") {
		t.Fatalf("out=%v", out)
	}
}

func TestFilterSpecsForChild(t *testing.T) {
	eng := &Engine{}
	eng.SetConnectorGroups([]config.ConnectorGroup{{ID: "finance", Connectors: []string{"expense"}}})
	eng.setChildScope("r1", childScope{GroupID: "finance", Sources: []string{"expense"}, Mode: store.SpawnModeReadonly})
	specs := []llm.ToolSpec{
		{Name: "a", Source: "expense"},
		{Name: "b", Source: "mall"},
		{Name: SpawnGroupRunName, Source: ""},
		{Name: "activate_skill", Source: ""},
	}
	out := eng.filterSpecsForChild("r1", specs)
	if len(out) != 1 || out[0].Name != "a" {
		t.Fatalf("out=%v", toolNames(out))
	}
}

func TestSpawnRejectedInsideWorkflowStep(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "p"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, Settings: scopedKnobs{on: true}}
	eng.SetConnectorGroups([]config.ConnectorGroup{{ID: "finance", Connectors: []string{"expense"}}})
	ctx := identity.WithToolCallID(identity.WithRunID(context.Background(), parent.ID), "tc1")
	ctx = workflow.WithStepState(ctx, workflow.State{Skill: "demo", NextStep: 0, StepID: "a"})
	out, isErr, err := eng.spawnGroupRun(ctx, map[string]any{"group_id": "finance", "goal": "x"})
	if err != nil || !isErr {
		t.Fatalf("want soft error, err=%v out=%v", err, out)
	}
	if !strings.Contains(out["error"].(string), "workflow") {
		t.Fatalf("out=%v", out)
	}
	kids, _ := st.ListChildRuns(parent.ID)
	if len(kids) != 0 {
		t.Fatalf("must not create child, got %d", len(kids))
	}
}

func TestNotifyChildTerminalPublishesFacts(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "p"})
	child, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: parent.ID, GroupID: "finance",
	})
	_ = st.UpdateRun(child.ID, store.StatusSucceeded,
		"合计 1240\nRECEIPT_JSON: {\"summary\":\"合计 1240\",\"facts\":{\"total_tax\":\"1240.00\"}}", "")
	eng := &Engine{Store: st}
	eng.NotifyChildTerminal(child)
	evs, _ := st.ListEvents(parent.ID)
	var receipt map[string]any
	for _, ev := range evs {
		if ev.Type == EventRunReceipt {
			receipt = ev.Data
		}
	}
	if receipt == nil {
		t.Fatal("missing run.receipt")
	}
	if receipt["summary"] != "合计 1240" {
		t.Fatalf("summary=%v", receipt["summary"])
	}
	facts, _ := receipt["facts"].(map[string]any)
	if facts["total_tax"] != "1240.00" {
		t.Fatalf("facts=%+v", receipt["facts"])
	}
}

func TestSpawnAsyncReturnsStartedWithoutWaiting(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "p", ConversationID: "c1"})
	eng := &Engine{Store: st, Settings: scopedKnobs{on: true}, Tools: tool.NewRegistry()}
	eng.SetConnectorGroups([]config.ConnectorGroup{{ID: "finance", Connectors: []string{"expense"}}})
	ctx := identity.WithToolCallID(identity.WithRunID(context.Background(), parent.ID), "tc1")
	out, isErr, err := eng.spawnGroupRun(ctx, map[string]any{
		"group_id": "finance", "goal": "核对发票", "mode": "readonly", "async": true,
	})
	if err != nil || isErr {
		t.Fatalf("err=%v out=%v", err, out)
	}
	if out["status"] != "started" || out["async"] != true || out["child_run_id"] == "" {
		t.Fatalf("out=%+v", out)
	}
	got, _ := st.GetRun(parent.ID)
	if got.Status == store.StatusWaitingHuman {
		t.Fatal("async spawn must not wait")
	}
	if h, _ := st.GetHITL(parent.ID); h != nil {
		t.Fatalf("async spawn must not persist HITL: %+v", h)
	}
	evs, _ := st.ListEvents(parent.ID)
	var spawned map[string]any
	for _, ev := range evs {
		if ev.Type == EventRunSpawned {
			spawned = ev.Data
		}
	}
	if spawned == nil || spawned["async"] != true {
		t.Fatalf("run.spawned=%+v", spawned)
	}
}

func TestAsyncChildTerminalAddsNoteWithoutWakingParent(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	convs := conversation.NewMemoryStore()
	parent, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "p", ConversationID: "c1"})
	child, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: parent.ID, GroupID: "finance",
	})
	_ = st.AppendEvent(parent.ID, store.Event{
		Type: EventRunSpawned,
		Data: map[string]any{
			"child_run_id": child.ID, "group_id": "finance", "goal": "核对发票", "async": true,
		},
	})
	_ = st.UpdateRun(child.ID, store.StatusSucceeded,
		"合计 1240\nRECEIPT_JSON: {\"summary\":\"合计 1240\",\"facts\":{\"total_tax\":\"1240.00\"}}", "")
	var woke bool
	eng := &Engine{Store: st, Messages: convs, OnChildTerminal: func(string, GroupRunReceipt) { woke = true }}
	eng.NotifyChildTerminal(child)
	if woke {
		t.Fatal("async child must not wake the parent")
	}
	evs, _ := st.ListEvents(parent.ID)
	var receipt map[string]any
	for _, ev := range evs {
		if ev.Type == EventRunReceipt {
			receipt = ev.Data
		}
	}
	if receipt == nil || receipt["async"] != true || receipt["summary"] != "合计 1240" {
		t.Fatalf("receipt=%+v", receipt)
	}
	msgs := convs.List("c1")
	found := false
	for _, m := range msgs {
		if m.Role == conversation.RoleSystemNote && strings.Contains(m.Content, "子代理完成") &&
			strings.Contains(m.Content, "合计 1240") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing async child system note: %+v", msgs)
	}
}

func TestSyncChildTerminalWakesWaitingParent(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "p"})
	child, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: parent.ID, GroupID: "finance",
	})
	_ = st.UpdateRun(parent.ID, store.StatusWaitingHuman, "", "")
	_ = st.SetHITL(parent.ID, &store.HITLPayload{
		Kind: store.HITLKindWaitingChild, ChildRunID: child.ID, ToolName: SpawnGroupRunName,
	})
	_ = st.UpdateRun(child.ID, store.StatusSucceeded, "ok", "")
	var woke bool
	eng := &Engine{Store: st, OnChildTerminal: func(string, GroupRunReceipt) { woke = true }}
	eng.NotifyChildTerminal(child)
	if !woke {
		t.Fatal("sync child terminal must wake a waiting parent")
	}
}

func TestNotifyChildTerminalAutoWakesWhenEnabled(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	convs := conversation.NewMemoryStore()
	parent, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "p", ConversationID: "c1",
	})
	_ = st.UpdateRun(parent.ID, store.StatusSucceeded, "done", "")
	child, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: parent.ID, ConversationID: "c1",
	})
	_ = st.AppendEvent(parent.ID, store.Event{
		Type: EventRunSpawned,
		Data: map[string]any{
			"child_run_id": child.ID, "group_id": "tickets", "goal": "count", "async": true,
		},
	})
	_ = st.UpdateRun(child.ID, store.StatusSucceeded,
		"1 ticket\nRECEIPT_JSON: {\"summary\":\"1 ticket\",\"facts\":{\"total\":1}}", "")
	var jobs []middleware.Job
	eng := &Engine{
		Store: st, Messages: convs, Settings: scopedKnobs{on: true, wake: true},
		DispatchJob: func(j middleware.Job) { jobs = append(jobs, j) },
	}
	eng.NotifyChildTerminal(child)
	if len(jobs) != 1 {
		t.Fatalf("jobs=%d want 1", len(jobs))
	}
	msgs := convs.List("c1")
	found := false
	for _, m := range msgs {
		if m.Role == conversation.RoleSystemNote && strings.Contains(m.Content, "自动继续") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing auto-wake note: %+v", msgs)
	}
}

func TestAutoWakeStartsFollowUpWhenIdle(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	convs := conversation.NewMemoryStore()
	parent, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "p", ConversationID: "c1",
	})
	_ = st.UpdateRun(parent.ID, store.StatusSucceeded, "done", "")
	child, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: parent.ID, ConversationID: "c1",
	})
	_ = st.UpdateRun(child.ID, store.StatusSucceeded, "ok", "")
	var jobs []middleware.Job
	eng := &Engine{
		Store: st, Messages: convs, Settings: scopedKnobs{on: true, wake: true},
		DispatchJob: func(j middleware.Job) { jobs = append(jobs, j) },
	}
	eng.maybeAutoWakeAsyncChild(child, "count tickets", GroupRunReceipt{
		Status: "succeeded", Summary: "1 ticket", ChildRunID: child.ID,
		Facts: map[string]any{"total": 1},
	})
	if len(jobs) != 1 {
		t.Fatalf("jobs=%d want 1", len(jobs))
	}
	next, _ := st.GetRun(jobs[0].RunID)
	if next == nil || next.ConversationID != "c1" || !strings.Contains(next.Input, "子代理回执") {
		t.Fatalf("next run=%+v", next)
	}
	evs, _ := st.ListEvents(parent.ID)
	if findEvent(evs, EventRunAutoWake) == nil {
		t.Fatal("missing parent run.auto_wake")
	}
	msgs := convs.List("c1")
	found := false
	for _, m := range msgs {
		if m.Role == conversation.RoleSystemNote && strings.Contains(m.Content, "自动继续") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing auto-wake note: %+v", msgs)
	}
}

func TestAutoWakeSkipsBusyOrOff(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "p", ConversationID: "c1",
	})
	_ = st.UpdateRun(parent.ID, store.StatusSucceeded, "done", "")
	child, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: parent.ID, ConversationID: "c1",
	})
	// Another active run in the same conversation.
	_, _ = st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "user again", ConversationID: "c1"})
	var jobs []middleware.Job
	eng := &Engine{
		Store: st, Settings: scopedKnobs{on: true, wake: true},
		DispatchJob: func(j middleware.Job) { jobs = append(jobs, j) },
	}
	eng.maybeAutoWakeAsyncChild(child, "goal", GroupRunReceipt{Status: "succeeded", Summary: "ok"})
	if len(jobs) != 0 {
		t.Fatalf("busy conversation must not auto-wake: %+v", jobs)
	}

	eng.Settings = scopedKnobs{on: true, wake: false}
	eng.maybeAutoWakeAsyncChild(child, "goal", GroupRunReceipt{Status: "succeeded", Summary: "ok"})
	if len(jobs) != 0 {
		t.Fatalf("setting off must not auto-wake: %+v", jobs)
	}
}

func TestAutoWakeDoesNotChain(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "p", ConversationID: "c1",
	})
	_ = st.UpdateRun(parent.ID, store.StatusSucceeded, "done", "")
	_ = st.AppendEvent(parent.ID, store.Event{Type: EventRunAutoWake})
	child, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: parent.ID, ConversationID: "c1",
	})
	var jobs []middleware.Job
	eng := &Engine{
		Store: st, Settings: scopedKnobs{on: true, wake: true},
		DispatchJob: func(j middleware.Job) { jobs = append(jobs, j) },
	}
	eng.maybeAutoWakeAsyncChild(child, "goal", GroupRunReceipt{Status: "succeeded", Summary: "ok"})
	if len(jobs) != 0 {
		t.Fatalf("auto-wake chain must stop: %+v", jobs)
	}
}

func TestCancelChildPublishesCancelledReceipt(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "p"})
	child, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: parent.ID, GroupID: "finance",
	})
	_ = st.UpdateRun(child.ID, store.StatusRunning, "", "")
	eng := &Engine{Store: st}
	if err := eng.Cancel(child.ID); err != nil {
		t.Fatal(err)
	}
	evs, _ := st.ListEvents(parent.ID)
	var receipt map[string]any
	for _, ev := range evs {
		if ev.Type == EventRunReceipt && ev.Data["child_run_id"] == child.ID {
			receipt = ev.Data
		}
	}
	if receipt == nil || receipt["status"] != string(store.StatusCancelled) {
		t.Fatalf("receipt=%+v", receipt)
	}
	got, _ := st.GetRun(child.ID)
	if got.Status != store.StatusCancelled {
		t.Fatalf("child status=%s", got.Status)
	}
}

func TestScopedChildRunEndToEndReceipt(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "parent goal", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	var dispatched []middleware.Job
	eng := &Engine{
		Store: st, Tools: tool.NewRegistry(), Settings: scopedKnobs{on: true},
		DispatchJob: func(job middleware.Job) { dispatched = append(dispatched, job) },
		LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
			return llm.Message{Role: llm.RoleAssistant,
				Content: "合计 1240\nRECEIPT_JSON: {\"summary\":\"合计 1240\",\"facts\":{\"total_tax\":\"1240.00\"}}"}
		}},
	}
	eng.SetConnectorGroups([]config.ConnectorGroup{{ID: "finance", Connectors: []string{"expense"}}})
	ctx := identity.WithToolCallID(identity.WithRunID(context.Background(), parent.ID), "tc1")
	if _, _, err := eng.spawnGroupRun(ctx, map[string]any{
		"group_id": "finance", "goal": "核对发票", "mode": "readonly",
	}); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("spawn err=%v", err)
	}
	kids, _ := st.ListChildRuns(parent.ID)
	if len(kids) != 1 {
		t.Fatalf("children=%d", len(kids))
	}
	if err := eng.Execute(context.Background(), kids[0].ID, agent.Def{ID: "a", System: "sys"}, kids[0].Input); err != nil {
		t.Fatalf("child execute: %v", err)
	}
	child, _ := st.GetRun(kids[0].ID)
	if child.Status != store.StatusSucceeded {
		t.Fatalf("child status=%s", child.Status)
	}
	eng.NotifyChildTerminal(child)
	summary, facts := parseChildReceipt(child.Output)
	if summary != "合计 1240" || facts["total_tax"] != "1240.00" {
		t.Fatalf("receipt summary=%q facts=%+v", summary, facts)
	}
	if err := eng.ContinueFromChildReceipt(context.Background(), parent.ID, GroupRunReceipt{
		Status: string(child.Status), Summary: summary, Facts: facts, ChildRunID: child.ID,
	}); err != nil {
		t.Fatalf("parent resume: %v", err)
	}
	got, _ := st.GetRun(parent.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("parent status=%s", got.Status)
	}
}

func TestChildWaitingAndResumedNotifyParent(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "p"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "c", Depth: 1, ParentRunID: parent.ID, GroupID: "finance",
	})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st}
	payload := &store.HITLPayload{
		Kind: store.HITLKindApproval, ToolName: "create_ticket", Prompt: "approve?",
	}
	if err := eng.persistHITLWaiting(context.Background(), child.ID, payload); err != nil {
		t.Fatal(err)
	}
	// Re-persisting the same wait must not duplicate the parent event.
	if err := eng.persistHITLWaiting(context.Background(), child.ID, payload); err != nil {
		t.Fatal(err)
	}
	evs, _ := st.ListEvents(parent.ID)
	waiting := 0
	for _, ev := range evs {
		if ev.Type == "run.child_waiting" {
			waiting++
			if ev.Data["child_run_id"] != child.ID || ev.Data["kind"] != store.HITLKindApproval {
				t.Fatalf("event data=%+v", ev.Data)
			}
		}
	}
	if waiting != 1 {
		t.Fatalf("run.child_waiting count=%d want 1", waiting)
	}

	eng.notifyParentChildResumed(child.ID)
	evs, _ = st.ListEvents(parent.ID)
	resumed := 0
	for _, ev := range evs {
		if ev.Type == "run.child_resumed" && ev.Data["child_run_id"] == child.ID {
			resumed++
		}
	}
	if resumed != 1 {
		t.Fatalf("run.child_resumed count=%d want 1", resumed)
	}
}

func TestParseChildReceiptFacts(t *testing.T) {
	out := "合计 1240\nRECEIPT_JSON: {\"summary\":\"合计 1240\",\"facts\":{\"total_tax\":\"1240.00\",\"count\":3,\"ok\":true}}"
	summary, facts := parseChildReceipt(out)
	if summary != "合计 1240" {
		t.Fatalf("summary=%q", summary)
	}
	if facts["total_tax"] != "1240.00" || facts["count"] != float64(3) || facts["ok"] != true {
		t.Fatalf("facts=%+v", facts)
	}
}

func TestParseChildReceiptFallsBack(t *testing.T) {
	summary, facts := parseChildReceipt("普通回答")
	if summary != "普通回答" || facts != nil {
		t.Fatalf("summary=%q facts=%+v", summary, facts)
	}
	summary, facts = parseChildReceipt("答案\nRECEIPT_JSON: {broken")
	if summary != "答案\nRECEIPT_JSON: {broken" || facts != nil {
		t.Fatalf("bad json summary=%q facts=%+v", summary, facts)
	}
}

func TestContinueFromChildReceipt(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	parent, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "parent goal", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(parent.ID, store.Event{
		Type: EventLLMToolCall,
		Data: map[string]any{"id": "tc1", "name": SpawnGroupRunName, "arguments": map[string]any{}},
	})
	_ = st.UpdateRun(parent.ID, store.StatusWaitingHuman, "", "")
	_ = st.SetHITL(parent.ID, &store.HITLPayload{
		Kind: store.HITLKindWaitingChild, ToolName: SpawnGroupRunName, ToolCallID: "tc1", ChildRunID: "child_x",
	})
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		for _, m := range msgs {
			if m.Role == llm.RoleTool && strings.Contains(m.Content, "child_x") {
				return llm.Message{Role: llm.RoleAssistant, Content: "got receipt"}
			}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "done"}
	}}
	eng := &Engine{Store: st, LLM: llmStub, Tools: tool.NewRegistry(), Settings: scopedKnobs{on: true}}
	err = eng.ContinueFromChildReceipt(context.Background(), parent.ID, GroupRunReceipt{
		Status: string(store.StatusSucceeded), Summary: "ok", ChildRunID: "child_x",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(parent.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
}
