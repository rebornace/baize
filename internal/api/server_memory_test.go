package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func memorySettingsServer(t *testing.T) *Server {
	t.Helper()
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.AdminToken = "adm-token"
	srv.Operators = []controlplane.Operator{
		{ID: "alice", Token: "alice-token"},
		{ID: "bob", Token: "bob-token"},
	}
	srv.Memory = memory.NewMemoryStore()
	return srv
}

func withNamedOp(req *http.Request, token string) *http.Request {
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestMemorySettingsCRUDIsolation(t *testing.T) {
	srv := memorySettingsServer(t)
	h := srv.Handler()

	// alice creates a memory entry
	postRec := httptest.NewRecorder()
	postReq := withNamedOp(httptest.NewRequest(http.MethodPost, "/v0/settings/memory",
		strings.NewReader(`{"text":"喜欢绿茶","key":"tea"}`)), "alice-token")
	h.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusCreated {
		t.Fatalf("alice POST: status=%d body=%s", postRec.Code, postRec.Body.String())
	}
	var created memory.Entry
	if err := json.NewDecoder(postRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.OwnerID != "alice" || created.Text != "喜欢绿茶" {
		t.Fatalf("unexpected create payload: %+v", created)
	}

	// bob cannot delete alice's entry
	delRec := httptest.NewRecorder()
	delReq := withNamedOp(httptest.NewRequest(http.MethodDelete, "/v0/settings/memory/"+created.ID, nil), "bob-token")
	h.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusForbidden {
		t.Fatalf("bob DELETE alice entry: want 403, got %d body=%s", delRec.Code, delRec.Body.String())
	}

	// alice list sees it; bob list is empty / isolated
	aliceList := httptest.NewRecorder()
	h.ServeHTTP(aliceList, withNamedOp(httptest.NewRequest(http.MethodGet, "/v0/settings/memory", nil), "alice-token"))
	if aliceList.Code != http.StatusOK {
		t.Fatalf("alice GET: status=%d body=%s", aliceList.Code, aliceList.Body.String())
	}
	var aliceItems struct {
		Items []memory.Entry `json:"items"`
	}
	if err := json.NewDecoder(aliceList.Body).Decode(&aliceItems); err != nil {
		t.Fatalf("decode alice list: %v", err)
	}
	if len(aliceItems.Items) != 1 || aliceItems.Items[0].ID != created.ID {
		t.Fatalf("alice list want 1 own entry, got %+v", aliceItems.Items)
	}

	bobList := httptest.NewRecorder()
	h.ServeHTTP(bobList, withNamedOp(httptest.NewRequest(http.MethodGet, "/v0/settings/memory", nil), "bob-token"))
	if bobList.Code != http.StatusOK {
		t.Fatalf("bob GET: status=%d body=%s", bobList.Code, bobList.Body.String())
	}
	var bobItems struct {
		Items []memory.Entry `json:"items"`
	}
	if err := json.NewDecoder(bobList.Body).Decode(&bobItems); err != nil {
		t.Fatalf("decode bob list: %v", err)
	}
	if len(bobItems.Items) != 0 {
		t.Fatalf("bob list must be empty, got %+v", bobItems.Items)
	}

	// alice can delete own entry
	okDel := httptest.NewRecorder()
	h.ServeHTTP(okDel, withNamedOp(httptest.NewRequest(http.MethodDelete, "/v0/settings/memory/"+created.ID, nil), "alice-token"))
	if okDel.Code != http.StatusOK {
		t.Fatalf("alice DELETE: status=%d body=%s", okDel.Code, okDel.Body.String())
	}
}

func TestMemorySettingsPatchOwnEntry(t *testing.T) {
	srv := memorySettingsServer(t)
	h := srv.Handler()

	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, withNamedOp(httptest.NewRequest(http.MethodPost, "/v0/settings/memory",
		strings.NewReader(`{"text":"旧文"}`)), "alice-token"))
	if postRec.Code != http.StatusCreated {
		t.Fatalf("POST: %d %s", postRec.Code, postRec.Body.String())
	}
	var created memory.Entry
	_ = json.NewDecoder(postRec.Body).Decode(&created)

	patchRec := httptest.NewRecorder()
	h.ServeHTTP(patchRec, withNamedOp(httptest.NewRequest(http.MethodPatch, "/v0/settings/memory/"+created.ID,
		strings.NewReader(`{"text":"新文","key":"k1"}`)), "alice-token"))
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH: %d %s", patchRec.Code, patchRec.Body.String())
	}
	var updated memory.Entry
	if err := json.NewDecoder(patchRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if updated.Text != "新文" || updated.Key != "k1" || updated.OwnerID != "alice" {
		t.Fatalf("unexpected patch result: %+v", updated)
	}

	// bob cannot patch alice's entry
	bobPatch := httptest.NewRecorder()
	h.ServeHTTP(bobPatch, withNamedOp(httptest.NewRequest(http.MethodPatch, "/v0/settings/memory/"+created.ID,
		strings.NewReader(`{"text":"劫持"}`)), "bob-token"))
	if bobPatch.Code != http.StatusForbidden {
		t.Fatalf("bob PATCH: want 403, got %d body=%s", bobPatch.Code, bobPatch.Body.String())
	}
}

func TestMemorySettingsOperatorACL(t *testing.T) {
	srv := memorySettingsServer(t)
	h := srv.Handler()

	// unauthenticated → 401
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/settings/memory", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: want 401, got %d", rec.Code)
	}

	// operator allowed
	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, withNamedOp(httptest.NewRequest(http.MethodGet, "/v0/settings/memory", nil), "alice-token"))
	if ok.Code != http.StatusOK {
		t.Fatalf("operator GET: want 200, got %d body=%s", ok.Code, ok.Body.String())
	}
}
