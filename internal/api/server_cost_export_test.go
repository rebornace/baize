package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
}
