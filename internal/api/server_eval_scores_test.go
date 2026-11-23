package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestEvalScoreHistoryDayRange(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	now := time.Now().UTC()
	id := "ec_range_" + now.Format("20060102150405")
	if err := st.UpsertEvalCase(store.EvalCase{
		ID: id, Status: store.EvalCaseConfirmed, Protocol: "V1",
		EventsJSON: []byte("[]"), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, rec := range []store.EvalScoreRecord{
		{CaseID: id, Pass: true, ScoredAt: now.Add(-time.Hour)},
		{CaseID: id, Pass: true, ScoredAt: now.Add(-48 * time.Hour)},
		{CaseID: id, Pass: false, ScoredAt: now.AddDate(0, 0, -10)},
	} {
		if err := st.AppendEvalScore(rec); err != nil {
			t.Fatal(err)
		}
	}
	h := srv.Handler()
	fetch := func(query string) (int, struct {
		Items []struct {
			Pass bool `json:"pass"`
		} `json:"items"`
		HistoryCap int `json:"history_cap"`
	}) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet,
			"/v0/settings/eval/cases/"+id+"/scores"+query, nil)
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var body struct {
			Items []struct {
				Pass bool `json:"pass"`
			} `json:"items"`
			HistoryCap int `json:"history_cap"`
		}
		if rr.Code == http.StatusOK {
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
		}
		return rr.Code, body
	}

	code, all := fetch("")
	if code != http.StatusOK || len(all.Items) != 3 || all.HistoryCap != store.EvalScoreHistoryCap {
		t.Fatalf("all=%d %+v", code, all)
	}
	code, week := fetch("?days=7&limit=100")
	if code != http.StatusOK || len(week.Items) != 2 {
		t.Fatalf("week=%d %+v", code, week)
	}
	code, one := fetch("?days=7&limit=1")
	if code != http.StatusOK || len(one.Items) != 1 {
		t.Fatalf("limit=%d %+v", code, one)
	}
	if code, _ := fetch("?days=0"); code != http.StatusBadRequest {
		t.Fatalf("days=0 code=%d", code)
	}
	if code, _ := fetch("?days=9999"); code != http.StatusBadRequest {
		t.Fatalf("days=9999 code=%d", code)
	}
}
