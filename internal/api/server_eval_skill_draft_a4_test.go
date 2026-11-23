package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/procfs"
	"github.com/rebornace/baize/internal/store"
)

func countEventsOfType(t *testing.T, st store.Store, runID, typ string) int {
	t.Helper()
	evs, err := st.ListEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range evs {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func lastEventOfType(t *testing.T, st store.Store, runID, typ string) store.Event {
	t.Helper()
	evs, err := st.ListEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == typ {
			return evs[i]
		}
	}
	t.Fatalf("no event type %s on %s", typ, runID)
	return store.Event{}
}

func TestSkillDraftA4EmitsHumanEvents(t *testing.T) {
	srv, st, h := skillDraftServer(t)
	r := seedRunWithToolEvent(t, st)

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/runs/"+r.ID+"/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if countEventsOfType(t, st, r.ID, procfs.EventHumanSkillDraftOpened) != 1 {
		t.Fatal("want opened after GET")
	}
	opened := lastEventOfType(t, st, r.ID, procfs.EventHumanSkillDraftOpened)
	if opened.Data["source"] != "eval" {
		t.Fatalf("source=%v", opened.Data["source"])
	}

	srv.LLM = &fixedPolishLLM{
		content: "---\nname: a4-polish\ndescription: d\ntools:\n  - list_tickets\n---\n\nbody\n",
	}
	polishBody, _ := json.Marshal(map[string]string{"markdown": ""})
	req = httptest.NewRequest(http.MethodPost, "/v0/settings/eval/runs/"+r.ID+"/skill-draft/polish", bytes.NewReader(polishBody))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("polish: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if countEventsOfType(t, st, r.ID, procfs.EventHumanSkillDraftPolished) != 1 {
		t.Fatal("want polished")
	}

	md := string(skillMDBytes("a4-installed", "from a4", []string{"list_tickets"}))
	installBody, _ := json.Marshal(map[string]any{
		"id":       "a4-installed",
		"markdown": md,
	})
	req = httptest.NewRequest(http.MethodPost, "/v0/settings/eval/runs/"+r.ID+"/skill-draft/install", bytes.NewReader(installBody))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("install: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if countEventsOfType(t, st, r.ID, procfs.EventHumanSkillDraftInstalled) != 1 {
		t.Fatal("want installed")
	}
	inst := lastEventOfType(t, st, r.ID, procfs.EventHumanSkillDraftInstalled)
	if inst.Data["skill_id"] != "a4-installed" {
		t.Fatalf("skill_id=%v", inst.Data["skill_id"])
	}
	if inst.Data["edited"] != true {
		// custom markdown differs from rule draft
		t.Fatalf("edited=%v want true", inst.Data["edited"])
	}

	badBody, _ := json.Marshal(map[string]string{"markdown": "not-a-skill"})
	before := countEventsOfType(t, st, r.ID, procfs.EventHumanSkillDraftInstalled)
	req = httptest.NewRequest(http.MethodPost, "/v0/settings/eval/runs/"+r.ID+"/skill-draft/install", bytes.NewReader(badBody))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code == http.StatusOK {
		t.Fatal("want install failure")
	}
	if countEventsOfType(t, st, r.ID, procfs.EventHumanSkillDraftInstalled) != before {
		t.Fatal("failed install must not append installed")
	}
}

func TestConversationSkillDraftA4OpenedChat(t *testing.T) {
	_, st, h := skillDraftServer(t)
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "q", ConversationID: "conv_a4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendEvent(r.ID, store.Event{
		Type: "llm.tool_call",
		Data: map[string]any{"name": "list_tickets"},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/conv_a4/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	opened := lastEventOfType(t, st, r.ID, procfs.EventHumanSkillDraftOpened)
	if opened.Data["source"] != "chat" {
		t.Fatalf("source=%v", opened.Data["source"])
	}
}
