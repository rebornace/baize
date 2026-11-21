package schedule_test

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/schedule"
	"github.com/rebornace/baize/internal/tool"
)

func fixedNow() time.Time {
	return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
}

func ctxAlice() context.Context {
	return identity.WithConversationID(context.Background(), "conv1")
}

type stubMeta struct{ owner string }

func (s stubMeta) GetMeta(string) (string, error) { return s.owner, nil }

func setupMeta() schedule.MetaReader { return stubMeta{owner: "alice"} }

func TestValidateCreateArgsRejectsPastOnce(t *testing.T) {
	now := fixedNow()
	err := schedule.ValidateCreateArgs(map[string]any{
		"kind":        "once",
		"instruction": "喝水",
		"timezone":    "UTC",
		"fire_at":     now.Add(-time.Hour).Format(time.RFC3339),
	}, now)
	if err == nil {
		t.Fatal("want error for past fire_at")
	}
}

func TestValidateCreateArgsInMinutes(t *testing.T) {
	now := fixedNow()
	base := map[string]any{"kind": "once", "instruction": "喝水", "timezone": "UTC"}
	ok := map[string]any{"in_minutes": 30}
	for k, v := range base {
		ok[k] = v
	}
	if err := schedule.ValidateCreateArgs(ok, now); err != nil {
		t.Fatalf("in_minutes should validate: %v", err)
	}
	for _, minutes := range []int{0, -1, 43201} {
		args := map[string]any{"in_minutes": minutes}
		for k, v := range base {
			args[k] = v
		}
		if err := schedule.ValidateCreateArgs(args, now); err == nil {
			t.Fatalf("in_minutes=%d should be rejected", minutes)
		}
	}
	both := map[string]any{
		"kind": "once", "instruction": "x", "timezone": "UTC",
		"fire_at": now.Add(time.Hour).Format(time.RFC3339), "in_minutes": 30,
	}
	if err := schedule.ValidateCreateArgs(both, now); err == nil {
		t.Fatal("fire_at + in_minutes must be rejected")
	}
	neither := map[string]any{"kind": "once", "instruction": "x", "timezone": "UTC"}
	if err := schedule.ValidateCreateArgs(neither, now); err == nil {
		t.Fatal("once without fire_at/in_minutes must be rejected")
	}
}

func TestCreateInvokerResolvesInMinutes(t *testing.T) {
	store := schedule.NewMemoryStore()
	now := fixedNow()
	reg := tool.NewRegistry()
	tm := schedule.CreateToolMeta(store, setupMeta(), fixedNow)
	reg.RegisterSpecApproved(tm.Spec, tm.Invoker, true)
	_ = reg.RegisterArgsPrecheck(schedule.CreateName, schedule.ValidateCreateArgs)

	c, isErr, err := reg.Invoke(ctxAlice(), schedule.CreateName, map[string]any{
		"kind":        "once",
		"instruction": "再查一次",
		"timezone":    "UTC",
		"in_minutes":  30,
	})
	if err != nil || isErr {
		t.Fatalf("invoke err=%v isErr=%v c=%v", err, isErr, c)
	}
	items, err := store.List("alice")
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	want := now.Add(30 * time.Minute)
	if !items[0].FireAt.Equal(want) || !items[0].NextRunAt.Equal(want) {
		t.Fatalf("fire_at=%v next=%v want %v", items[0].FireAt, items[0].NextRunAt, want)
	}
}

func TestCreateInvokerInsertsActiveRow(t *testing.T) {
	store := schedule.NewMemoryStore()
	meta := setupMeta()
	now := fixedNow()
	fireAt := now.Add(time.Hour)

	reg := tool.NewRegistry()
	tm := schedule.CreateToolMeta(store, meta, fixedNow)
	reg.RegisterSpecApproved(tm.Spec, tm.Invoker, true)
	_ = reg.RegisterArgsPrecheck(schedule.CreateName, schedule.ValidateCreateArgs)

	c, isErr, err := reg.Invoke(ctxAlice(), schedule.CreateName, map[string]any{
		"kind":        "once",
		"instruction": "喝水",
		"timezone":    "UTC",
		"fire_at":     fireAt.Format(time.RFC3339),
	})
	if err != nil || isErr {
		t.Fatalf("invoke err=%v isErr=%v c=%v", err, isErr, c)
	}
	if c["id"] == nil || c["id"] == "" {
		t.Fatalf("missing id: %v", c)
	}

	items, err := store.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d want 1", len(items))
	}
	if items[0].Status != schedule.StatusActive {
		t.Fatalf("status=%q", items[0].Status)
	}
	if items[0].Instruction != "喝水" {
		t.Fatalf("instruction=%q", items[0].Instruction)
	}
	if items[0].ConversationID != "conv1" {
		t.Fatalf("conversation=%q", items[0].ConversationID)
	}
	if !items[0].NextRunAt.Equal(fireAt) {
		t.Fatalf("next_run_at=%v want %v", items[0].NextRunAt, fireAt)
	}
}

func TestCreateInvokerActiveCapOn21st(t *testing.T) {
	store := schedule.NewMemoryStore()
	meta := setupMeta()
	now := fixedNow()
	for i := 0; i < schedule.MaxActivePerOwner; i++ {
		if _, err := store.Insert(schedule.Item{
			OwnerID: "alice", ConversationID: "conv1", Instruction: "x",
			Kind: "daily", TimeOfDay: "09:00", Timezone: "UTC",
			Status: schedule.StatusActive, NextRunAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}

	reg := tool.NewRegistry()
	tm := schedule.CreateToolMeta(store, meta, fixedNow)
	reg.RegisterSpecApproved(tm.Spec, tm.Invoker, true)

	_, isErr, err := reg.Invoke(ctxAlice(), schedule.CreateName, map[string]any{
		"kind":        "once",
		"instruction": "喝水",
		"timezone":    "UTC",
		"fire_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err == nil && !isErr {
		t.Fatal("want cap error")
	}

	items, _ := store.List("alice")
	if len(items) != schedule.MaxActivePerOwner {
		t.Fatalf("count=%d want %d", len(items), schedule.MaxActivePerOwner)
	}
}
