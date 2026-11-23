package mockproject_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mockproject "github.com/rebornace/baize/examples/mock-project"
)

func TestProjectLifecycle(t *testing.T) {
	h := mockproject.NewHandler()
	list := func() []mockproject.Project {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("list status=%d", rec.Code)
		}
		var out []mockproject.Project
		if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := list(); len(got) != 2 || got[0].ID != "proj-1" {
		t.Fatalf("seed=%+v", got)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(`{"title":"扩展语料项目"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created mockproject.Project
	_ = json.NewDecoder(rec.Body).Decode(&created)
	if created.Title != "扩展语料项目" || created.Status != "active" || created.ID == "" {
		t.Fatalf("created=%+v", created)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/projects/proj-1", strings.NewReader(`{"status":"archived"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := list(); got[0].Status != "archived" {
		t.Fatalf("patched=%+v", got[0])
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/projects/proj-1", strings.NewReader(`{"status":"bogus"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d", rec.Code)
	}
}

func TestProjectNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	mockproject.NewHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
}
