package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func TestCostExportCSV(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if err := srv.Store.PutCostDay(store.CostDay{
		Day: yesterday, CostUSD: 0.42, Runs: 2, Finalized: true,
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v0/settings/cost-export?days=3", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("export=%d %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content-type=%q", ct)
	}
	disposition := rr.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "baize-cost-") {
		t.Fatalf("content-disposition=%q", disposition)
	}
	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("lines=%d: %q", len(lines), lines)
	}
	if lines[0] != "date,runs,cost_usd" {
		t.Fatalf("header=%q", lines[0])
	}
	wantDay := yesterday + ",2,0.420000"
	found := false
	for _, line := range lines {
		if line == wantDay {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing %q in %q", wantDay, lines)
	}
	if !strings.HasPrefix(lines[len(lines)-1], "total,2,0.42") {
		t.Fatalf("total=%q", lines[len(lines)-1])
	}
}

func TestCostBreakdownExportCSV(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	lab, err := ms.CreateWorkspace("Lab")
	if err != nil {
		t.Fatal(err)
	}
	if err := ms.EnsureMeta(conversation.Meta{ID: "c1", WorkspaceID: lab.ID, Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := ms.EnsureMeta(conversation.Meta{ID: "c2", WorkspaceID: lab.ID, Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "breakdown-model", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	mk := func(agentID, convID string, cost float64) {
		r, err := st.CreateRun(store.CreateRunInput{
			AgentID: agentID, Input: "x", ConversationID: convID, ModelProfileID: profile.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		if err := st.PutRunCost(store.RunCost{
			RunID: r.ID, Day: now.Format("2006-01-02"), CostUSD: cost, Priced: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("a1", "c1", 1.5)
	mk("a2", "c2", 2.5)

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/cost-export?days=7&group=breakdown", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("export=%d %s", rr.Code, rr.Body.String())
	}
	if disposition := rr.Header().Get("Content-Disposition"); !strings.Contains(disposition, "breakdown") {
		t.Fatalf("content-disposition=%q", disposition)
	}
	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	if lines[0] != "group,key,name,cost_usd,ratio" {
		t.Fatalf("header=%q", lines[0])
	}
	want := []string{
		"model," + profile.ID + ",breakdown-model,4.000000,1.000000",
		"provider,openai_compatible,openai_compatible,4.000000,1.000000",
		"agent,a2,,2.500000,0.625000",
		"agent,a1,,1.500000,0.375000",
		"conversation,c1,,1.500000,0.375000",
		"conversation,c2,,2.500000,0.625000",
		"workspace," + lab.ID + ",Lab,4.000000,1.000000",
	}
	for _, w := range want {
		found := false
		for _, line := range lines {
			if line == w {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %q in %q", w, lines)
		}
	}
}

func TestCostExportRejectsBadWindow(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	for _, q := range []string{"", "?days=0", "?days=abc", "?days=367"} {
		req := httptest.NewRequest(http.MethodGet, "/v0/settings/cost-export"+q, nil)
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if q == "" {
			if rr.Code != http.StatusOK {
				t.Fatalf("default=%d", rr.Code)
			}
			continue
		}
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s=%d", q, rr.Code)
		}
	}
	bad := httptest.NewRequest(http.MethodGet, "/v0/settings/cost-export?group=nope", nil)
	bad.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad group=%d", rr.Code)
	}
}
