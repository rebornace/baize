package eventbus_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
)

func TestPublishExternalNotifiesSubscribers(t *testing.T) {
	hub := eventbus.NewHub()
	sub := hub.Subscribe("run_e")
	defer sub.Cancel()
	hub.PublishExternal("run_e", 3) // cross-replica nudge; seq=3
	select {
	case ev := <-sub.Events:
		if ev.Index != 3 {
			t.Fatalf("index=%d want 3", ev.Index)
		}
		if ev.Event.Type != "external.nudge" {
			t.Fatalf("type=%q want external.nudge", ev.Event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive external nudge")
	}
}

func TestNotifyAppendAndEnd(t *testing.T) {
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	run, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}
	sub := hub.Subscribe(run.ID)
	defer sub.Cancel()
	if err := st.AppendEvent(run.ID, store.Event{Type: "run.started"}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-sub.Events:
		if ev.Index != 0 || ev.Event.Type != "run.started" {
			t.Fatalf("%+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	if err := st.UpdateRun(run.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case stt := <-sub.Ended:
		if stt != store.StatusSucceeded {
			t.Fatalf("%s", stt)
		}
	case <-time.After(time.Second):
		t.Fatal("no end")
	}
}
