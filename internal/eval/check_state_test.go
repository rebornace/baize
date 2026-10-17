package eval_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/store"
)

type upsertTrackingStore struct {
	store.Store
	upsertCalls      int
	failNextUpsert   bool
	nextUpsertErr    error
}

func (s *upsertTrackingStore) UpsertEvalCase(c store.EvalCase) error {
	s.upsertCalls++
	if s.failNextUpsert && s.nextUpsertErr != nil {
		s.failNextUpsert = false
		return s.nextUpsertErr
	}
	return s.Store.UpsertEvalCase(c)
}

func confirmedListOnlyCase(t *testing.T, st store.Store) store.EvalCase {
	t.Helper()
	r, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a",
		Input:   "只查询工单列表，了解当前有哪些工单。不要新建工单，也不要改任何工单状态。",
	})
	for _, ev := range loadFixtureEvents(t, "EV-V0-list-only.pass.events.json") {
		_ = st.AppendEvent(r.ID, ev)
	}
	c, err := eval.IngestFromRun(st, eval.IngestInput{
		RunID:         r.ID,
		Protocol:      "V0",
		RequiredReads: []string{"list_tickets"},
		Consequential: []string{"create_ticket", "update_ticket_status"},
	})
	if err != nil || c.Status != store.EvalCaseConfirmed {
		t.Fatalf("ingest: %+v %v", c, err)
	}
	return c
}

func TestCheckStateRequiresMockBase(t *testing.T) {
	st := store.NewMemory()
	c := confirmedListOnlyCase(t, st)
	_, _, err := eval.CheckState(st, c.ID, "  ", nil)
	if err != eval.ErrMockBaseRequired {
		t.Fatalf("want ErrMockBaseRequired, got %v", err)
	}
}

func TestCheckStateAtomicSingleUpsertPersistsSnapshotAndScore(t *testing.T) {
	mem := store.NewMemory()
	track := &upsertTrackingStore{Store: mem}
	st := store.Store(track)
	c := confirmedListOnlyCase(t, st)
	c, err := eval.PatchCase(st, c.ID, eval.PatchInput{
		SetExpectState: true,
		ExpectState: []store.EvalExpectState{
			{ID: "t1", TitleContains: "链路评测", Status: "closed"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tickets" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"t1","title":"链路评测","status":"closed"}]`))
	}))
	defer srv.Close()

	agree := true
	c.HumanAgree = &agree
	c.HumanNote = "looks good"
	cal := time.Now().UTC()
	c.CalibratedAt = &cal
	if err := st.UpsertEvalCase(c); err != nil {
		t.Fatal(err)
	}
	beforeUpserts := track.upsertCalls

	got, res, err := eval.CheckState(st, c.ID, srv.URL, srv.Client())
	if err != nil || !res.Pass {
		t.Fatalf("check-state: err=%v res=%+v", err, res)
	}
	if len(got.StateSnapshotJSON) == 0 || got.LastScoredAt == nil || got.LastScorePass == nil {
		t.Fatalf("atomic persist: snapshot/score missing: %+v", got)
	}
	if track.upsertCalls-beforeUpserts != 1 {
		t.Fatalf("CheckState want single UpsertEvalCase, got %d", track.upsertCalls-beforeUpserts)
	}
	if got.HumanAgree == nil || !*got.HumanAgree || got.HumanNote != "looks good" || got.CalibratedAt == nil {
		t.Fatalf("calibration cleared: %+v", got)
	}
}

func TestCheckStateUpsertFailureLeavesSnapshotUnchanged(t *testing.T) {
	mem := store.NewMemory()
	track := &upsertTrackingStore{Store: mem, nextUpsertErr: errors.New("upsert failed")}
	st := store.Store(track)
	c := confirmedListOnlyCase(t, st)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"t1","title":"x","status":"open"}]`))
	}))
	defer srv.Close()

	track.failNextUpsert = true
	_, _, err := eval.CheckState(st, c.ID, srv.URL, srv.Client())
	if err == nil {
		t.Fatal("want upsert error")
	}
	got, err := st.GetEvalCase(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.StateSnapshotJSON) != 0 || got.LastScoredAt != nil {
		t.Fatalf("store must stay unchanged on upsert failure: %+v", got)
	}
}

func TestCheckStateBadResponseDoesNotPersist(t *testing.T) {
	st := store.NewMemory()
	c := confirmedListOnlyCase(t, st)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"unexpected":true}`))
	}))
	defer srv.Close()

	_, _, err := eval.CheckState(st, c.ID, srv.URL, srv.Client())
	if err == nil {
		t.Fatal("want error")
	}
	got, err := st.GetEvalCase(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.StateSnapshotJSON) != 0 {
		t.Fatalf("snapshot must not be written: %q", got.StateSnapshotJSON)
	}
}
