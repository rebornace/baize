package run_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type hitlNoopJobQueue struct{}

func (hitlNoopJobQueue) Enqueue(context.Context, middleware.Job) error { return nil }
func (hitlNoopJobQueue) Consume(context.Context) (middleware.Job, func(), bool) {
	return middleware.Job{}, nil, false
}
func (hitlNoopJobQueue) Close() error { return nil }

type hitlNoopRunner struct{}

func (hitlNoopRunner) Execute(context.Context, string, agent.Def, string) error { return nil }
func (hitlNoopRunner) ContinueFromHITL(context.Context, string, run.Decision) error {
	return nil
}

type scheduledApprovalLLM struct{}

func (scheduledApprovalLLM) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolSpec) (llm.Message, error) {
	for _, m := range messages {
		if m.Role == llm.RoleTool {
			return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
		}
	}
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
		{ID: "c1", Name: "write_file", Arguments: map[string]any{"path": "x"}},
	}}, nil
}

func (scheduledApprovalLLM) SupportsVision() bool { return false }

func waitScheduledRunStatus(t *testing.T, st store.Store, id string, want store.Status) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r, err := st.GetRun(id)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	r, _ := st.GetRun(id)
	t.Fatalf("status=%s want %s", r.Status, want)
}

func TestScheduledRunStillWaitsForApproval(t *testing.T) {
	const scheduleInstruction = "到点执行写文件"

	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "agent-a", System: "sys"})
	if _, err := st.CreateRun(store.CreateRunInput{
		AgentID: "agent-a", Input: "setup", ConversationID: "conv1",
	}); err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	var invokes atomic.Int32
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "write_file"}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		invokes.Add(1)
		return map[string]any{"ok": true}, false, nil
	}, true)

	srv := api.NewServer(st, reg, hitlNoopRunner{})
	srv.Queue = hitlNoopJobQueue{}
	opener := api.NewScheduleRunOpener(srv)

	runID, err := opener.Open("conv1", scheduleInstruction, "alice")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	opened, err := st.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Input != scheduleInstruction || opened.AgentID != "agent-a" {
		t.Fatalf("opened run=%+v", opened)
	}

	gate := run.NewGate()
	eng := &run.Engine{Store: st, LLM: scheduledApprovalLLM{}, Tools: reg, Gate: gate, MaxSteps: 4}
	ag := agent.Def{ID: "agent-a", System: "sys"}

	errCh := make(chan error, 1)
	go func() { errCh <- eng.Execute(context.Background(), runID, ag, opened.Input) }()

	waitScheduledRunStatus(t, st, runID, store.StatusWaitingHuman)
	if invokes.Load() != 0 {
		t.Fatalf("invoke count=%d want 0 before approval", invokes.Load())
	}

	select {
	case err := <-errCh:
		if err == nil || !errors.Is(err, run.ErrWaitingHuman) {
			t.Fatalf("Execute should release with ErrWaitingHuman, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Execute did not return after HITL pause")
	}
}
