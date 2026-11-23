package store

import (
	"os"
	"path/filepath"
	"testing"
)

func hitlClaimCase(t *testing.T, st Store) {
	t.Helper()
	r, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "hitl", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(r.ID, StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	ok, err := st.BeginHITLResume(r.ID)
	if err != nil || !ok {
		t.Fatalf("begin: ok=%v err=%v", ok, err)
	}
	if ok, err := st.BeginHITLResume(r.ID); err != nil || ok {
		t.Fatalf("second begin: ok=%v err=%v want false", ok, err)
	}
	if ok, err := st.ExpireRunIfWaiting(r.ID, "expired"); err != nil || ok {
		t.Fatalf("expire while claimed: ok=%v err=%v want false", ok, err)
	}
	if err := st.ReleaseHITLResume(r.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetRun(r.ID); got.Status != StatusWaitingHuman {
		t.Fatalf("after release status=%s", got.Status)
	}
	if ok, err := st.ExpireRunIfWaiting(r.ID, "expired"); err != nil || !ok {
		t.Fatalf("expire: ok=%v err=%v", ok, err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != StatusCancelled || got.Error != "expired" {
		t.Fatalf("expired run=%+v", got)
	}
	if ok, err := st.BeginHITLResume(r.ID); err != nil || ok {
		t.Fatalf("begin after expire: ok=%v err=%v want false", ok, err)
	}

	r2, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "fail", ConversationID: "c2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(r2.ID, StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	if ok, err := st.FailRunIfWaiting(r2.ID, "repair"); err != nil || !ok {
		t.Fatalf("fail waiting: ok=%v err=%v", ok, err)
	}
	got2, _ := st.GetRun(r2.ID)
	if got2.Status != StatusFailed || got2.Error != "repair" {
		t.Fatalf("failed run=%+v", got2)
	}
	if ok, err := st.FailRunIfWaiting(r2.ID, "again"); err != nil || ok {
		t.Fatalf("second fail: ok=%v err=%v want false", ok, err)
	}
}

func TestHITLClaimMemory(t *testing.T) {
	hitlClaimCase(t, NewMemory())
}

func TestHITLClaimSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "hitl_claim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hitlClaimCase(t, st)
}

func TestHITLClaimPostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hitlClaimCase(t, st)
}
