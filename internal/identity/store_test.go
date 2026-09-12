package identity_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/identity"
)

func TestStoreUpsertListDeleteDefault(t *testing.T) {
	s := identity.NewMemoryStore()
	now := time.Now().UTC()
	id, err := s.Upsert("conv1", identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer SECRET_TOKEN"},
		Source:            identity.SourceLoginCapture,
		Subject:           "admin@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id == "" {
		t.Fatalf("upsert: id=%q err=%v", id, err)
	}
	views := s.ListPublic("conv1")
	if len(views) != 1 || views[0].Label != "admin@x.com" || views[0].IsDefault != true {
		t.Fatalf("views=%+v", views)
	}
	raw, _ := json.Marshal(views)
	if strings.Contains(string(raw), "SECRET_TOKEN") {
		t.Fatal("public list leaked token")
	}
	if err := s.SetDefault("conv1", id); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("conv1", id); err != nil {
		t.Fatal(err)
	}
	if len(s.ListPublic("conv1")) != 0 {
		t.Fatal("expected empty")
	}
}

func TestStoreUpsertSameSchemeSubject(t *testing.T) {
	s := identity.NewMemoryStore()
	now := time.Now().UTC()
	id1, err := s.Upsert("conv1", identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer TOKEN1"},
		Source:            identity.SourceLoginCapture,
		Subject:           "admin@x.com",
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id1 == "" {
		t.Fatalf("first upsert: id=%q err=%v", id1, err)
	}
	id2, err := s.Upsert("conv1", identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer TOKEN2"},
		Source:            identity.SourceLoginCapture,
		Subject:           "admin@x.com",
		UpdatedAt:         now,
	})
	if err != nil || id2 == "" {
		t.Fatalf("second upsert: id=%q err=%v", id2, err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id, got %q and %q", id1, id2)
	}
	if len(s.ListPublic("conv1")) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(s.ListPublic("conv1")))
	}
	got, err := s.Get("conv1", id1)
	if err != nil {
		t.Fatal(err)
	}
	if got.CredentialHeaders["Authorization"] != "Bearer TOKEN2" {
		t.Fatalf("credential not updated: got %q", got.CredentialHeaders["Authorization"])
	}
}

func countDefaults(ids []identity.Identity) int {
	n := 0
	for _, id := range ids {
		if id.IsDefault {
			n++
		}
	}
	return n
}

func TestStoreIsDefaultMutualExclusionOnUpsert(t *testing.T) {
	s := identity.NewMemoryStore()
	now := time.Now().UTC()
	id1, err := s.Upsert("conv1", identity.Identity{
		Label:             "user1@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer T1"},
		Source:            identity.SourceLoginCapture,
		Subject:           "user1@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id1 == "" {
		t.Fatalf("first upsert: id=%q err=%v", id1, err)
	}
	id2, err := s.Upsert("conv1", identity.Identity{
		Label:             "user2@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer T2"},
		Source:            identity.SourceLoginCapture,
		Subject:           "user2@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id2 == "" {
		t.Fatalf("second upsert: id=%q err=%v", id2, err)
	}
	list := s.List("conv1")
	if countDefaults(list) != 1 {
		t.Fatalf("expected exactly 1 default, got %d in %+v", countDefaults(list), list)
	}
	got2, err := s.Get("conv1", id2)
	if err != nil {
		t.Fatal(err)
	}
	if !got2.IsDefault {
		t.Fatal("second identity should be default")
	}
	got1, err := s.Get("conv1", id1)
	if err != nil {
		t.Fatal(err)
	}
	if got1.IsDefault {
		t.Fatal("first identity should no longer be default")
	}
}

func TestStoreIsDefaultMutualExclusionOnSetDefault(t *testing.T) {
	s := identity.NewMemoryStore()
	now := time.Now().UTC()
	id1, err := s.Upsert("conv1", identity.Identity{
		Label:             "user1@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer T1"},
		Source:            identity.SourceLoginCapture,
		Subject:           "user1@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id1 == "" {
		t.Fatalf("first upsert: id=%q err=%v", id1, err)
	}
	id2, err := s.Upsert("conv1", identity.Identity{
		Label:             "user2@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer T2"},
		Source:            identity.SourceLoginCapture,
		Subject:           "user2@x.com",
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id2 == "" {
		t.Fatalf("second upsert: id=%q err=%v", id2, err)
	}
	if err := s.SetDefault("conv1", id2); err != nil {
		t.Fatal(err)
	}
	list := s.List("conv1")
	if countDefaults(list) != 1 {
		t.Fatalf("expected exactly 1 default, got %d in %+v", countDefaults(list), list)
	}
	got2, err := s.Get("conv1", id2)
	if err != nil {
		t.Fatal(err)
	}
	if !got2.IsDefault {
		t.Fatal("second identity should be default after SetDefault")
	}
	got1, err := s.Get("conv1", id1)
	if err != nil {
		t.Fatal(err)
	}
	if got1.IsDefault {
		t.Fatal("first identity should no longer be default after SetDefault")
	}
}
