package store

import (
	"path/filepath"
	"testing"
)

func TestMemoryModelProfileCRUDAndDefault(t *testing.T) {
	s := NewMemory()

	p, err := s.UpsertModelProfile(ModelProfile{
		Name: "主力", Provider: "openai_compatible", BaseURL: "https://x/v1",
		Model: "m1", APIKey: "sk-secret-1234",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if p.ID == "" || p.CreatedAt.IsZero() {
		t.Fatalf("id/createdAt not set: %+v", p)
	}
	if err := s.SetDefaultModelProfile(p.ID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	p2, _ := s.UpsertModelProfile(ModelProfile{
		Name: "廉价", Provider: "openai_compatible", BaseURL: "https://y/v1",
		Model: "m2", APIKeyEnv: "KEY2",
	})
	if err := s.SetDefaultModelProfile(p2.ID); err != nil {
		t.Fatalf("set default 2: %v", err)
	}

	list, _ := s.ListModelProfiles()
	defaults := 0
	for _, m := range list {
		if m.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("want exactly 1 default, got %d", defaults)
	}

	if got, err := s.GetModelProfile(p.ID); err != nil || got.APIKey != "sk-secret-1234" {
		t.Fatalf("store should keep raw key internally; got %q err=%v", got.APIKey, err)
	}

	if err := s.DeleteModelProfile(p2.ID); err == nil {
		t.Fatalf("deleting the default profile must be rejected")
	}
	if err := s.DeleteModelProfile(p.ID); err != nil {
		t.Fatalf("delete non-default: %v", err)
	}
	if _, err := s.GetModelProfile(p.ID); err == nil {
		t.Fatalf("expected not-found after delete")
	}
}

func TestMemoryUpsertRejectsEmptyNameAndDuplicate(t *testing.T) {
	s := NewMemory()
	if _, err := s.UpsertModelProfile(ModelProfile{Provider: "openai_compatible", Model: "m"}); err == nil {
		t.Fatal("empty name must be rejected")
	}
	if _, err := s.UpsertModelProfile(ModelProfile{Name: "dup", Provider: "openai_compatible", Model: "m", BaseURL: "u"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := s.UpsertModelProfile(ModelProfile{Name: "dup", Provider: "openai_compatible", Model: "m2", BaseURL: "u2"}); err == nil {
		t.Fatal("duplicate name (different id) must be rejected")
	}
}

func TestMemoryUpsertEditKeepsRawKeyWhenRedacted(t *testing.T) {
	s := NewMemory()
	p, err := s.UpsertModelProfile(ModelProfile{
		Name: "主力", Provider: "openai_compatible", BaseURL: "https://x/v1",
		Model: "m1", APIKey: "sk-secret-1234",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := s.UpsertModelProfile(ModelProfile{
		ID: p.ID, Name: "主力", Provider: "openai_compatible",
		BaseURL: "https://x/v1", Model: "m1b",
		APIKey: RedactAPIKey("sk-secret-1234"),
	}); err != nil {
		t.Fatalf("upsert edit: %v", err)
	}
	got, err := s.GetModelProfile(p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.APIKey != "sk-secret-1234" {
		t.Fatalf("redacted key overwrote stored key: %q", got.APIKey)
	}
}

func newSQLiteProfileStore(t *testing.T) *SQLStore {
	t.Helper()
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSQLiteModelProfileRoundTrip(t *testing.T) {
	s := newSQLiteProfileStore(t)
	p, err := s.UpsertModelProfile(ModelProfile{
		Name: "主力", Provider: "openai_compatible", BaseURL: "https://x/v1",
		Model: "m1", APIKey: "sk-secret-1234", SupportsVision: true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.SetDefaultModelProfile(p.ID); err != nil {
		t.Fatalf("default: %v", err)
	}
	got, err := s.GetModelProfile(p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.APIKey != "sk-secret-1234" || !got.SupportsVision || !got.IsDefault {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// edit: redacted key must not overwrite; UpdatedAt must advance
	updated, err := s.UpsertModelProfile(ModelProfile{
		ID: p.ID, Name: "主力", Provider: "openai_compatible",
		BaseURL: "https://x/v1", Model: "m1b", APIKey: RedactAPIKey("sk-secret-1234"),
		SupportsVision: true,
	})
	if err != nil {
		t.Fatalf("upsert edit: %v", err)
	}
	if updated.APIKey != "sk-secret-1234" {
		t.Fatalf("redacted key overwrote stored key: %q", updated.APIKey)
	}
	if updated.Model != "m1b" {
		t.Fatalf("model not updated: %q", updated.Model)
	}

	if err := s.DeleteModelProfile(p.ID); err == nil {
		t.Fatal("deleting default must be rejected on SQL store")
	}
}

func TestSQLiteRunPersistsModelProfileID(t *testing.T) {
	s := newSQLiteProfileStore(t)
	s.UpsertAgent(Agent{ID: "a"})
	r, err := s.CreateRun(CreateRunInput{AgentID: "a", Input: "hi", ModelProfileID: "mp_123"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	got, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.ModelProfileID != "mp_123" {
		t.Fatalf("model_profile_id not persisted: %q", got.ModelProfileID)
	}
}
