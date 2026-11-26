package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func TestAuditRecordsAdminMutations(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	if err := ms.EnsureMeta(conversation.Meta{ID: "c-audit", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	pin := httptest.NewRequest(http.MethodPut, "/v0/conversations/c-audit/pin",
		bytes.NewBufferString(`{"pinned":true}`))
	pin.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, pin)
	if rr.Code != http.StatusOK {
		t.Fatalf("pin=%d %s", rr.Code, rr.Body.String())
	}

	// Reads are not audited, including this listing itself.
	list := httptest.NewRequest(http.MethodGet, "/v0/audit/events", nil)
	list.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, list)
	if rr.Code != http.StatusOK {
		t.Fatalf("list=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Items []store.AuditEvent `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items=%+v", body.Items)
	}
	ev := body.Items[0]
	if ev.Action != "PUT /v0/conversations/{id}/pin" || ev.Target != "/v0/conversations/c-audit/pin" {
		t.Fatalf("event=%+v", ev)
	}
	if ev.Status != http.StatusOK || ev.Method != http.MethodPut {
		t.Fatalf("event=%+v", ev)
	}
	if ev.CreatedAt.IsZero() {
		t.Fatal("missing created_at")
	}
}

func TestAuditListingRequiresAdmin(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	req := httptest.NewRequest(http.MethodGet, "/v0/audit/events", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator list=%d", rr.Code)
	}
}

func TestAuditSkipsWorkloadTraffic(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	cases := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPost, "/v0/runs", false},
		{http.MethodPost, "/v0/runs/r1/cancel", false},
		{http.MethodPost, "/v0/inbox/x", false},
		{http.MethodPost, "/v0/channels/wecom/inbound", false},
		{http.MethodPut, "/v0/conversations/c1/read", false},
		{http.MethodGet, "/v0/conversations", false},
		{http.MethodDelete, "/v0/conversations/c1", true},
		{http.MethodPatch, "/v0/settings/models/m1", true},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if got := srv.shouldAuditRequest(req); got != tc.want {
			t.Fatalf("%s %s: got=%v want=%v", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestAuditFilterAndLimit(t *testing.T) {
	st := store.NewMemory()
	for _, action := range []string{"A", "B", "A"} {
		if err := st.AppendAuditEvent(store.AuditEvent{Action: action, Method: "POST"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.ListAuditEvents(store.AuditFilter{Action: "A", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Action != "A" {
		t.Fatalf("got=%+v", got)
	}
	all, err := st.ListAuditEvents(store.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Action != "A" {
		t.Fatalf("all=%+v", all)
	}
}

func TestAuditFilterQueryAndRange(t *testing.T) {
	st := store.NewMemory()
	t1 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	_ = st.AppendAuditEvent(store.AuditEvent{
		OperatorID: "alice", Action: "DELETE /v0/conversations/{id}",
		Target: "/v0/conversations/c1", CreatedAt: t1,
	})
	_ = st.AppendAuditEvent(store.AuditEvent{
		OperatorID: "bob", Action: "PUT /v0/settings/runtime",
		Target: "/v0/settings/runtime", CreatedAt: t2,
	})
	byQuery, _ := st.ListAuditEvents(store.AuditFilter{Query: "delete"})
	if len(byQuery) != 1 || byQuery[0].OperatorID != "alice" {
		t.Fatalf("query=%+v", byQuery)
	}
	byRange, _ := st.ListAuditEvents(store.AuditFilter{From: t1.Add(30 * time.Second)})
	if len(byRange) != 1 || byRange[0].OperatorID != "bob" {
		t.Fatalf("range=%+v", byRange)
	}
	byTo, _ := st.ListAuditEvents(store.AuditFilter{To: t1.Add(30 * time.Second)})
	if len(byTo) != 1 || byTo[0].OperatorID != "alice" {
		t.Fatalf("to=%+v", byTo)
	}
}

func TestExportAuditEventsCSV(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	if err := srv.Store.AppendAuditEvent(store.AuditEvent{
		OperatorID: "alice", Role: "admin", Method: "DELETE",
		Action: "DELETE /v0/conversations/{id}", Target: "/v0/conversations/c1", Status: 200,
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v0/audit/export?q=delete", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("export=%d %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Fatalf("content-type=%q", ct)
	}
	body := rr.Body.String()
	if !strings.HasPrefix(body, "\ufeff") {
		t.Fatal("missing BOM")
	}
	if !strings.Contains(body, "operator_id") || !strings.Contains(body, "alice") ||
		!strings.Contains(body, "DELETE /v0/conversations/{id}") {
		t.Fatalf("csv=%q", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/audit/export", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator export=%d", rr.Code)
	}
}

func TestAuditRetentionSetting(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	get := httptest.NewRequest(http.MethodGet, "/v0/settings/audit-retention", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d", rr.Code)
	}
	var body struct {
		Rows int `json:"rows"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.Rows != 5000 {
		t.Fatalf("default=%+v err=%v", body, err)
	}
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/audit-retention",
		bytes.NewBufferString(`{"rows":500}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if got := srv.auditRetentionRows(); got != 500 {
		t.Fatalf("rows=%d", got)
	}
	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/audit-retention",
		bytes.NewBufferString(`{"rows":50}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}
