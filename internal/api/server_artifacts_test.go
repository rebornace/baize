package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/file"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func testArtifactStore(t *testing.T) artifact.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.OpenSQLite(filepath.Join(dir, "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	blobs, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	as, err := artifact.NewStore(blobs, st)
	if err != nil {
		t.Fatal(err)
	}
	return as
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
	artID, err := srv.Artifacts.PutHTML(context.Background(), runID, html)
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

// failingArtifactStore 的 Get 总是返回一个非 not-found 的普通错误（模拟
// S3 网络/权限/5xx 故障）。handler 必须返回 5xx，而不是误报 404。
type failingArtifactStore struct {
	artifact.Store
}

func (f *failingArtifactStore) Get(_ context.Context, _ string) (string, string, error) {
	return "", "", errors.New("s3 upstream: 503 Service Unavailable")
}

func TestGetArtifactUpstreamErrorReturns5xx(t *testing.T) {
	mem := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})
	srv.Artifacts = &failingArtifactStore{}
	srv.OperatorToken = "op-secret"
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v0/artifacts/art_whatever", nil)
	req.Header.Set("Authorization", "Bearer op-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Fatalf("upstream failure must not be reported as 404; body=%s", rr.Body.String())
	}
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
