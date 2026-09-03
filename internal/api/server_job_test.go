package api_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	_ "github.com/rebornace/baize/internal/middleware/memory"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// jobRunner records executed run IDs and captures per-run options when
// invoked through the RunWithOptions path.
type jobRunner struct {
	executed chan string
	opts     chan run.RunOptions
}

func (r *jobRunner) Execute(_ context.Context, runID string, _ agent.Def, _ string) error {
	r.executed <- runID
	return nil
}

func (r *jobRunner) ExecuteWithOpts(_ context.Context, runID string, _ agent.Def, _ string, opts run.RunOptions) error {
	r.executed <- runID
	if r.opts != nil {
		r.opts <- opts
	}
	return nil
}

func (r *jobRunner) ContinueFromHITL(context.Context, string, run.Decision) error { return nil }

func newJobTestServer(t *testing.T, rr *jobRunner) (*api.Server, store.Store) {
	t.Helper()
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "你是助手"})
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, rr)
	return srv, st
}

func newJobRunner() *jobRunner {
	return &jobRunner{executed: make(chan string, 16), opts: make(chan run.RunOptions, 16)}
}

func TestExecuteJobTerminalRunSkipped(t *testing.T) {
	rr := newJobRunner()
	srv, st := newJobTestServer(t, rr)

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(r.ID, store.StatusSucceeded, "out", ""); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.ExecuteJob(context.Background(), middleware.Job{RunID: r.ID, Kind: middleware.KindRun})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ExecuteJob on terminal run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ExecuteJob on terminal run should return immediately")
	}
	select {
	case id := <-rr.executed:
		t.Fatalf("runner must not be called for terminal run, got %s", id)
	default:
	}
}

func TestExecuteJobLeaseGuardsRun(t *testing.T) {
	rr := newJobRunner()
	srv, st := newJobTestServer(t, rr)

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// Another worker holds a live lease: ExecuteJob must ack-skip.
	held, err := st.LeaseRun(r.ID, time.Minute)
	if err != nil || !held {
		t.Fatalf("seed lease: held=%v err=%v", held, err)
	}

	if err := srv.ExecuteJob(context.Background(), middleware.Job{RunID: r.ID, Kind: middleware.KindRun}); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-rr.executed:
		t.Fatalf("runner must not run while lease held by another worker, got %s", id)
	default:
	}
}

func TestDispatchLocalRunsJob(t *testing.T) {
	rr := newJobRunner()
	srv, st := newJobTestServer(t, rr)

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	srv.Dispatch(context.Background(), middleware.Job{RunID: r.ID, Kind: middleware.KindRun, AgentID: "a", Input: "hello"})

	select {
	case id := <-rr.executed:
		if id != r.ID {
			t.Fatalf("executed %s want %s", id, r.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("locally dispatched job was not executed")
	}
}

func TestDispatchEnqueuesWhenQueuePresent(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	rr := newJobRunner()
	srv, st := newJobTestServer(t, rr)
	srv.Queue = mw.Queue

	stop := mw.StartWorkers(context.Background(), srv)
	defer stop()

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	srv.Dispatch(context.Background(), middleware.Job{RunID: r.ID, Kind: middleware.KindRun, AgentID: "a", Input: "hello"})

	select {
	case id := <-rr.executed:
		if id != r.ID {
			t.Fatalf("executed %s want %s", id, r.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued job was not executed by worker")
	}
}

func TestPartsRoundTripThroughJob(t *testing.T) {
	imgBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A}
	llmParts := []llm.ContentPart{
		{Type: "text", Text: "看图"},
		{Type: "image", ImageMIME: "image/png", ImageBytes: imgBytes},
		{Type: "image", DataURL: "data:image/jpeg;base64,QUJD"},
	}
	mwParts := api.PartsToMiddleware(llmParts)
	if len(mwParts) != 3 {
		t.Fatalf("PartsToMiddleware len=%d want 3", len(mwParts))
	}
	if mwParts[0].Type != "text" || mwParts[0].Text != "看图" {
		t.Fatalf("text part = %+v", mwParts[0])
	}
	if mwParts[1].Type != "image" || !strings.HasPrefix(mwParts[1].DataURL, "data:image/png;base64,") {
		t.Fatalf("image bytes part = %+v", mwParts[1])
	}
	if mwParts[2].DataURL != "data:image/jpeg;base64,QUJD" {
		t.Fatalf("prebuilt data URL part = %+v", mwParts[2])
	}

	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	rr := newJobRunner()
	srv, st := newJobTestServer(t, rr)
	srv.Queue = mw.Queue
	stop := mw.StartWorkers(context.Background(), srv)
	defer stop()

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	srv.Dispatch(context.Background(), middleware.Job{
		RunID:     r.ID,
		Kind:      middleware.KindRun,
		AgentID:   "a",
		Input:     "hello",
		Skills:    []string{"s1"},
		UserParts: mwParts,
	})

	var opts run.RunOptions
	select {
	case <-rr.executed:
	case <-time.After(2 * time.Second):
		t.Fatal("job with parts was not executed")
	}
	select {
	case opts = <-rr.opts:
	case <-time.After(time.Second):
		t.Fatal("runner options were not captured")
	}
	if len(opts.UserParts) != 3 {
		t.Fatalf("roundtrip parts len=%d want 3", len(opts.UserParts))
	}
	if opts.UserParts[0].Type != "text" || opts.UserParts[0].Text != "看图" {
		t.Fatalf("roundtrip text part = %+v", opts.UserParts[0])
	}
	if opts.UserParts[1].Type != "image" || opts.UserParts[1].DataURL != mwParts[1].DataURL {
		t.Fatalf("roundtrip image part = %+v", opts.UserParts[1])
	}
	if opts.UserParts[2].DataURL != "data:image/jpeg;base64,QUJD" {
		t.Fatalf("roundtrip prebuilt image part = %+v", opts.UserParts[2])
	}
	if len(opts.Skills) != 1 || opts.Skills[0] != "s1" {
		t.Fatalf("roundtrip skills = %v", opts.Skills)
	}
}
