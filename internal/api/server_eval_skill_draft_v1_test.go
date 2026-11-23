package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
)

type fixedPolishLLM struct {
	content string
	err     error
}

func (f *fixedPolishLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	_ = ctx
	_ = messages
	_ = tools
	if f.err != nil {
		return llm.Message{}, f.err
	}
	return llm.Message{Role: llm.RoleAssistant, Content: f.content}, nil
}

func (f *fixedPolishLLM) SupportsVision() bool { return false }

func TestSkillDraftPolishMergesTools(t *testing.T) {
	srv, st, h := skillDraftServer(t)
	r := seedRunWithToolEvent(t, st)
	srv.LLM = &fixedPolishLLM{
		content: "---\nname: from-polish\ndescription: polished\ntools:\n  - list_tickets\n---\n\n## Steps\n\nPolished body.\n\n**Warning:** review\n",
	}

	// First get rule draft to know suggested id tools include list_tickets
	req := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/runs/"+r.ID+"/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	var draft struct {
		Markdown string `json:"markdown"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &draft)

	body, _ := json.Marshal(map[string]string{"markdown": draft.Markdown})
	req = httptest.NewRequest(http.MethodPost, "/v0/settings/eval/runs/"+r.ID+"/skill-draft/polish", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("polish: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Markdown string `json:"markdown"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Markdown, "Polished body") {
		t.Fatalf("markdown=%q", out.Markdown)
	}
	pkg, err := skill.ParseSKILLMD([]byte(out.Markdown))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Tools) < 1 || pkg.Tools[0] != "list_tickets" {
		t.Fatalf("tools=%v", pkg.Tools)
	}
}

func TestSkillDraftPolishNilLLM503(t *testing.T) {
	srv, st, h := skillDraftServer(t)
	r := seedRunWithToolEvent(t, st)
	srv.LLM = nil
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/runs/"+r.ID+"/skill-draft/polish", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d", rr.Code)
	}
}

func TestSkillDraftInstallCreateEvalStub(t *testing.T) {
	srv, st, h := skillDraftServer(t)
	r := seedRunWithToolEvent(t, st)
	md := string(skillMDBytes("from-stub", "d", []string{"list_tickets"}))
	installBody, _ := json.Marshal(map[string]any{
		"markdown":         md,
		"create_eval_stub": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/runs/"+r.ID+"/skill-draft/install", bytes.NewReader(installBody))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		ID       string `json:"id"`
		EvalStub string `json:"eval_stub"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out.EvalStub != "created" {
		t.Fatalf("eval_stub=%q", out.EvalStub)
	}
	doc, ok := srv.SkillCatalog.GetEval("from-stub")
	if !ok || doc.Harness != "evidence-eval" || len(doc.Cases) != 0 {
		t.Fatalf("eval=%+v ok=%v", doc, ok)
	}

	// second install with stub should skip
	installBody, _ = json.Marshal(map[string]any{
		"markdown":         md,
		"create_eval_stub": true,
	})
	req = httptest.NewRequest(http.MethodPost, "/v0/settings/eval/runs/"+r.ID+"/skill-draft/install", bytes.NewReader(installBody))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out.EvalStub != "skipped_exists" {
		t.Fatalf("second eval_stub=%q", out.EvalStub)
	}
}

func TestConversationSkillDraft(t *testing.T) {
	_, st, h := skillDraftServer(t)
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "q", ConversationID: "conv_skill",
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

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/conv_skill/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/conversations/conv_missing/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing: code=%d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/conversations/conv_skill/skill-draft", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op: code=%d", rr.Code)
	}
}
