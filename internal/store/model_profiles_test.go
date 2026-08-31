package store

import "testing"

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
