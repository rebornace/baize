package run

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestProbeReadonlyParallelOrdered(t *testing.T) {
	reg := tool.NewRegistry()
	var order atomic.Int32
	reg.RegisterSpec(llm.ToolSpec{Name: "read_a"}, func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		time.Sleep(30 * time.Millisecond)
		order.Add(1)
		return map[string]any{"v": "a"}, false, nil
	})
	reg.RegisterSpec(llm.ToolSpec{Name: "read_b"}, func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		order.Add(1)
		return map[string]any{"v": "b"}, false, nil
	})
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a"})
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "q"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, Tools: reg, ParallelToolLimit: 8}
	eng.beginRunSkills(r.ID, nil, "sys")
	ctx := identity.WithRunID(context.Background(), r.ID)
	out, isErr, err := eng.probeReadonly(ctx, map[string]any{
		"calls": []any{
			map[string]any{"name": "read_a", "arguments": map[string]any{}},
			map[string]any{"name": "read_b", "arguments": map[string]any{}},
		},
	})
	if err != nil || isErr {
		t.Fatalf("out=%v isErr=%v err=%v", out, isErr, err)
	}
	results, _ := out["results"].([]map[string]any)
	if len(results) != 2 {
		t.Fatalf("results=%+v", out)
	}
	if results[0]["name"] != "read_a" || results[1]["name"] != "read_b" {
		t.Fatalf("order broken: %+v", results)
	}
	c0, _ := results[0]["content"].(map[string]any)
	c1, _ := results[1]["content"].(map[string]any)
	if c0["v"] != "a" || c1["v"] != "b" {
		t.Fatalf("content=%+v %+v", c0, c1)
	}
	evs, _ := st.ListEvents(r.ID)
	if !containsType(eventTypes(evs), EventProbeReadonly) {
		t.Fatalf("missing probe.readonly event: %v", eventTypes(evs))
	}
}

func TestProbeReadonlyRejectsApprovalAndLogin(t *testing.T) {
	reg := tool.NewRegistry()
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "write_x"}, func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}, true)
	reg.RegisterSpec(llm.ToolSpec{Name: "need_login"}, func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	if err := reg.SetRequireLogin("need_login", true); err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a"})
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "q"})
	eng := &Engine{Store: st, Tools: reg}
	eng.beginRunSkills(r.ID, nil, "sys")
	ctx := identity.WithRunID(context.Background(), r.ID)

	out, isErr, err := eng.probeReadonly(ctx, map[string]any{
		"calls": []any{map[string]any{"name": "write_x"}},
	})
	if err != nil || !isErr {
		t.Fatalf("approval: out=%v isErr=%v err=%v", out, isErr, err)
	}

	out, isErr, err = eng.probeReadonly(ctx, map[string]any{
		"calls": []any{map[string]any{"name": "need_login"}},
	})
	if err != nil || !isErr {
		t.Fatalf("login: out=%v isErr=%v err=%v", out, isErr, err)
	}
}

func TestProbeReadonlyRejectsActivate(t *testing.T) {
	reg := tool.NewRegistry()
	st := store.NewMemory()
	eng := &Engine{Store: st, Tools: reg}
	out, isErr, err := eng.probeReadonly(context.Background(), map[string]any{
		"calls": []any{map[string]any{"name": "activate_skill"}},
	})
	if err != nil || !isErr {
		t.Fatalf("out=%v isErr=%v err=%v", out, isErr, err)
	}
}
