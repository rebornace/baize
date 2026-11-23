package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workspace"
)

// Paste offload happens before the run exists; successful rewrites must emit
// context.offloaded (source=paste) as a pre-event so CX2 can observe it.
func TestPostRunPasteOffloadEmitsContextOffloaded(t *testing.T) {
	mem := store.NewMemory()
	mem.UpsertAgent(store.Agent{ID: "a", System: "s"})
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})

	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv.Workspace = workspace.New(blobs)
	h := srv.Handler()

	large := strings.Repeat("p", run.PasteOffloadThreshold)
	if utf8.RuneCountInString(large) < run.PasteOffloadThreshold {
		t.Fatalf("fixture rune count=%d", utf8.RuneCountInString(large))
	}

	body := map[string]any{
		"agent_id":        "a",
		"input":           large,
		"conversation_id": "conv-paste-offload",
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	if runID == "" {
		t.Fatalf("created=%v", created)
	}

	getEvs := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID+"/events", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getEvs)
	if rr.Code != http.StatusOK {
		t.Fatalf("events status=%d", rr.Code)
	}
	var evs []store.Event
	if err := json.NewDecoder(rr.Body).Decode(&evs); err != nil {
		t.Fatal(err)
	}

	var found *store.Event
	for i := range evs {
		if evs[i].Type == run.EventContextOffloaded {
			found = &evs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("missing context.offloaded; events=%+v", evs)
	}
	if src, _ := found.Data["source"].(string); src != "paste" {
		t.Fatalf("source=%v want paste; data=%+v", found.Data["source"], found.Data)
	}
	path, _ := found.Data["path"].(string)
	if path == "" || !strings.HasPrefix(path, "offload/") {
		t.Fatalf("path=%q want offload/...; data=%+v", path, found.Data)
	}
}

func TestPostRunShortPasteNoOffloadEvent(t *testing.T) {
	mem := store.NewMemory()
	mem.UpsertAgent(store.Agent{ID: "a", System: "s"})
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})

	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv.Workspace = workspace.New(blobs)
	h := srv.Handler()

	body := map[string]any{
		"agent_id":        "a",
		"input":           "short paste",
		"conversation_id": "conv-short",
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)

	getEvs := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID+"/events", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getEvs)
	var evs []store.Event
	if err := json.NewDecoder(rr.Body).Decode(&evs); err != nil {
		t.Fatal(err)
	}
	for _, ev := range evs {
		if ev.Type == run.EventContextOffloaded {
			t.Fatalf("unexpected context.offloaded for short paste: %+v", ev)
		}
	}
}
