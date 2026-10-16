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
