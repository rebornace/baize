package run

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type dualReadLLM struct{ calls int }

func (d *dualReadLLM) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	d.calls++
	if d.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "slow_a", Arguments: map[string]any{}},
			{ID: "c2", Name: "slow_b", Arguments: map[string]any{}},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}

func (d *dualReadLLM) SupportsVision() bool { return false }

func TestParallelSafeToolsOrderedAndFaster(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var started atomic.Int32
	gate := make(chan struct{})
	reg.Register("slow_a", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		started.Add(1)
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, true, ctx.Err()
		case <-time.After(2 * time.Second):
			return nil, true, context.DeadlineExceeded
		}
		return map[string]any{"id": "a"}, false, nil
	})
	reg.Register("slow_b", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		started.Add(1)
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, true, ctx.Err()
		case <-time.After(2 * time.Second):
			return nil, true, context.DeadlineExceeded
		}
		return map[string]any{"id": "b"}, false, nil
	})

	// Release both tools only after both have started (proves overlap).
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if started.Load() >= 2 {
				close(gate)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	eng := &Engine{
		Store:             st,
		LLM:               &dualReadLLM{},
		Tools:             reg,
		MaxSteps:          4,
		ToolTimeout:       3 * time.Second,
		ParallelToolLimit: 8,
	}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "go"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "go"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	elapsed := time.Since(start)
	if started.Load() < 2 {
		t.Fatalf("tools did not overlap; started=%d", started.Load())
	}
	// Serial would be ~2 sleeps; parallel should finish well under 1.5s wall once gated.
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("wall %s looks serial (want parallel)", elapsed)
	}

	evs, _ := st.ListEvents(r.ID)
	var toolOrder []string
	for _, ev := range evs {
		if ev.Type != EventToolResult {
			continue
		}
		name, _ := ev.Data["name"].(string)
		toolOrder = append(toolOrder, name)
	}
	if len(toolOrder) < 2 || toolOrder[0] != "slow_a" || toolOrder[1] != "slow_b" {
		t.Fatalf("tool result order=%v want [slow_a slow_b]", toolOrder)
	}
}

type approvalDualLLM struct{ calls int }

func (d *approvalDualLLM) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	d.calls++
	if d.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "read_x", Arguments: map[string]any{}},
			{ID: "c2", Name: "write_y", Arguments: map[string]any{}},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}

func (d *approvalDualLLM) SupportsVision() bool { return false }

func TestApprovalBatchStaysSerial(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	slow := func(name string) tool.Invoker {
		return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			cur := concurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(40 * time.Millisecond)
			concurrent.Add(-1)
			return map[string]any{"name": name}, false, nil
		}
	}
	reg.Register("read_x", slow("read_x"))
	reg.RegisterSpecApproved(llm.ToolSpec{
		Name: "write_y", Description: "write", InputSchema: map[string]any{"type": "object"},
	}, slow("write_y"), true)

	gate := NewGate()
	eng := &Engine{
		Store:             st,
		LLM:               &approvalDualLLM{},
		Tools:             reg,
		Gate:              gate,
		MaxSteps:          4,
		ToolTimeout:       3 * time.Second,
		ParallelToolLimit: 8,
	}
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "go"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "go")
	}()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := resumeAfterWait(t, eng, done, r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if maxConcurrent.Load() > 1 {
		t.Fatalf("approval batch must stay serial; maxConcurrent=%d", maxConcurrent.Load())
	}
}
