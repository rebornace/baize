package api_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type scheduleNoopJobQueue struct{}

func (scheduleNoopJobQueue) Enqueue(context.Context, middleware.Job) error { return nil }
func (scheduleNoopJobQueue) Consume(context.Context) (middleware.Job, func(), bool) {
	return middleware.Job{}, nil, false
}
func (scheduleNoopJobQueue) Close() error { return nil }

type scheduleNoopRunner struct{}

func (scheduleNoopRunner) Execute(context.Context, string, agent.Def, string) error { return nil }
func (scheduleNoopRunner) ContinueFromHITL(context.Context, string, run.Decision) error {
	return nil
}

func TestScheduleRunOpenCopiesLatestRun(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "agent-a", System: "sys"})
	if _, err := st.CreateRun(store.CreateRunInput{
		AgentID:        "agent-a",
		Input:          "first turn",
		ConversationID: "conv1",
		IdentityID:     "id-1",
		ModelProfileID: "mp-1",
		ThinkingLevel:  store.ThinkingLow,
	}); err != nil {
		t.Fatal(err)
	}

	srv := api.NewServer(st, tool.NewRegistry(), scheduleNoopRunner{})
	srv.Queue = scheduleNoopJobQueue{}
	opener := api.NewScheduleRunOpener(srv)

	runID, err := opener.Open("conv1", "喝水", "alice")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := st.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Input != "喝水" || got.AgentID != "agent-a" {
		t.Fatalf("run=%+v", got)
	}
	if got.IdentityID != "id-1" || got.ModelProfileID != "mp-1" || got.ThinkingLevel != store.ThinkingLow {
		t.Fatalf("identity/model/thinking=%+v", got)
	}
	if len(got.PassthroughHeaders) != 0 || got.WebhookConfig != nil {
		t.Fatalf("unexpected extras on scheduled run: %+v", got)
	}
}

func TestScheduleRunOpenNoPriorRun(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "agent-a", System: "sys"})
	srv := api.NewServer(st, tool.NewRegistry(), scheduleNoopRunner{})
	srv.Queue = scheduleNoopJobQueue{}
	opener := api.NewScheduleRunOpener(srv)

	if _, err := st.LatestConversationRun("conv_empty"); !errors.Is(err, store.ErrNoPriorConversationRun) {
		t.Fatalf("before Open: %v", err)
	}
	runID, err := opener.Open("conv_empty", "喝水", "alice")
	if err == nil {
		t.Fatal("want error when no prior run")
	}
	if runID != "" {
		t.Fatalf("runID=%q", runID)
	}
	if _, err := st.LatestConversationRun("conv_empty"); !errors.Is(err, store.ErrNoPriorConversationRun) {
		t.Fatalf("must not create run: %v", err)
	}
}

func TestScheduleRunOpenUsesNewestRun(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "old-agent", System: "sys"})
	st.UpsertAgent(store.Agent{ID: "new-agent", System: "sys"})
	if _, err := st.CreateRun(store.CreateRunInput{
		AgentID: "old-agent", Input: "old", ConversationID: "conv1",
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := st.CreateRun(store.CreateRunInput{
		AgentID: "new-agent", Input: "new", ConversationID: "conv1",
	}); err != nil {
		t.Fatal(err)
	}

	srv := api.NewServer(st, tool.NewRegistry(), scheduleNoopRunner{})
	srv.Queue = scheduleNoopJobQueue{}
	opener := api.NewScheduleRunOpener(srv)
	runID, err := opener.Open("conv1", "定时指令", "alice")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(runID)
	if got.AgentID != "new-agent" || got.Input != "定时指令" {
		t.Fatalf("run=%+v", got)
	}
}
