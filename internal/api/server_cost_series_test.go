package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func TestCostSeriesAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "series", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	day1 := today.AddDate(0, 0, -2)
	day2 := today.AddDate(0, 0, -1)
	mk := func(created time.Time, cost float64) {
		t.Helper()
		r, err := st.CreateRun(store.CreateRunInput{
			AgentID: "a", Input: "x", ConversationID: "c-series",
			ModelProfileID: profile.ID, CreatedAt: created.Add(12 * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		if err := st.PutRunCost(store.RunCost{
			RunID: r.ID, Day: created.Format("2006-01-02"), CostUSD: cost, Priced: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk(day1, 1)
	mk(day2, 2.5)

	req := httptest.NewRequest(http.MethodGet,
		"/v0/settings/cost-series?days=7&conversation_id=c-series", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("series=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		ConversationID string  `json:"conversation_id"`
		Total          float64 `json:"total_usd"`
		Series         []struct {
			Date     string  `json:"date"`
			TotalUSD float64 `json:"total_usd"`
		} `json:"series"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ConversationID != "c-series" || len(body.Series) != 7 || body.Total != 3.5 {
		t.Fatalf("body=%+v", body)
	}
	byDate := map[string]float64{}
	for _, d := range body.Series {
		byDate[d.Date] = d.TotalUSD
	}
	if byDate[day1.Format("2006-01-02")] != 1 || byDate[day2.Format("2006-01-02")] != 2.5 {
		t.Fatalf("series=%+v", body.Series)
	}
}

func TestCostSeriesRejectsBadRequest(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	for _, path := range []string{
		"/v0/settings/cost-series",
		"/v0/settings/cost-series?conversation_id=c1&workspace_id=default",
		"/v0/settings/cost-series?conversation_id=c1&days=0",
		"/v0/settings/cost-series?conversation_id=c1&days=91",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s=%d", path, rr.Code)
		}
	}
}

func TestWorkspaceCostSeriesAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	lab, err := ms.CreateWorkspace("Lab")
	if err != nil {
		t.Fatal(err)
	}
	if err := ms.EnsureMeta(conversation.Meta{ID: "c-in", WorkspaceID: lab.ID, Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "ws-series", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	mk := func(conv string, cost float64) {
		t.Helper()
		r, err := st.CreateRun(store.CreateRunInput{
			AgentID: "a", Input: "x", ConversationID: conv,
			ModelProfileID: profile.ID, CreatedAt: day.Add(12 * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		if err := st.PutRunCost(store.RunCost{
			RunID: r.ID, Day: day.Format("2006-01-02"), CostUSD: cost, Priced: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("c-in", 1.5)
	mk("c-out", 2.5)

	req := httptest.NewRequest(http.MethodGet,
		"/v0/settings/cost-series?days=7&workspace_id="+lab.ID, nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("series=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Scope  string  `json:"scope"`
		Total  float64 `json:"total_usd"`
		Series []struct {
			Date     string  `json:"date"`
			TotalUSD float64 `json:"total_usd"`
		} `json:"series"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Scope != "workspace" || body.Total != 1.5 {
		t.Fatalf("body=%+v", body)
	}
	byDate := map[string]float64{}
	for _, d := range body.Series {
		byDate[d.Date] = d.TotalUSD
	}
	if byDate[day.Format("2006-01-02")] != 1.5 {
		t.Fatalf("series=%+v", body.Series)
	}
}
