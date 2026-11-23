package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func uploadSkillMD(t *testing.T, h http.Handler, name string, raw []byte) {
	t.Helper()
	body, ctype := multipartFile(t, "file", name+".md", raw)
	req := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	req.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upload: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSkillWorkflowRoundTripAndValidation(t *testing.T) {
	srv, _, h, _ := skillsServer(t)
	srv.Registry.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{}, false, nil
	})
	uploadSkillMD(t, h, "demo", skillMDBytes("demo", "demo", []string{"ta"}))

	initial := httptest.NewRecorder()
	h.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/v0/skills/demo/workflow", nil))
	if initial.Code != http.StatusOK {
		t.Fatalf("GET initial: %d %s", initial.Code, initial.Body.String())
	}
	var empty struct {
		Editable bool `json:"editable"`
		Workflow any  `json:"workflow"`
	}
	_ = json.Unmarshal(initial.Body.Bytes(), &empty)
	if !empty.Editable || empty.Workflow != nil {
		t.Fatalf("initial view=%s", initial.Body.String())
	}

	wf := map[string]any{
		"name": "ignored-name-is-reset",
		"steps": []any{
			map[string]any{"id": "lookup", "tool": "ta", "args": map[string]any{"q": "{{input.query}}"}},
			map[string]any{"id": "again", "tool": "ta", "args": map[string]any{"id": "{{lookup.result.id}}"}},
		},
	}
	raw, _ := json.Marshal(wf)
	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/workflow", bytes.NewReader(raw))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rr.Code, rr.Body.String())
	}
	var view struct {
		SkillID  string `json:"skill_id"`
		Editable bool   `json:"editable"`
		YAML     string `json:"yaml"`
		Workflow struct {
			Name  string `json:"name"`
			Steps []struct {
				ID   string `json:"id"`
				Tool string `json:"tool"`
			} `json:"steps"`
		} `json:"workflow"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Workflow.Name != "demo" || len(view.Workflow.Steps) != 2 || view.Workflow.Steps[1].ID != "again" {
		t.Fatalf("PUT view=%s", rr.Body.String())
	}
	if !bytes.Contains([]byte(view.YAML), []byte("name: demo")) {
		t.Fatalf("YAML missing: %q", view.YAML)
	}

	rawYAML := "name: demo\nsteps:\n  - id: lookup\n    tool: ta\n    args:\n      q: \"{{input.query}}\"\n# keep comment\n"
	rawReq := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/workflow/raw",
		bytes.NewReader([]byte(`{"yaml":`+strconv.Quote(rawYAML)+`}`)))
	rawRR := httptest.NewRecorder()
	h.ServeHTTP(rawRR, rawReq)
	if rawRR.Code != http.StatusOK {
		t.Fatalf("raw PUT: %d %s", rawRR.Code, rawRR.Body.String())
	}
	get2 := httptest.NewRecorder()
	h.ServeHTTP(get2, httptest.NewRequest(http.MethodGet, "/v0/skills/demo/workflow", nil))
	if !bytes.Contains(get2.Body.Bytes(), []byte("keep comment")) {
		t.Fatalf("raw workflow not preserved: %s", get2.Body.String())
	}

	previewBody := []byte(`{"input":{"query":"open"},"step_results":{"lookup":{"id":"t1"}}}`)
	previewReq := httptest.NewRequest(http.MethodPost, "/v0/skills/demo/workflow/preview", bytes.NewReader(previewBody))
	previewRR := httptest.NewRecorder()
	h.ServeHTTP(previewRR, previewReq)
	if previewRR.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", previewRR.Code, previewRR.Body.String())
	}
	var preview struct {
		Steps []struct {
			ID     string         `json:"id"`
			Status string         `json:"status"`
			Args   map[string]any `json:"args"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(previewRR.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Steps) != 1 || preview.Steps[0].Status != "rendered" || preview.Steps[0].Args["q"] != "open" {
		t.Fatalf("preview=%s", previewRR.Body.String())
	}

	for _, bad := range []map[string]any{
		{"name": "demo", "steps": []any{map[string]any{"id": "a", "tool": "missing"}}},
		{"name": "demo", "steps": []any{map[string]any{"id": "a", "tool": "ta", "args": map[string]any{"x": "{{later.result.id}}"}}}},
		{"name": "demo", "steps": []any{map[string]any{"id": "a.b", "tool": "ta"}}},
	} {
		raw, _ := json.Marshal(bad)
		req := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/workflow", bytes.NewReader(raw))
		badRR := httptest.NewRecorder()
		h.ServeHTTP(badRR, req)
		if badRR.Code != http.StatusBadRequest {
			t.Fatalf("bad workflow accepted: %+v -> %d %s", bad, badRR.Code, badRR.Body.String())
		}
	}

	del := httptest.NewRequest(http.MethodDelete, "/v0/skills/demo/workflow", nil)
	delRR := httptest.NewRecorder()
	h.ServeHTTP(delRR, del)
	if delRR.Code != http.StatusOK {
		t.Fatalf("DELETE: %d %s", delRR.Code, delRR.Body.String())
	}
	after := httptest.NewRecorder()
	h.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/v0/skills/demo/workflow", nil))
	if after.Body.String() == "" || bytes.Contains(after.Body.Bytes(), []byte(`"workflow":{`)) {
		t.Fatalf("workflow still present: %s", after.Body.String())
	}
}

func TestCloneBuiltinWorkflowToUser(t *testing.T) {
	srv, _, h, cat := skillsServer(t)
	p, ok := cat.Get("builtin-skill")
	if !ok || p.Dir == "" {
		t.Fatal("builtin skill dir missing")
	}
	wf := "name: builtin-skill\nsteps:\n  - id: a\n    tool: a\n"
	if err := os.WriteFile(filepath.Join(p.Dir, "workflow.yaml"), []byte(wf), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cat.Reload(); err != nil {
		t.Fatal(err)
	}
	srv.Registry.Register("a", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{}, false, nil
	})
	clone := httptest.NewRequest(http.MethodPost, "/v0/skills/builtin-skill/workflow/clone", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, clone)
	if rr.Code != http.StatusOK {
		t.Fatalf("clone: %d %s", rr.Code, rr.Body.String())
	}
	var view struct {
		Editable bool   `json:"editable"`
		Source   string `json:"source"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &view)
	if !view.Editable || view.Source != "user" {
		t.Fatalf("clone view=%s", rr.Body.String())
	}
	// Second clone is an explicit conflict.
	clone2 := httptest.NewRequest(http.MethodPost, "/v0/skills/builtin-skill/workflow/clone", nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, clone2)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("second clone: %d %s", rr2.Code, rr2.Body.String())
	}
}

func TestBuiltinWorkflowIsReadOnly(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	raw, _ := json.Marshal(map[string]any{"name": "builtin-skill", "steps": []any{map[string]any{"id": "a", "tool": "a"}}})
	req := httptest.NewRequest(http.MethodPut, "/v0/skills/builtin-skill/workflow", bytes.NewReader(raw))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("builtin PUT: %d %s", rr.Code, rr.Body.String())
	}
}

func TestSkillRequiresToolsGuard(t *testing.T) {
	srv, _, h, _ := skillsServer(t)
	srv.Registry.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{}, false, nil
	})
	raw := []byte(`---
name: domain-demo
description: domain guard
domain: project
tools:
  - ta
requires_tools:
  - ta
  - missing_tool
---
body
`)
	uploadSkillMD(t, h, "domain-demo", raw)

	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/v0/skills", nil))
	if !bytes.Contains(list.Body.Bytes(), []byte(`"missing_tools":["missing_tool"]`)) {
		t.Fatalf("list missing_tools=%s", list.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/agents/default-agent",
		bytes.NewReader([]byte(`{"system":"x","skills":["domain-demo"]}`)))
	putRR := httptest.NewRecorder()
	h.ServeHTTP(putRR, put)
	if putRR.Code != http.StatusBadRequest || !bytes.Contains(putRR.Body.Bytes(), []byte("missing_tool")) {
		t.Fatalf("agent save must reject missing tools: %d %s", putRR.Code, putRR.Body.String())
	}
}

func TestAgentDomainWarning(t *testing.T) {
	srv, _, h, _ := skillsServer(t)
	srv.Registry.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{}, false, nil
	})
	raw := []byte(`---
name: domain-ok
description: domain ok
domain: project
tools:
  - ta
---
body
`)
	uploadSkillMD(t, h, "domain-ok", raw)
	put := httptest.NewRequest(http.MethodPut, "/v0/agents/default-agent",
		bytes.NewReader([]byte(`{"system":"x","skills":["domain-ok"]}`)))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte("domain_skill_default:domain-ok:project")) {
		t.Fatalf("domain warning missing: %d %s", rr.Code, rr.Body.String())
	}
}
