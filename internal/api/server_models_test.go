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
