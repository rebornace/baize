package store

import (
	"os"
	"testing"
)

func TestPostgresDriverRegistered(t *testing.T) {
	found := false
	for _, d := range ListDrivers() {
		if d == "postgres" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("postgres driver not registered")
	}
}

func TestOpenPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	st.UpsertAgent(Agent{ID: "a1", System: "sys"})
	if _, err := st.GetAgent("a1"); err != nil {
		t.Fatal(err)
	}

	run, err := st.CreateRun(CreateRunInput{AgentID: "a1", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(run.ID, StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(run.ID)
	if err != nil || got.Status != StatusSucceeded {
		t.Fatalf("run=%+v err=%v", got, err)
	}
}
