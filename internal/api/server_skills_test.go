package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func skillsServer(t *testing.T) (*Server, store.Store, http.Handler, *skill.Catalog) {
	t.Helper()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	mustWriteSkillDir(t, filepath.Join(builtin, "builtin-skill"), "builtin-skill", "from-builtin", []string{"a"})
	cat, err := skill.LoadCatalog([]string{builtin}, user)
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.SkillCatalog = cat
	return srv, st, srv.Handler(), cat
}

func mustWriteSkillDir(t *testing.T, dir, name, desc string, tools []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("---\nname: " + name + "\ndescription: " + desc + "\ntools:\n")
	for _, x := range tools {
		b.WriteString("  - " + x + "\n")
	}
	b.WriteString("---\n\nbody for " + name + "\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillMDBytes(name, desc string, tools []string) []byte {
	var b strings.Builder
	b.WriteString("---\nname: " + name + "\ndescription: " + desc + "\ntools:\n")
	for _, x := range tools {
		b.WriteString("  - " + x + "\n")
	}
	b.WriteString("---\n\nbody for " + name + "\n")
	return []byte(b.String())
}

func multipartFile(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func zipSkillBytes(t *testing.T, name, desc string, tools []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(skillMDBytes(name, desc, tools)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSkillsUploadMDListGet(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "user-demo", []string{"t1"}))
	req := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	req.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST md status=%d body=%s", rr.Code, rr.Body.String())
	}
	var posted map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &posted); err != nil {
		t.Fatal(err)
	}
	if posted["id"] != "demo" || posted["source"] != "user" {
		t.Fatalf("posted=%v", posted)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/skills", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var list struct {
		Skills []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var foundUser, foundBuiltin bool
	for _, s := range list.Skills {
		if s.ID == "demo" && s.Source == "user" {
			foundUser = true
		}
		if s.ID == "builtin-skill" && s.Source == "builtin" {
			foundBuiltin = true
		}
	}
	if !foundUser || !foundBuiltin {
		t.Fatalf("list=%+v", list.Skills)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/skills/demo", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET skill status=%d body=%s", rr.Code, rr.Body.String())
	}
	var detail map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	bodyStr, _ := detail["body"].(string)
	if !strings.Contains(bodyStr, "body for demo") {
		t.Fatalf("detail body=%v", detail["body"])
	}
}

func TestSkillsUploadZip(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	body, ctype := multipartFile(t, "file", "pack.zip", zipSkillBytes(t, "zip-skill", "from-zip", []string{"z1"}))
	req := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	req.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST zip status=%d body=%s", rr.Code, rr.Body.String())
	}
	var posted map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &posted); err != nil {
		t.Fatal(err)
	}
	if posted["id"] != "zip-skill" || posted["source"] != "user" {
		t.Fatalf("posted=%v", posted)
	}
}

func TestSkillsDeleteBuiltin400(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/v0/skills/builtin-skill", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("DELETE builtin status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid_request") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestSkillsDeleteUserStripsAgentSkills(t *testing.T) {
	_, st, h, _ := skillsServer(t)
	body, ctype := multipartFile(t, "file", "u.md", skillMDBytes("user-skill", "u", []string{"t"}))
	req := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	req.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", rr.Code, rr.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/agents/ticket-agent",
		strings.NewReader(`{"system":"sys","skills":["user-skill","keep-me"]}`))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT agent status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/v0/skills/user-skill", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE user status=%d body=%s", rr.Code, rr.Body.String())
	}

	ag, err := st.GetAgent("ticket-agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(ag.Skills) != 1 || ag.Skills[0] != "keep-me" {
		t.Fatalf("agent skills after delete: %+v", ag.Skills)
	}
}

func TestSkillsOperatorWritesForbidden(t *testing.T) {
	srv, _, _, cat := skillsServer(t)
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"
	srv.SkillCatalog = cat
	h := srv.Handler()
	auth := func(r *http.Request) { r.Header.Set("Authorization", "Bearer op") }

	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	req := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	req.Header.Set("Content-Type", ctype)
	auth(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("operator POST skills want 403, got %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/v0/skills/builtin-skill", nil)
	auth(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("operator DELETE skills want 403, got %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/v0/agents/a1",
		strings.NewReader(`{"system":"x","skills":["s1"]}`))
	req.Header.Set("Content-Type", "application/json")
	auth(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("operator PUT agents want 403, got %d %s", rr.Code, rr.Body.String())
	}
}

func TestAgentGetPutSkills(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	put := httptest.NewRequest(http.MethodPut, "/v0/agents/a1",
		strings.NewReader(`{"system":"hello","skills":["s1","s2"]}`))
	put.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
	}
	var putBody store.Agent
	if err := json.Unmarshal(rr.Body.Bytes(), &putBody); err != nil {
		t.Fatal(err)
	}
	if putBody.System != "hello" || len(putBody.Skills) != 2 {
		t.Fatalf("put body=%+v", putBody)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/agents/a1", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got store.Agent
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "a1" || got.System != "hello" || len(got.Skills) != 2 || got.Skills[0] != "s1" {
		t.Fatalf("got=%+v", got)
	}
}

func TestSkillsGetNotFound(t *testing.T) {
	_, _, h, _ := skillsServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/skills/missing", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
