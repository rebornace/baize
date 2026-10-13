package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestMCPExportKeyIdentitySlotsCRUD(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	createIdentity := func(name string) string {
		t.Helper()
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/settings/mcp-export/identities",
			jsonBody(t, map[string]any{"name": name})))
		if rr.Code != http.StatusCreated {
			t.Fatalf("create identity %s: %d %s", name, rr.Code, rr.Body.String())
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.ID == "" {
			t.Fatalf("parse identity: %v body=%s", err, rr.Body.String())
		}
		return body.ID
	}
	defID := createIdentity("Default")
	mallID := createIdentity("Mall")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/settings/mcp-export/keys",
		jsonBody(t, map[string]any{
			"name":        "multi",
			"identity_id": defID,
			"identity_slots": []map[string]any{
				{"connector_id": "mall", "identity_id": mallID},
			},
			"allowed_tools": []string{"t1"},
		})))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create key: %d %s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID            string                       `json:"id"`
		IdentityID    string                       `json:"identity_id"`
		IdentitySlots []store.MCPExportIdentitySlot `json:"identity_slots"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.IdentityID != defID {
		t.Fatalf("identity_id=%q", created.IdentityID)
	}
	if len(created.IdentitySlots) < 2 {
		t.Fatalf("expected default+mall slots, got %+v", created.IdentitySlots)
	}

	got, err := st.GetMCPExportKey(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := store.ResolveExportIdentityID(got, "mall", "")
	if !ok || id != mallID {
		t.Fatalf("resolve mall=%q ok=%v", id, ok)
	}

	patchBody, _ := json.Marshal(map[string]any{
		"identity_slots": []map[string]any{
			{"identity_id": defID},
			{"connector_id": "crm", "role": "admin", "identity_id": mallID},
		},
	})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/v0/settings/mcp-export/keys/"+created.ID,
		strings.NewReader(string(patchBody))))
	if rr.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rr.Code, rr.Body.String())
	}
	got, _ = st.GetMCPExportKey(created.ID)
	id, ok = store.ResolveExportIdentityID(got, "crm", "admin")
	if !ok || id != mallID {
		t.Fatalf("resolve crm admin=%q ok=%v slots=%+v", id, ok, got.IdentitySlots)
	}
}
