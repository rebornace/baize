package api_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type approvalOnceLLM struct{}

func (approvalOnceLLM) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolSpec) (llm.Message, error) {
	for _, m := range messages {
		if m.Role == llm.RoleTool {
			return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
		}
	}
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
		{ID: "c1", Name: "needs_approval", Arguments: map[string]any{}},
	}}, nil
}

func (approvalOnceLLM) SupportsVision() bool { return false }

type freeReplyLLM struct{}

func (freeReplyLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSpec) (llm.Message, error) {
	return llm.Message{Role: llm.RoleAssistant, Content: "pong"}, nil
}
func (freeReplyLLM) SupportsVision() bool { return false }

func TestHITLWaitingDoesNotExhaustWorkerSlots(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	reg := tool.NewRegistry()
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "needs_approval"}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}, true)

	eng := &run.Engine{
		Store: st, LLM: approvalOnceLLM{}, Tools: reg, Gate: run.NewGate(), MaxSteps: 4,
	}
	srv := &api.Server{Store: st, Runner: eng, LeaseTTL: time.Second}

	const n = 8
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "approve me"})
			if err != nil {
				t.Errorf("create: %v", err)
				return
			}
			if err := srv.ExecuteJob(context.Background(), middleware.Job{
				RunID: r.ID, Kind: middleware.KindRun, AgentID: "a", Input: "approve me",
			}); err != nil {
				t.Errorf("ExecuteJob: %v", err)
				return
			}
			got, _ := st.GetRun(r.ID)
			if got.Status != store.StatusWaitingHuman {
				t.Errorf("status=%s want waiting_human", got.Status)
			}
		}()
	}
	wg.Wait()

	// While N runs wait on HITL, a free worker can still execute a non-HITL job.
	freeEng := &run.Engine{Store: st, LLM: freeReplyLLM{}, Tools: reg, MaxSteps: 4}
	freeSrv := &api.Server{Store: st, Runner: freeEng, LeaseTTL: time.Second}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := freeSrv.ExecuteJob(context.Background(), middleware.Job{
		RunID: r.ID, Kind: middleware.KindRun, AgentID: "a", Input: "ping",
	}); err != nil {
		t.Fatalf("free job: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("free job blocked by HITL waiters (%s)", time.Since(start))
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("free run status=%s want succeeded", got.Status)
	}
}

func TestHITLResumeInvokesOnce(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	reg := tool.NewRegistry()
	var invokes atomic.Int32
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "needs_approval"}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		invokes.Add(1)
		return map[string]any{"ok": true}, false, nil
	}, true)

	eng := &run.Engine{
		Store: st, LLM: approvalOnceLLM{}, Tools: reg, Gate: run.NewGate(), MaxSteps: 4,
	}
	srv := &api.Server{Store: st, Runner: eng, LeaseTTL: time.Second}

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.ExecuteJob(context.Background(), middleware.Job{
		RunID: r.ID, Kind: middleware.KindRun, AgentID: "a", Input: "go",
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%s", got.Status)
	}

	if err := srv.ExecuteJob(context.Background(), middleware.Job{
		RunID: r.ID, Kind: middleware.KindHitlResume, AgentID: "a", HITLApprove: true,
	}); err != nil {
		t.Fatal(err)
	}
	if invokes.Load() != 1 {
		t.Fatalf("invokes=%d want 1", invokes.Load())
	}
	got, _ = st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("after resume status=%s", got.Status)
	}

	if err := srv.ExecuteJob(context.Background(), middleware.Job{
		RunID: r.ID, Kind: middleware.KindHitlResume, AgentID: "a", HITLApprove: true,
	}); err != nil {
		t.Fatal(err)
	}
	if invokes.Load() != 1 {
		t.Fatalf("double resume invoked tool again: %d", invokes.Load())
	}
}
