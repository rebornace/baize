package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestSQLiteAuditEventsOrderAndFilter(t *testing.T) {
	s, err := store.Open("sqlite", filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if c, ok := s.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}()
	base := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	// Trailing-zero fractions are the case RFC3339Nano orders incorrectly on
	// TEXT columns; the store must still return the later event first.
	first := base.Add(200 * time.Millisecond)
	second := base.Add(250 * time.Millisecond)
	if err := s.AppendAuditEvent(store.AuditEvent{
		OperatorID: "alice", Method: "delete", Action: "A", Status: 200, CreatedAt: first,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendAuditEvent(store.AuditEvent{
		OperatorID: "bob", Method: "put", Action: "B", Status: 400, CreatedAt: second,
	}); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListAuditEvents(store.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Action != "B" || all[1].Action != "A" {
		t.Fatalf("order=%+v", all)
	}
	if all[0].Method != "PUT" || all[0].CreatedAt.IsZero() {
		t.Fatalf("normalize=%+v", all[0])
	}

	filtered, err := s.ListAuditEvents(store.AuditFilter{OperatorID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Action != "A" {
		t.Fatalf("filtered=%+v", filtered)
	}
}

func TestAuditTrim(t *testing.T) {
	check := func(t *testing.T, st store.Store) {
		t.Helper()
		base := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
		for i := 0; i < 5; i++ {
			if err := st.AppendAuditEvent(store.AuditEvent{
				Action: "A", CreatedAt: base.Add(time.Duration(i) * time.Second),
			}); err != nil {
				t.Fatal(err)
			}
		}
		if err := st.TrimAuditEvents(2); err != nil {
			t.Fatal(err)
		}
		items, err := st.ListAuditEvents(store.AuditFilter{})
		if err != nil || len(items) != 2 {
			t.Fatalf("items=%+v err=%v", items, err)
		}
		if !items[0].CreatedAt.After(items[1].CreatedAt) {
			t.Fatalf("order=%+v", items)
		}
	}
	t.Run("memory", func(t *testing.T) { check(t, store.NewMemory()) })
	t.Run("sqlite", func(t *testing.T) {
		st, err := store.Open("sqlite", filepath.Join(t.TempDir(), "trim.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if c, ok := st.(interface{ Close() error }); ok {
				_ = c.Close()
			}
		}()
		check(t, st)
	})
}
