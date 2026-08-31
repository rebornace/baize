package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func modelProfilesServer(t *testing.T) *Server {
	t.Helper()
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.OperatorToken = "op-token"
	srv.AdminToken = "adm-token"
	return srv
}

func TestModelProfilesCRUDPermissionsAndRedaction(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()

	// operator can read (empty list ok), cannot create
	opReq := withOperator(httptest.NewRequest(http.MethodGet, "/v0/settings/models", nil))
	opRR := httptest.NewRecorder()
	h.ServeHTTP(opRR, opReq)
	if opRR.Code != http.StatusOK {
		t.Fatalf("operator GET models: code=%d body=%s", opRR.Code, opRR.Body.String())
	}

	createBody := `{"name":"主力","provider":"openai_compatible","base_url":"https://x/v1","model":"m1","api_key":"sk-secret-1234"}`
	opPost := withOperator(httptest.NewRequest(http.MethodPost, "/v0/settings/models", strings.NewReader(createBody)))
	opPost.Header.Set("Content-Type", "application/json")
	opPostRR := httptest.NewRecorder()
	h.ServeHTTP(opPostRR, opPost)
	if opPostRR.Code != http.StatusForbidden {
		t.Fatalf("operator POST must be forbidden, got %d", opPostRR.Code)
	}

	// admin creates
	adPost := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models", strings.NewReader(createBody)))
	adPost.Header.Set("Content-Type", "application/json")
	adRR := httptest.NewRecorder()
	h.ServeHTTP(adRR, adPost)
	if adRR.Code != http.StatusCreated {
		t.Fatalf("admin create: code=%d body=%s", adRR.Code, adRR.Body.String())
	}
	var created struct {
		Profile store.ModelProfile `json:"profile"`
	}
	if err := json.Unmarshal(adRR.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Profile.ID == "" {
		t.Fatal("created profile missing id")
	}
	if created.Profile.APIKey == "sk-secret-1234" {
		t.Fatal("create response must not echo raw api_key")
	}

	// list redacts key
	listReq := withAdmin(httptest.NewRequest(http.MethodGet, "/v0/settings/models", nil))
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list: code=%d body=%s", listRR.Code, listRR.Body.String())
	}
	if strings.Contains(listRR.Body.String(), "sk-secret-1234") {
		t.Fatalf("list must not contain raw api_key: %s", listRR.Body.String())
	}

	// set default, then deleting default is rejected
	defReq := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models/"+created.Profile.ID+"/default", nil))
	defRR := httptest.NewRecorder()
	h.ServeHTTP(defRR, defReq)
	if defRR.Code != http.StatusOK {
		t.Fatalf("set default: code=%d body=%s", defRR.Code, defRR.Body.String())
	}
	delReq := withAdmin(httptest.NewRequest(http.MethodDelete, "/v0/settings/models/"+created.Profile.ID, nil))
	delRR := httptest.NewRecorder()
	h.ServeHTTP(delRR, delReq)
	if delRR.Code != http.StatusBadRequest {
		t.Fatalf("deleting default must be 400, got %d body=%s", delRR.Code, delRR.Body.String())
	}
}

func TestModelProfilesRejectMissingCredentials(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()
	body := `{"name":"nokey","provider":"openai_compatible","base_url":"https://x/v1","model":"m1"}`
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models", strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("profile without api_key/api_key_env must be 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestModelProfilesRejectUnsupportedProvider(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()
	body := `{"name":"p","provider":"anthropic","base_url":"https://x/v1","model":"m1","api_key":"k"}`
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models", strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unsupported provider must be 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestModelProfilesPatchFieldLevelMerge(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()

	createBody := `{"name":"vision","base_url":"https://x/v1","model":"m1","api_key":"sk-env-key-9999","api_key_env":"MY_KEY","supports_vision":true}`
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models", strings.NewReader(createBody)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		Profile store.ModelProfile `json:"profile"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created.Profile.ID
	if !created.Profile.SupportsVision || created.Profile.APIKeyEnv != "MY_KEY" {
		t.Fatalf("create did not persist vision/env: %+v", created.Profile)
	}
	wantRedacted := store.RedactAPIKey("sk-env-key-9999")
	if created.Profile.APIKey != wantRedacted {
		t.Fatalf("create redaction mismatch: got %q want %q", created.Profile.APIKey, wantRedacted)
	}

	// set default (irrelevant to merge but exercises the path)
	defReq := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models/"+id+"/default", nil))
	defRR := httptest.NewRecorder()
	h.ServeHTTP(defRR, defReq)
	if defRR.Code != http.StatusOK {
		t.Fatalf("set default: code=%d", defRR.Code)
	}

	// Partial update: only model changes. Booleans and api_key_env must survive.
	patchBody := `{"model":"m2"}`
	patchReq := withAdmin(httptest.NewRequest(http.MethodPatch, "/v0/settings/models/"+id, strings.NewReader(patchBody)))
	patchReq.Header.Set("Content-Type", "application/json")
	patchRR := httptest.NewRecorder()
	h.ServeHTTP(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("patch: code=%d body=%s", patchRR.Code, patchRR.Body.String())
	}
	var patched struct {
		Profile store.ModelProfile `json:"profile"`
	}
	if err := json.Unmarshal(patchRR.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Profile.Model != "m2" {
		t.Fatalf("model not updated: %q", patched.Profile.Model)
	}
	if !patched.Profile.SupportsVision {
		t.Fatal("supports_vision was reset to false by partial patch")
	}
	if patched.Profile.DisableThinking {
		t.Fatal("disable_thinking should remain false")
	}
	if patched.Profile.APIKeyEnv != "MY_KEY" {
		t.Fatalf("api_key_env was wiped: %q", patched.Profile.APIKeyEnv)
	}
	if patched.Profile.APIKey != wantRedacted {
		t.Fatalf("api_key redaction changed: got %q want %q", patched.Profile.APIKey, wantRedacted)
	}
	if patched.Profile.Name != "vision" || patched.Profile.BaseURL != "https://x/v1" {
		t.Fatalf("untouched fields changed: name=%q base_url=%q", patched.Profile.Name, patched.Profile.BaseURL)
	}

	// Confirm via list as well (persisted state).
	listReq := withAdmin(httptest.NewRequest(http.MethodGet, "/v0/settings/models", nil))
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	var list struct {
		Profiles []store.ModelProfile `json:"profiles"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var got *store.ModelProfile
	for i := range list.Profiles {
		if list.Profiles[i].ID == id {
			got = &list.Profiles[i]
		}
	}
	if got == nil {
		t.Fatal("patched profile missing from list")
	}
	if got.Model != "m2" || !got.SupportsVision || got.APIKeyEnv != "MY_KEY" || got.APIKey != wantRedacted {
		t.Fatalf("persisted state wrong: %+v", got)
	}
}

func TestModelProfilesNotFoundReturns404(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()

	patchReq := withAdmin(httptest.NewRequest(http.MethodPatch, "/v0/settings/models/mp_missing", strings.NewReader(`{"model":"x"}`)))
	patchReq.Header.Set("Content-Type", "application/json")
	patchRR := httptest.NewRecorder()
	h.ServeHTTP(patchRR, patchReq)
	if patchRR.Code != http.StatusNotFound {
		t.Fatalf("patch missing must be 404, got %d", patchRR.Code)
	}

	delReq := withAdmin(httptest.NewRequest(http.MethodDelete, "/v0/settings/models/mp_missing", nil))
	delRR := httptest.NewRecorder()
	h.ServeHTTP(delRR, delReq)
	if delRR.Code != http.StatusNotFound {
		t.Fatalf("delete missing must be 404, got %d", delRR.Code)
	}

	defReq := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models/mp_missing/default", nil))
	defRR := httptest.NewRecorder()
	h.ServeHTTP(defRR, defReq)
	if defRR.Code != http.StatusNotFound {
		t.Fatalf("set-default missing must be 404, got %d", defRR.Code)
	}
}
