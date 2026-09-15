package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestPutConnectorSyncsManagedLoginSkill(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	managedDir := filepath.Join(root, "managed")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}

	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatalf("open memory blob: %v", err)
	}
	cat, err := skill.LoadCatalog(nil, userDir, managedDir, blobs)
	if err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.Identities = identity.NewMemoryStore()
	srv.DataDir = t.TempDir()
	srv.Blobs = blobs
	srv.SkillCatalog = cat
	h := srv.Handler()

	specContent := `{
  "openapi": "3.0.3",
  "info": {"title": "auth", "version": "1.0.0"},
  "paths": {
    "/login": {
      "post": {
        "operationId": "AuthController_phoneLogin",
        "responses": {"200": {"description": "ok"}}
      }
    },
    "/sms": {
      "post": {
        "operationId": "AuthController_sendSms",
        "responses": {"200": {"description": "ok"}}
      }
    }
  }
}`
	putBody := map[string]any{
		"type":          "openapi",
		"base_url":      "https://api.example.com",
		"spec_content":  specContent,
		"import_format": "openapi3",
	}
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/auth", jsonBodyAPI(t, putBody))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	skillPath := filepath.Join(managedDir, "login-auth", "SKILL.md")
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Fatalf("must not write local managedDir, stat err=%v", err)
	}
	key := blob.SkillObjectKey("managed", "login-auth", "SKILL.md")
	if _, err := blobs.Get(context.Background(), key); err != nil {
		t.Fatalf("managed package missing in blob: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v0/skills", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("GET /v0/skills status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var listBody struct {
		Skills []struct {
			ID string `json:"id"`
		} `json:"skills"`
	}
	if err := json.NewDecoder(listRR.Body).Decode(&listBody); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range listBody.Skills {
		if s.ID == "login-auth" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("GET /v0/skills missing login-auth: %+v", listBody.Skills)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/v0/connectors/auth", nil)
	delRR := httptest.NewRecorder()
	h.ServeHTTP(delRR, delReq)
	if delRR.Code != http.StatusOK && delRR.Code != http.StatusNoContent {
		t.Fatalf("DELETE status=%d body=%s", delRR.Code, delRR.Body.String())
	}
	if _, err := blobs.Get(context.Background(), key); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("managed blob should be gone after connector delete, err=%v", err)
	}
}
