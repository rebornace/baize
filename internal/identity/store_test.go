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
}
