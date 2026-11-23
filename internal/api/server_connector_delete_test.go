package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDeleteConnectorCascadesByAdmin: an Admin token issues
// DELETE /v0/connectors/{id}. The connector, its tools (Store + Registry)
// must all be gone. A subsequent GET on the same id returns 404
// connector_not_found. Deleting a missing connector returns 404 too.
func TestDeleteConnectorCascadesByAdmin(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, reg, st, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	// Sanity: connector + its two spec tools exist.
	if _, err := st.GetConnector("c1"); err != nil {
		t.Fatalf("precondition GetConnector: %v", err)
	}
	if _, ok := reg.Get("create_ticket"); !ok {
		t.Fatal("precondition: create_ticket must be registered")
	}
	if _, ok := reg.Get("get_ticket"); !ok {
		t.Fatal("precondition: get_ticket must be registered")
	}

	// catalogServer leaves the gate off, so no Authorization header is
	// required; the ACL still maps DELETE /v0/connectors/{id} to Admin and the
	// gate-off path authorizes as Admin.
	del := httptest.NewRequest(http.MethodDelete, "/v0/connectors/c1", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, del)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE connector status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("204 must have empty body, got %q", rr.Body.String())
	}

	// Store: connector gone.
	if _, err := st.GetConnector("c1"); err == nil {
		t.Fatal("connector must be gone from Store after DELETE")
	}
	// Store: no tools belong to the connector.
	if byC := st.ListToolsByConnector("c1"); len(byC) != 0 {
		t.Fatalf("ListToolsByConnector after DELETE = %d, want 0", len(byC))
	}
	// Registry: connector's tools unregistered.
	if _, ok := reg.Get("create_ticket"); ok {
		t.Fatal("create_ticket must be unregistered from Registry after DELETE")
	}
	if _, ok := reg.Get("get_ticket"); ok {
		t.Fatal("get_ticket must be unregistered from Registry after DELETE")
	}

	// GET /v0/connectors/c1 now returns 404 connector_not_found.
	get := httptest.NewRequest(http.MethodGet, "/v0/connectors/c1", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET after DELETE status=%d want 404 body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "connector_not_found" {
		t.Fatalf("GET after DELETE code=%q want connector_not_found", wrap.Error.Code)
	}

	// DELETE missing connector → 404 connector_not_found.
	delMissing := httptest.NewRequest(http.MethodDelete, "/v0/connectors/nope", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, delMissing)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("DELETE missing status=%d want 404 body=%s", rr.Code, rr.Body.String())
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "connector_not_found" {
		t.Fatalf("DELETE missing code=%q want connector_not_found", wrap.Error.Code)
	}
}

// TestDeleteConnectorOperatorForbidden: with the control-plane gate on,
// an Operator token must be rejected (403) on DELETE /v0/connectors/{id},
// while an Admin token performs the cascade delete successfully.
func TestDeleteConnectorOperatorForbidden(t *testing.T) {
	spec := writeCatalogSpec(t)
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	h := srv.Handler()
	putConnectorAuth(t, h, "c1", spec, "http://127.0.0.1:9", "adm")

	// Operator → 403.
	op := httptest.NewRequest(http.MethodDelete, "/v0/connectors/c1", nil)
	op.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, op)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("operator DELETE connector: %d %s", rr.Code, rr.Body.String())
	}
	// Connector still present.
	if _, err := st.GetConnector("c1"); err != nil {
		t.Fatal("connector must still exist after operator 403")
	}

	// Admin → 204.
	ad := httptest.NewRequest(http.MethodDelete, "/v0/connectors/c1", nil)
	ad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, ad)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("admin DELETE connector: %d %s", rr.Code, rr.Body.String())
	}
	if _, err := st.GetConnector("c1"); err == nil {
		t.Fatal("connector must be gone after admin DELETE")
	}
}

// putConnectorAuth PUTs a connector with the given admin bearer token; used
// when the control-plane gate is on (putCatalogConnector sends no auth).
func putConnectorAuth(t *testing.T, h http.Handler, id, spec, baseURL, adminToken string) {
	t.Helper()
	body := map[string]any{
		"type":     "openapi",
		"spec":     spec,
		"base_url": baseURL,
	}
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/"+id, jsonBodyAPI(t, body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT connector %s status=%d body=%s", id, rr.Code, rr.Body.String())
	}
}
