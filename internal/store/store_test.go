package store_test

import (
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestCreateAndGetRun(t *testing.T) {
	s := store.New()
	r, err := s.CreateRun("ticket-agent", "创建工单")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID == "" || r.Status != store.StatusRunning {
		t.Fatalf("got %+v", r)
	}
	got, err := s.GetRun(r.ID)
	if err != nil || got.Input != "创建工单" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	s.AppendEvent(r.ID, store.Event{Type: "run.started"})
	evs, _ := s.ListEvents(r.ID)
	if len(evs) != 1 || evs[0].Type != "run.started" {
		t.Fatalf("events=%+v", evs)
	}
}
