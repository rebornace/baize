package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func testArtifactStore(t *testing.T) artifact.Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "b.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fs, err := artifact.NewFileStore(filepath.Join(dir, "artifacts"), st)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestGetArtifactOK(t *testing.T) {
	mem := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})
	srv.Artifacts = testArtifactStore(t)
	srv.OperatorToken = "op-secret"
	h := srv.Handler()

	run, err := mem.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	runID := run.ID

	html := "<html><body><h1>Analysis</h1></body></html>"
	artID, err := srv.Artifacts.PutHTML(runID, html)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/artifacts/"+artID, nil)
	req.Header.Set("Authorization", "Bearer op-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type=%q", ct)
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "script-src 'unsafe-inline'") {
		t.Fatalf("CSP=%q", csp)
	}
	if body := rr.Body.String(); !strings.Contains(body, "Analysis") {
		t.Fatalf("body=%q", body)
	}
}

func TestGetArtifactNotFound(t *testing.T) {
	mem := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})
	srv.Artifacts = testArtifactStore(t)
	srv.OperatorToken = "op-secret"
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v0/artifacts/art_missing", nil)
	req.Header.Set("Authorization", "Bearer op-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
