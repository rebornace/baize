package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// TestCatalogPatchPreservesMutatingHITL: a connector registered with
// RequireApprovalMutating=true bakes RequireApproval=true into mutating spec
// rows. PATCHing require_login (enabled unchanged) must go through
// Registry.SetRequireLogin and NOT drop mutating HITL. Disabling then
// re-enabling the row must also restore mutating HITL via re-registration.
func TestCatalogPatchPreservesMutatingHITL(t *testing.T) {
	spec := writeCatalogSpec(t) // has POST /tickets (create_ticket) + GET /tickets/{id} (get_ticket)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	emptyLogin := []string{}
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store:      st,
		Registry:   reg,
		Identities: ids,
		ID:         "c1",
		Type:       "openapi",
		Spec:       spec,
		BaseURL:    "http://127.0.0.1:9",
		Auth: store.ConnectorAuth{
			Mode: "static",
		},
		RequireApprovalMutating: true,
		RequireLogin:           &emptyLogin,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.Identities = ids
	h := srv.Handler()

	// Sanity: create_ticket (POST) requires approval after Apply; get_ticket (GET) does not.
	if !reg.RequiresApproval("create_ticket") {
		t.Fatal("create_ticket must require approval after Apply with RequireApprovalMutating")
	}
	if reg.RequiresApproval("get_ticket") {
		t.Fatal("get_ticket (GET) must not require approval")
	}

	// PATCH require_login=true on create_ticket (enabled unchanged) → SetRequireLogin path.
	patchLogin := httptest.NewRequest(http.MethodPatch, "/v0/tools/create_ticket",
		jsonBodyAPI(t, map[string]any{"require_login": true}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, patchLogin)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH require_login status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !reg.RequiresApproval("create_ticket") {
		t.Fatal("PATCH require_login must not drop mutating HITL (SetRequireLogin path)")
	}
	if !reg.RequiresLogin("create_ticket") {
		t.Fatal("PATCH require_login must set Registry.RequiresLogin")
	}

	// PATCH enabled=false → Unregister.
	patchOff := httptest.NewRequest(http.MethodPatch, "/v0/tools/create_ticket",
		jsonBodyAPI(t, map[string]any{"enabled": false}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, patchOff)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH enabled=false status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := reg.Get("create_ticket"); ok {
		t.Fatal("create_ticket must be unregistered after PATCH enabled=false")
	}

	// PATCH enabled=true → re-register; mutating HITL must be restored from
	// the baked-in row.RequireApproval flag.
	patchOn := httptest.NewRequest(http.MethodPatch, "/v0/tools/create_ticket",
		jsonBodyAPI(t, map[string]any{"enabled": true}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, patchOn)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH enabled=true status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := reg.Get("create_ticket"); !ok {
		t.Fatal("create_ticket must be registered after PATCH enabled=true")
	}
	if !reg.RequiresApproval("create_ticket") {
		t.Fatal("re-enable must restore mutating HITL via baked-in RequireApproval")
	}
}
