package api_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type workflowActivateLLM struct{}

func (workflowActivateLLM) Chat(context.Context, []llm.Message, []llm.ToolSpec) (llm.Message, error) {
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
		ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"},
	}}}, nil
}
func (workflowActivateLLM) SupportsVision() bool { return false }

func loadAPIWorkflowCatalog(t *testing.T, id, wfYAML string) *skill.Catalog {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: " + id + "\ndescription: pipeline\ntools:\n  - ta\n  - tb\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte(wfYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := skill.LoadCatalog([]string{root}, filepath.Join(root, "_user"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestWorkflowApprovalReleasesLeaseAndResumes(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	reg := tool.NewRegistry()
	var taCalls, tbCalls atomic.Int32
	reg.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		taCalls.Add(1)
		return map[string]any{"ok": true}, false, nil
	})
	reg.Register("tb", func(context.Context, map[string]any) (map[string]any, bool, error) {
		tbCalls.Add(1)
		return map[string]any{"id": "1"}, false, nil
	})
	cat := loadAPIWorkflowCatalog(t, "demo", "name: demo\nsteps:\n  - id: a\n    tool: ta\n  - id: b\n    tool: tb\n    approve: true\n")
	eng := &run.Engine{Store: st, LLM: workflowActivateLLM{}, Tools: reg, Skills: cat}
	srv := &api.Server{Store: st, Runner: eng, LeaseTTL: time.Minute}

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.ExecuteJob(context.Background(), middleware.Job{
		RunID: r.ID, Kind: middleware.KindRun, AgentID: "a", Input: "hi",
	}); err != nil {
		t.Fatalf("initial job: %v", err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%s want waiting_human", got.Status)
	}
	if taCalls.Load() != 1 || tbCalls.Load() != 0 {
		t.Fatalf("calls ta=%d tb=%d", taCalls.Load(), tbCalls.Load())
	}

	// The worker released its lease while waiting for approval.
	acquired, err := st.LeaseRun(r.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("workflow approval must not keep the run lease")
	}
	_ = st.ClearRunLease(r.ID)

	if err := srv.ExecuteJob(context.Background(), middleware.Job{
		RunID: r.ID, Kind: middleware.KindHitlResume, AgentID: "a", HITLApprove: true,
	}); err != nil {
		t.Fatalf("resume job: %v", err)
	}
	got, _ = st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("after resume status=%s", got.Status)
	}
	if tbCalls.Load() != 1 {
		t.Fatalf("tb calls=%d want 1", tbCalls.Load())
	}
	evs, _ := st.ListEvents(r.ID)
	completed := false
	for _, ev := range evs {
		if ev.Type == "workflow.step_completed" && ev.Data["step"] == "b" {
			completed = true
		}
	}
	if !completed {
		t.Fatalf("approved step b must emit workflow.step_completed; events=%+v", evs)
	}
}
