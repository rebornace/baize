package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func skillDraftServer(t *testing.T) (*Server, store.Store, http.Handler) {
	t.Helper()
	srv, st, h, _ := skillsServer(t)
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"
	return srv, st, h
}

func seedRunWithToolEvent(t *testing.T, st store.Store) *store.Run {
	t.Helper()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "query tickets"})
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now().UTC().Add(-time.Minute)
	if err := st.AppendEvent(r.ID, store.Event{
		Type:      "llm.tool_call",
		Timestamp: ts,
		Data:      map[string]any{"name": "list_tickets", "arguments": map[string]any{}},
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSkillDraftRequiresAdmin(t *testing.T) {
	_, st, h := skillDraftServer(t)
	r := seedRunWithToolEvent(t, st)

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/runs/"+r.ID+"/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator GET: code=%d body=%s", rr.Code, rr.Body.String())
	}

	installBody, _ := json.Marshal(map[string]string{"markdown": "---\nname: x\ndescription: d\n---\n\nb\n"})
	req = httptest.NewRequest(http.MethodPost, "/v0/settings/eval/runs/"+r.ID+"/skill-draft/install", bytes.NewReader(installBody))
	req.Header.Set("Authorization", "Bearer op")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator POST install: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSkillDraftGETReturnsMarkdown(t *testing.T) {
	_, st, h := skillDraftServer(t)
	r := seedRunWithToolEvent(t, st)

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/runs/"+r.ID+"/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		SourceRunID string `json:"source_run_id"`
		SuggestedID string `json:"suggested_id"`
		Markdown    string `json:"markdown"`
		Timeline    []struct {
			RunID   string `json:"run_id"`
			Type    string `json:"type"`
			Summary string `json:"summary"`
		} `json:"timeline"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.SourceRunID != r.ID || out.SuggestedID == "" {
		t.Fatalf("meta: %+v", out)
	}
	if !strings.Contains(out.Markdown, "---") || !strings.Contains(out.Markdown, "name:") {
		t.Fatalf("markdown=%q", out.Markdown)
	}
	if len(out.Timeline) < 1 || out.Timeline[0].RunID != r.ID {
		t.Fatalf("timeline=%+v", out.Timeline)
	}
}

func TestSkillDraftGETNoEvents422(t *testing.T) {
	_, st, h := skillDraftServer(t)
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "empty"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/runs/"+r.ID+"/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "no_events") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestSkillDraftGETRunNotFound(t *testing.T) {
	_, _, h := skillDraftServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/runs/run_missing/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSkillDraftInstall(t *testing.T) {
	srv, st, h := skillDraftServer(t)
	r := seedRunWithToolEvent(t, st)

	md := string(skillMDBytes("from-test", "eval skill draft", []string{"list_tickets"}))
	installBody, _ := json.Marshal(map[string]string{
		"id":       "from-test",
		"markdown": md,
	})
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/runs/"+r.ID+"/skill-draft/install", bytes.NewReader(installBody))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("install: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var installed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &installed); err != nil {
		t.Fatal(err)
	}
	if installed.ID != "from-test" {
		t.Fatalf("id=%q", installed.ID)
	}
	if _, ok := srv.SkillCatalog.Get("from-test"); !ok {
		t.Fatal("skill not in catalog after install")
	}
}
