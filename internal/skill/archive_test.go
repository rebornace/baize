package skill_test

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/skill"
)

func testArchiveBlobs(t *testing.T) blob.Store {
	t.Helper()
	s, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestArchiveUnarchiveAndCurator(t *testing.T) {
	blobs := testArchiveBlobs(t)
	cat, err := skill.LoadCatalog(nil, "", blobs)
	if err != nil {
		t.Fatal(err)
	}
	usage := skill.NewMemoryUsageStore()
	cat.Usage = usage

	raw := []byte("---\nname: idle-demo\ndescription: d\n---\nbody\n")
	if _, err := cat.InstallMD("SKILL.md", raw); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	// Overwrite install time to look idle.
	_ = usage.EnsureInstalled("idle-demo", now) // no-op if already set
	u, ok := usage.Get("idle-demo")
	if !ok {
		t.Fatal("usage missing after install")
	}
	// Force installed_at into the past via re-seed on a fresh usage store.
	usage = skill.NewMemoryUsageStore()
	cat.Usage = usage
	_ = usage.EnsureInstalled("idle-demo", now.Add(-40*24*time.Hour))

	if _, ok := cat.Get("idle-demo"); !ok {
		t.Fatal("skill must load before archive")
	}
	archived := skill.CurateArchives(cat, usage, 30, now)
	if len(archived) != 1 || archived[0] != "idle-demo" {
		t.Fatalf("archived=%v usage=%+v", archived, u)
	}
	if _, ok := cat.Get("idle-demo"); ok {
		t.Fatal("archived skill must leave active catalog")
	}
	ids, err := cat.ListArchivedIDs()
	if err != nil || len(ids) != 1 || ids[0] != "idle-demo" {
		t.Fatalf("ListArchivedIDs=%v err=%v", ids, err)
	}
	if _, err := blobs.Get(context.Background(), "skills/archived/idle-demo/SKILL.md"); err != nil {
		t.Fatalf("archived blob missing: %v", err)
	}
	if err := cat.UnarchiveUser("idle-demo"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Get("idle-demo"); !ok {
		t.Fatal("unarchive must restore catalog entry")
	}
}

func TestCuratorDisabledWhenZeroDays(t *testing.T) {
	blobs := testArchiveBlobs(t)
	cat, err := skill.LoadCatalog(nil, "", blobs)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("---\nname: keep-me\ndescription: d\n---\nbody\n")
	if _, err := cat.InstallMD("SKILL.md", raw); err != nil {
		t.Fatal(err)
	}
	usage := skill.NewMemoryUsageStore()
	_ = usage.EnsureInstalled("keep-me", time.Now().UTC().Add(-365*24*time.Hour))
	if got := skill.CurateArchives(cat, usage, 0, time.Now().UTC()); len(got) != 0 {
		t.Fatalf("disabled curator archived %v", got)
	}
	if _, ok := cat.Get("keep-me"); !ok {
		t.Fatal("skill must remain")
	}
}

func TestRecordActivateIncrements(t *testing.T) {
	u := skill.NewMemoryUsageStore()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := u.RecordActivate("s1", at); err != nil {
		t.Fatal(err)
	}
	got, ok := u.Get("s1")
	if !ok || got.ActivateCount != 1 || !got.LastActivatedAt.Equal(at) {
		t.Fatalf("%+v ok=%v", got, ok)
	}
	if err := u.RecordActivate("s1", at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ = u.Get("s1")
	if got.ActivateCount != 2 {
		t.Fatalf("count=%d", got.ActivateCount)
	}
}
