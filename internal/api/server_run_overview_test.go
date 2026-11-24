package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestRunOverview(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	mk := func(status store.Status, errMsg string) {
		t.Helper()
		r, cerr := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
		if cerr != nil {
			t.Fatal(cerr)
		}
		if uerr := st.UpdateRun(r.ID, status, "", errMsg); uerr != nil {
			t.Fatal(uerr)
		}
	}
	mk(store.StatusSucceeded, "")
	mk(store.StatusFailed, "boom\nstack trace")
	mk(store.StatusRunning, "")

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/run-overview?days=7", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("overview=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Total       int            `json:"total"`
		ByStatus    map[string]int `json:"by_status"`
		SuccessRate float64        `json:"success_rate"`
		Active      int            `json:"active"`
		TopErrors   []struct {
			Message string `json:"message"`
			Count   int    `json:"count"`
		} `json:"top_errors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 3 || body.ByStatus["succeeded"] != 1 || body.ByStatus["failed"] != 1 {
		t.Fatalf("body=%+v", body)
	}
	if body.SuccessRate != 0.5 || body.Active != 1 {
		t.Fatalf("rate=%v active=%d", body.SuccessRate, body.Active)
	}
	if len(body.TopErrors) != 1 || body.TopErrors[0].Message != "boom" || body.TopErrors[0].Count != 1 {
		t.Fatalf("top_errors=%+v", body.TopErrors)
	}

	bad := httptest.NewRequest(http.MethodGet, "/v0/settings/run-overview?days=0", nil)
	bad.Header.Set("Authorization", "Bearer adm")
	br := httptest.NewRecorder()
	srv.Handler().ServeHTTP(br, bad)
	if br.Code != http.StatusBadRequest {
		t.Fatalf("bad days=%d", br.Code)
	}
}
