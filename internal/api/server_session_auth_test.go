package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func sessionAuthServer(t *testing.T) (*Server, identity.Store, http.Handler) {
	t.Helper()
	st := store.NewMemory()
	ids := identity.NewMemoryStore()
	reg := tool.NewRegistry()
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.Identities = ids
	srv.DefaultAgentID = "default-agent"
	st.UpsertAgent(store.Agent{ID: "default-agent", System: "sys"})
	return srv, ids, srv.Handler()
}

func TestPostRunSessionTokenUpsertsIdentity(t *testing.T) {
	_, ids, h := sessionAuthServer(t)
	token := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ0ZXN0QHgLmNvbSJ9.sig"
	req := httptest.NewRequest(
		http.MethodPost,
		"/v0/runs",
		jsonBodyAPI(t, map[string]any{
			"agent_id":        "default-agent",
			"input":           "查询 step 数据",
			"conversation_id": "conv1",
			"session_token":   token,
		}),
	)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	views := ids.ListPublic("conv1")
	if len(views) != 1 {
		t.Fatalf("identities=%v", views)
	}
	if views[0].Source != identity.SourceManual {
		t.Fatalf("source=%q", views[0].Source)
	}
	if !views[0].IsDefault {
		t.Fatalf("expected default identity")
	}
}

func TestPostIdentityManualToken(t *testing.T) {
	_, ids, h := sessionAuthServer(t)
	req := httptest.NewRequest(
		http.MethodPost,
		"/v0/conversations/conv1/identities",
		jsonBodyAPI(t, map[string]any{"token": "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhIn0.sig"}),
	)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["source"] != identity.SourceManual {
		t.Fatalf("body=%v", body)
	}
	if len(ids.List("conv1")) != 1 {
		t.Fatalf("store count=%d", len(ids.List("conv1")))
	}
}
