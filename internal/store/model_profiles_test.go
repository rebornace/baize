package store

import (
	"path/filepath"
	"testing"
)

func TestMemoryModelProfileCRUDAndDeleteAll(t *testing.T) {
	s := NewMemory()

	p, err := s.UpsertModelProfile(ModelProfile{
		Name: "标准", Provider: "openai_compatible", BaseURL: "https://x/v1",
		Model: "m1", APIKey: "sk-secret-1234", AutoTier: AutoTierStandard,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if p.ID == "" || p.CreatedAt.IsZero() {
		t.Fatalf("id/createdAt not set: %+v", p)
	}
	p2, _ := s.UpsertModelProfile(ModelProfile{
		Name: "廉价", Provider: "openai_compatible", BaseURL: "https://y/v1",
		Model: "gpt-4o-mini", APIKeyEnv: "KEY2",
	})

	// The store normalizes an unset/unknown tier to standard; name-based
	// inference happens at the API/seed layer, not in the store.
	if got, _ := s.GetModelProfile(p2.ID); got.AutoTier != AutoTierStandard {
		t.Fatalf("unset tier should normalize to standard, stored=%q", got.AutoTier)
	}

	if got, err := s.GetModelProfile(p.ID); err != nil || got.APIKey != "sk-secret-1234" {
		t.Fatalf("store should keep raw key internally; got %q err=%v", got.APIKey, err)
	}

	// Any profile — including the last remaining one — can be deleted.
	if err := s.DeleteModelProfile(p2.ID); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if err := s.DeleteModelProfile(p.ID); err != nil {
		t.Fatalf("delete sole/last profile must be allowed: %v", err)
	}
	if _, err := s.GetModelProfile(p.ID); err == nil {
		t.Fatalf("expected not-found after delete")
	}
	list, _ := s.ListModelProfiles()
	if len(list) != 0 {
		t.Fatalf("expected empty store after deleting all, got %d", len(list))
	}
}

func TestNormalizeAutoTier(t *testing.T) {
	cases := map[string]string{
		"":         AutoTierStandard,
		"light":    AutoTierLight,
		"power":    AutoTierPower,
		"standard": AutoTierStandard,
		"weird":    AutoTierStandard,
	}
	for in, want := range cases {
		if got := NormalizeAutoTier(in); got != want {
			t.Errorf("NormalizeAutoTier(%q)=%q want %q", in, got, want)
		}
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
		Name: "标准", Provider: "openai_compatible", BaseURL: "https://x/v1",
		Model: "m1", APIKey: "sk-secret-1234", SupportsVision: true, AutoTier: AutoTierPower,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := s.GetModelProfile(p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.APIKey != "sk-secret-1234" || !got.SupportsVision || got.AutoTier != AutoTierPower {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// edit: redacted key must not overwrite; UpdatedAt must advance
	updated, err := s.UpsertModelProfile(ModelProfile{
		ID: p.ID, Name: "标准", Provider: "openai_compatible",
		BaseURL: "https://x/v1", Model: "m1b", APIKey: RedactAPIKey("sk-secret-1234"),
		SupportsVision: true, AutoTier: AutoTierPower,
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

	// Deleting the (here, only) profile must succeed.
	if err := s.DeleteModelProfile(p.ID); err != nil {
		t.Fatalf("deleting the only profile must be allowed: %v", err)
	}
	if _, err := s.GetModelProfile(p.ID); err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestSQLiteModelProfileContextTokens(t *testing.T) {
	s := newSQLiteProfileStore(t)
	got, err := s.UpsertModelProfile(ModelProfile{
		Name: "ctx", Provider: "openai_compatible", BaseURL: "http://x", Model: "m",
		APIKey: "sk-abcdefgh1234", ContextTokens: 200000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextTokens != 200000 {
		t.Fatalf("ContextTokens not persisted: %d", got.ContextTokens)
	}
	again, err := s.GetModelProfile(got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ContextTokens != 200000 {
		t.Fatalf("ContextTokens round trip failed: %d", again.ContextTokens)
	}

	// UPDATE path must persist a changed value.
	updated, err := s.UpsertModelProfile(ModelProfile{
		ID: got.ID, Name: "ctx", Provider: "openai_compatible", BaseURL: "http://x",
		Model: "m", APIKey: RedactAPIKey("sk-abcdefgh1234"), ContextTokens: 64000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContextTokens != 64000 {
		t.Fatalf("ContextTokens not updated: %d", updated.ContextTokens)
	}
	again2, err := s.GetModelProfile(got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again2.ContextTokens != 64000 {
		t.Fatalf("ContextTokens update round trip failed: %d", again2.ContextTokens)
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
