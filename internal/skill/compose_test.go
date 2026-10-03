package skill

import (
	"strings"
	"testing"
)

func newTestCatalog(pkgs []Package) *Catalog {
	byID := make(map[string]Package, len(pkgs))
	for _, p := range pkgs {
		byID[p.ID] = p
	}
	return &Catalog{byID: byID}
}

func TestComposeSystemAndVisibleTools(t *testing.T) {
	cat := newTestCatalog([]Package{
		{ID: "a", Name: "a", Description: "A desc", Tools: []string{"t1", "disabled_skip"}, Body: "body-a"},
		{ID: "b", Name: "b", Description: "", Tools: []string{"t2"}, Body: "body-b"},
	})
	enabled := map[string]bool{"t1": true, "t2": true}
	sys := ComposeSystem("base system", cat, []string{"a"})
	if !strings.Contains(sys, "base system") || !strings.Contains(sys, "a — A desc") || !strings.Contains(sys, "## Skill: a") {
		t.Fatalf("%s", sys)
	}
	vis := VisibleTools(cat, []string{"a"}, enabled)
	if len(vis) != 1 || vis[0] != "t1" {
		t.Fatalf("%v", vis)
	}
	visEmptyDefault := VisibleTools(cat, nil, enabled)
	if len(visEmptyDefault) != 2 {
		t.Fatalf("empty default skills => all enabled, got %v", visEmptyDefault)
	}
}

func TestComposeSystemEmptyCatalog(t *testing.T) {
	cat := newTestCatalog(nil)
	sys := ComposeSystem("base only", cat, []string{"missing"})
	if sys != "base only" {
		t.Fatalf("got %q", sys)
	}
}

func TestComposeSystemSkipsUnknownActivated(t *testing.T) {
	cat := newTestCatalog([]Package{
		{ID: "a", Name: "a", Description: "desc", Body: "body-a"},
	})
	sys := ComposeSystem("base", cat, []string{"missing", "a"})
	if !strings.Contains(sys, "## Skill: a") || strings.Contains(sys, "## Skill: missing") {
		t.Fatalf("%s", sys)
	}
}

func TestComposeSystemEmptyDescriptionUsesName(t *testing.T) {
	cat := newTestCatalog([]Package{
		{ID: "b", Name: "b", Description: "", Body: "body-b"},
	})
	sys := ComposeSystem("base", cat, nil)
	if !strings.Contains(sys, "b — b") {
		t.Fatalf("%s", sys)
	}
}

func TestVisibleToolsUnionAndSort(t *testing.T) {
	cat := newTestCatalog([]Package{
		{ID: "a", Name: "a", Tools: []string{"t2", "t1"}},
		{ID: "b", Name: "b", Tools: []string{"t3", "t1"}},
	})
	enabled := map[string]bool{"t1": true, "t2": true, "t3": true, "off": false}
	vis := VisibleTools(cat, []string{"a", "b"}, enabled)
	if len(vis) != 3 || vis[0] != "t1" || vis[1] != "t2" || vis[2] != "t3" {
		t.Fatalf("got %v", vis)
	}
}

func TestVisibleToolsEmptySliceSameAsNil(t *testing.T) {
	cat := newTestCatalog([]Package{
		{ID: "a", Name: "a", Tools: []string{"t1"}},
	})
	enabled := map[string]bool{"t1": true, "t2": true}
	visNil := VisibleTools(cat, nil, enabled)
	visEmpty := VisibleTools(cat, []string{}, enabled)
	if len(visNil) != 2 || len(visEmpty) != 2 {
		t.Fatalf("nil=%v empty=%v", visNil, visEmpty)
	}
}

func TestActivateToolSpec(t *testing.T) {
	spec := ActivateToolSpec()
	if spec.Name != ActivateToolName {
		t.Fatalf("name=%q", spec.Name)
	}
	if spec.Description == "" {
		t.Fatal("description required")
	}
	if spec.InputSchema["type"] != "object" {
		t.Fatalf("schema=%v", spec.InputSchema)
	}
	props, ok := spec.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties=%v", spec.InputSchema["properties"])
	}
	if _, ok := props["id"]; !ok {
		t.Fatal("missing id property")
	}
	if _, ok := props["ids"]; !ok {
		t.Fatal("missing ids property")
	}
}
