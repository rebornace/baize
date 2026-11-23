package tool

import (
	"context"
	"strings"
	"testing"
)

func TestResolveToolNameDeleteToRemove(t *testing.T) {
	catalog := []string{
		"UsersAdminController_findPage",
		"UsersAdminController_findOne",
		"UsersAdminController_remove",
		"PetsAdminController_remove",
	}
	got, ok := ResolveToolName("UsersAdminController_delete", catalog)
	if !ok || got != "UsersAdminController_remove" {
		t.Fatalf("got %q ok=%v want UsersAdminController_remove", got, ok)
	}
}

func TestResolveToolNameExact(t *testing.T) {
	catalog := []string{"UsersAdminController_remove"}
	got, ok := ResolveToolName("UsersAdminController_remove", catalog)
	if !ok || got != "UsersAdminController_remove" {
		t.Fatalf("exact: got %q ok=%v", got, ok)
	}
}

func TestResolveToolNameAmbiguousSynonym(t *testing.T) {
	catalog := []string{
		"UsersAdminController_remove",
		"UsersAdminController_delete",
	}
	if _, ok := ResolveToolName("UsersAdminController_destroy", catalog); ok {
		t.Fatal("ambiguous synonym must not auto-resolve")
	}
}

func TestResolveToolNameWrongPrefix(t *testing.T) {
	catalog := []string{"UsersAdminController_remove"}
	if _, ok := ResolveToolName("PetsAdminController_delete", catalog); ok {
		t.Fatal("must not cross controller prefixes")
	}
}

func TestSuggestToolNames(t *testing.T) {
	catalog := []string{"UsersAdminController_remove", "UsersAdminController_findOne"}
	s := SuggestToolNames("UsersAdminController_delete", catalog, 3)
	if len(s) == 0 || s[0] != "UsersAdminController_remove" {
		t.Fatalf("suggest=%v", s)
	}
}

func TestRegistryResolveNameContinues(t *testing.T) {
	r := NewRegistry()
	r.Register("UsersAdminController_remove", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	name, err := r.ResolveName("UsersAdminController_delete")
	if err != nil || name != "UsersAdminController_remove" {
		t.Fatalf("ResolveName: %q err=%v", name, err)
	}
}

func TestRegistryResolveNameSuggests(t *testing.T) {
	r := NewRegistry()
	r.Register("UsersAdminController_remove", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return nil, false, nil
	})
	_, err := r.ResolveName("TotallyUnknownTool_xyz")
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("err=%v", err)
	}
}
