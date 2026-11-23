package eventbus_test

import (
	"sync/atomic"
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

func TestPendingNotifications(t *testing.T) {
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	run, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}
	sub := hub.SubscribePending()
	defer sub.Cancel()
	wait := func(label string) {
		t.Helper()
		select {
		case <-sub.C:
		case <-time.After(time.Second):
			t.Fatalf("%s: no pending nudge", label)
		}
	}
	if err := st.UpdateRun(run.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetHITL(run.ID, &store.HITLPayload{Kind: store.HITLKindApproval, Prompt: "ok?"}); err != nil {
		t.Fatal(err)
	}
	wait("payload set")
	if err := st.SetHITL(run.ID, nil); err != nil {
		t.Fatal(err)
	}
	wait("payload cleared")
}

func TestPendingExternalDoesNotRepublish(t *testing.T) {
	hub := eventbus.NewHub()
	sub := hub.SubscribePending()
	defer sub.Cancel()
	var calls int32
	hub.OnPendingChange(func() { atomic.AddInt32(&calls, 1) })

	hub.PublishPendingExternal()
	select {
	case <-sub.C:
	case <-time.After(time.Second):
		t.Fatal("external pending nudge did not reach subscribers")
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("external nudge invoked local observer")
	}

	hub.PublishPending()
	select {
	case <-sub.C:
	case <-time.After(time.Second):
		t.Fatal("local pending nudge did not reach subscribers")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("local observer calls=%d want 1", calls)
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
