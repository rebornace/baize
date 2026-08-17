package connector

import (
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestMergeCatalogKeepsDisabledAndExtras(t *testing.T) {
	existing := []store.Tool{
		{Name: "old", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: false, RequireLogin: true},
		{Name: "gone", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: true},
		{Name: "extra1", ConnectorID: "c", Source: store.ToolSourceExtra, Enabled: true, Method: "GET", Path: "/e"},
	}
	discovered := []store.Tool{
		{Name: "old", ConnectorID: "c", Source: store.ToolSourceSpec, Method: "POST", Path: "/old", Description: "d"},
		{Name: "fresh", ConnectorID: "c", Source: store.ToolSourceSpec, Method: "GET", Path: "/n"},
	}
	out := MergeCatalog(MergeOpts{Existing: existing, Discovered: discovered})
	by := map[string]store.Tool{}
	for _, r := range out {
		by[r.Name] = r
	}
	if _, ok := by["gone"]; ok {
		t.Fatal("spec row missing from spec should drop")
	}
	if by["old"].Enabled || !by["old"].RequireLogin || by["old"].Path != "/old" {
		t.Fatalf("old=%+v", by["old"])
	}
	if !by["fresh"].Enabled || by["fresh"].RequireLogin {
		t.Fatalf("fresh=%+v", by["fresh"])
	}
	if by["extra1"].Path != "/e" {
		t.Fatalf("extra=%+v", by["extra1"])
	}
}

func TestMergeCatalogExplicitLoginRewrites(t *testing.T) {
	login := []string{"a"}
	out := MergeCatalog(MergeOpts{
		Existing: []store.Tool{
			{Name: "a", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: true, RequireLogin: false},
			{Name: "b", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: true, RequireLogin: true},
		},
		Discovered: []store.Tool{
			{Name: "a", ConnectorID: "c", Source: store.ToolSourceSpec},
			{Name: "b", ConnectorID: "c", Source: store.ToolSourceSpec},
		},
		RequireLogin: &login,
	})
	by := map[string]store.Tool{}
	for _, r := range out {
		by[r.Name] = r
	}
	if !by["a"].RequireLogin || by["b"].RequireLogin {
		t.Fatalf("%+v", by)
	}
}

func TestMergeCatalogApprovalRewritesAndDropsMissingPlugin(t *testing.T) {
	existing := []store.Tool{
		{Name: "a", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: true, RequireApproval: false},
		{Name: "b", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: true, RequireApproval: true},
		{Name: "plug", ConnectorID: "c", Source: store.ToolSourcePlugin, Enabled: true, Method: "GET", Path: "/p"},
		{Name: "extra1", ConnectorID: "c", Source: store.ToolSourceExtra, Enabled: true, Method: "GET", Path: "/e"},
	}
	discovered := []store.Tool{
		{Name: "a", ConnectorID: "c", Source: store.ToolSourceSpec, Method: "POST", Path: "/a"},
		{Name: "b", ConnectorID: "c", Source: store.ToolSourceSpec, Method: "GET", Path: "/b"},
	}
	out := MergeCatalog(MergeOpts{
		Existing:        existing,
		Discovered:      discovered,
		RequireApproval: []string{"a"},
	})
	by := map[string]store.Tool{}
	for _, r := range out {
		by[r.Name] = r
	}
	if _, ok := by["plug"]; ok {
		t.Fatal("plugin row missing from discovered should drop")
	}
	if by["extra1"].Path != "/e" {
		t.Fatalf("extra should be preserved, got %+v", by["extra1"])
	}
	if !by["a"].RequireApproval || by["b"].RequireApproval {
		t.Fatalf("approval rewrite wrong: %+v", by)
	}
}
