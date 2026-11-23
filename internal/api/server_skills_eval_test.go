package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func validEvalJSON() string {
	return `{
		"harness": "evidence-eval",
		"cases": [{
			"id": "c1",
			"protocol": "V0",
			"input": "list tickets"
		}]
	}`
}

func TestSkillEvalGetEmptyPutGet(t *testing.T) {
	_, _, h, _ := skillsServer(t)

	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "user-demo", []string{"t1"}))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST skill status=%d body=%s", rr.Code, rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/skills/demo/eval", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET empty eval status=%d body=%s", rr.Code, rr.Body.String())
	}
	var empty skill.EvalDoc
	if err := json.Unmarshal(rr.Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Harness != "evidence-eval" || len(empty.Cases) != 0 {
		t.Fatalf("empty eval=%+v", empty)
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(validEvalJSON()))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT eval status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/skills/demo/eval", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET eval status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got skill.EvalDoc
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Cases) != 1 || got.Cases[0].ID != "c1" || got.Cases[0].Input != "list tickets" {
		t.Fatalf("got eval=%+v", got)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v0/skills", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status=%d", listRR.Code)
	}
	var list struct {
		Skills []struct {
			ID            string `json:"id"`
			EvalCaseCount int    `json:"eval_case_count"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var demoCount int
	for _, s := range list.Skills {
		if s.ID == "demo" {
			demoCount = s.EvalCaseCount
		}
	}
	if demoCount != 1 {
		t.Fatalf("demo eval_case_count=%d want 1", demoCount)
	}

	badHarness := strings.Replace(validEvalJSON(), "evidence-eval", "other-harness", 1)
	put = httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(badHarness))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT bad harness status=%d body=%s", rr.Code, rr.Body.String())
	}

	put = httptest.NewRequest(http.MethodPut, "/v0/skills/builtin-skill/eval", strings.NewReader(validEvalJSON()))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT builtin eval status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSkillEvalDeleteClearsCases(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(validEvalJSON()))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	del := httptest.NewRequest(http.MethodDelete, "/v0/skills/demo/eval", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, del)
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE eval status=%d body=%s", rr.Code, rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/skills/demo/eval", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET after delete status=%d", rr.Code)
	}
	var doc skill.EvalDoc
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) != 0 {
		t.Fatalf("cases after delete=%+v", doc.Cases)
	}
}

func TestSkillEvalRequiresAdmin(t *testing.T) {
	srv, _, _, cat := skillsServer(t)
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"
	srv.SkillCatalog = cat
	h := srv.Handler()
	authOp := func(r *http.Request) { r.Header.Set("Authorization", "Bearer op") }
	authAdm := func(r *http.Request) { r.Header.Set("Authorization", "Bearer adm") }

	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	authAdm(post)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin POST skill status=%d body=%s", rr.Code, rr.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/skills/demo/eval", nil)
	authOp(get)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("operator GET eval want 200, got %d %s", rr.Code, rr.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(validEvalJSON()))
	put.Header.Set("Content-Type", "application/json")
	authOp(put)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("operator PUT eval want 403, got %d %s", rr.Code, rr.Body.String())
	}

	del := httptest.NewRequest(http.MethodDelete, "/v0/skills/demo/eval", nil)
	authOp(del)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, del)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("operator DELETE eval want 403, got %d %s", rr.Code, rr.Body.String())
	}
}

func TestSkillGetDetailEvalCaseCount(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/skills/demo", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET skill status=%d body=%s", rr.Code, rr.Body.String())
	}
	var detail map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if count, _ := detail["eval_case_count"].(float64); int(count) != 0 {
		t.Fatalf("eval_case_count before PUT=%v want 0", detail["eval_case_count"])
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(validEvalJSON()))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT eval status=%d body=%s", rr.Code, rr.Body.String())
	}

	get = httptest.NewRequest(http.MethodGet, "/v0/skills/demo", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET skill status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if count, _ := detail["eval_case_count"].(float64); int(count) != 1 {
		t.Fatalf("eval_case_count after PUT=%v want 1", detail["eval_case_count"])
	}
}

func TestSkillEvalPutRejectsUnknownCaseField(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	raw := `{"harness":"evidence-eval","cases":[{"id":"c1","protocol":"V0","input":"hi","extra_in_case":true}]}`
	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(raw))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT unknown case field status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSkillEvalPutManagedSkill400(t *testing.T) {
	_, _, h, _ := skillsServerWithManaged(t)
	put := httptest.NewRequest(http.MethodPut, "/v0/skills/login-auth/eval", strings.NewReader(validEvalJSON()))
	put.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT managed eval status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid_request") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func putManagedSkillBlob(t *testing.T, blobs blob.Store, name, connectorID string, tools []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\nname: " + name + "\ndescription: connector " + connectorID + " login\ntools:\n")
	for _, x := range tools {
		b.WriteString("  - " + x + "\n")
	}
	b.WriteString("managed: true\nmanaged_kind: connector_login\nmanaged_connector_id: " + connectorID + "\n")
	b.WriteString("---\n\nbody\n")
	key := blob.SkillObjectKey("managed", name, "SKILL.md")
	if err := blobs.Put(context.Background(), key, []byte(b.String()), "text/markdown"); err != nil {
		t.Fatal(err)
	}
}

func skillsServerWithManaged(t *testing.T) (*Server, store.Store, http.Handler, *skill.Catalog) {
	t.Helper()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	mustWriteSkillDir(t, filepath.Join(builtin, "builtin-skill"), "builtin-skill", "from-builtin", []string{"a"})
	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	putManagedSkillBlob(t, blobs, "login-auth", "auth", []string{"phoneLogin"})
	cat, err := skill.LoadCatalog([]string{builtin}, user, blobs)
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.SkillCatalog = cat
	srv.Blobs = blobs
	return srv, st, srv.Handler(), cat
}

func TestSkillEvalPutRejectsUnknownFields(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	raw := `{"harness":"evidence-eval","cases":[],"extra":true}`
	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(raw))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT unknown field status=%d body=%s", rr.Code, rr.Body.String())
	}
}
