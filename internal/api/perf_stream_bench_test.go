package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// BenchmarkPerfStreamReplay times store event replay + SSE write for a finished
// run (N=100 llm.message events). Measures the terminal replay path only — no
// live Hub loop / LLM. Run is StatusSucceeded so handleRunStream exits after
// replay + run.ended.
func BenchmarkPerfStreamReplay(b *testing.B) {
	mem := store.NewMemory()
	srv := NewServer(mem, tool.NewRegistry(), &gateFakeRunner{store: mem})

	run, err := mem.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		b.Fatal(err)
	}
	const nEvents = 100
	for i := 0; i < nEvents; i++ {
		ev := store.Event{
			Type: "llm.message",
			Data: map[string]any{"content": fmt.Sprintf("msg-%d", i)},
		}
		if err := mem.AppendEvent(run.ID, ev); err != nil {
			b.Fatal(err)
		}
	}
	if err := mem.UpdateRun(run.ID, store.StatusSucceeded, "done", ""); err != nil {
		b.Fatal(err)
	}

	marker := fmt.Sprintf("msg-%d", nEvents-1)
	path := "/v0/runs/" + run.ID + "/stream?after=-1"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			b.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		if !strings.Contains(body, marker) {
			b.Fatalf("missing last event marker %q in body (len=%d)", marker, len(body))
		}
		if !strings.Contains(body, "run.ended") {
			b.Fatalf("missing run.ended: %s", body)
		}
	}
}
