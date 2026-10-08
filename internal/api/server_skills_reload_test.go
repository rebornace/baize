package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestPostRunPinnedSkillsFromMention(t *testing.T) {
	_, _, _, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "@data-analytics build a dashboard",
		"conversation_id": "c-pin",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		PinnedSkills []string `json:"pinned_skills"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if len(created.PinnedSkills) != 1 || created.PinnedSkills[0] != "data-analytics" {
		t.Fatalf("pinned_skills=%v", created.PinnedSkills)
	}

	// History list must return the same pins (persisted on the user message).
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c-pin/messages", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, req)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var msgs []struct {
		Role         string   `json:"role"`
		PinnedSkills []string `json:"pinned_skills"`
	}
	if err := json.NewDecoder(listRR.Body).Decode(&msgs); err != nil {
		t.Fatal(err)
	}
	var userPins []string
	for _, m := range msgs {
		if m.Role == "user" && len(m.PinnedSkills) > 0 {
			userPins = m.PinnedSkills
			break
		}
	}
	if len(userPins) != 1 || userPins[0] != "data-analytics" {
		t.Fatalf("history pinned_skills=%v messages=%+v", userPins, msgs)
	}
}

func TestPostRunPinnedSkillsFromAgentDefaults(t *testing.T) {
	_, _, _, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "hello",
		"conversation_id": "c-defaults",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		PinnedSkills []string `json:"pinned_skills"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if len(created.PinnedSkills) != 1 || created.PinnedSkills[0] != "data-analytics" {
		t.Fatalf("pinned_skills=%v want agent default", created.PinnedSkills)
	}
}

func TestPostRunPinnedSkillsExplicitEmpty(t *testing.T) {
	_, _, _, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "hello",
		"conversation_id": "c-empty",
		"skills":          []string{},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		PinnedSkills []string `json:"pinned_skills"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.PinnedSkills == nil || len(created.PinnedSkills) != 0 {
		t.Fatalf("pinned_skills=%v want empty slice", created.PinnedSkills)
	}
}

func TestPostRunReloadMentionStripped(t *testing.T) {
	_, _, llmMock, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "/reload continue",
		"conversation_id": "c-reload",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	pollRunStatus(t, h, runID, store.StatusSucceeded)

	userText, _, _ := llmMock.snapshot()
	if strings.Contains(userText, "/reload") || strings.Contains(userText, "@reload") {
		t.Fatalf("reload mention must be stripped: %q", userText)
	}
	if !strings.Contains(userText, "continue") {
		t.Fatalf("cleaned text missing: %q", userText)
	}
}

func TestPostRunAutoReloadAfterCatalogChange(t *testing.T) {
	srv, _, _, h, cat := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	// First run aligns conversation watermark.
	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "hi",
		"conversation_id": "c-auto",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("first run status=%d body=%s", rr.Code, rr.Body.String())
	}
	genBefore := cat.Generation()

	// Simulate admin adding a skill: Reload bumps generation so the next run
	// on this conversation is dirty and realigns.
	if err := cat.Reload(); err != nil {
		t.Fatal(err)
	}
	if cat.Generation() <= genBefore {
		t.Fatalf("Reload must bump generation")
	}

	rr2 := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "next",
		"conversation_id": "c-auto",
		"skills_reload":   true,
	})
	if rr2.Code != http.StatusOK {
		t.Fatalf("second run status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if srv.SkillCatalog.Generation() < cat.Generation() {
		t.Fatal("server catalog should stay aligned after reload path")
	}
}

func TestPutAgentToolBindingExclusive(t *testing.T) {
	_, _, _, h, _ := attachmentsServer(t, false)

	req := httptest.NewRequest(http.MethodPut, "/v0/agents/a1",
		jsonBody(t, map[string]any{
			"system":       "helper",
			"skills":       []string{"data-analytics"},
			"tool_binding": "exclusive",
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var ag store.Agent
	if err := json.NewDecoder(rr.Body).Decode(&ag); err != nil {
		t.Fatal(err)
	}
	if ag.ToolBinding != store.ToolBindingExclusive {
		t.Fatalf("tool_binding=%q", ag.ToolBinding)
	}

	reqBad := httptest.NewRequest(http.MethodPut, "/v0/agents/a1",
		jsonBody(t, map[string]any{
			"system":       "helper",
			"skills":       []string{"data-analytics"},
			"tool_binding": "weird",
		}))
	rrBad := httptest.NewRecorder()
	h.ServeHTTP(rrBad, reqBad)
	if rrBad.Code != http.StatusBadRequest {
		t.Fatalf("bad binding status=%d", rrBad.Code)
	}
}
