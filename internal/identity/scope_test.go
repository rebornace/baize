package identity_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/identity"
)

func TestWebKeyDefaultWorkspace(t *testing.T) {
	if got := identity.WebKey(""); got != identity.WorkspaceScope {
		t.Fatalf("empty workspace key=%q", got)
	}
	if got := identity.WebKey("default"); got != identity.WorkspaceScope {
		t.Fatalf("default workspace key=%q", got)
	}
}

func TestKeySharesWithinWorkspaceOnly(t *testing.T) {
	a := identity.Key("conv_aaaa", "default")
	b := identity.Key("conv_bbbb", "default")
	if a != b || a != identity.WorkspaceScope {
		t.Fatalf("same workspace: %q vs %q", a, b)
	}
	other := identity.Key("conv_aaaa", "ws_other")
	if other == a {
		t.Fatal("different workspaces must not share identity keys")
	}
}

func TestKeyKeepsChannelAndMCPIsolated(t *testing.T) {
	weixin := "weixin:acc:peer-1"
	if got := identity.Key(weixin, "default"); got != weixin {
		t.Fatalf("channel key=%q", got)
	}
	export := "mcp-export-id:idt_export"
	if got := identity.Key(export, "ws_other"); got != export {
		t.Fatalf("mcp export key=%q", got)
	}
	if got := identity.Key("", "default"); got != "" {
		t.Fatalf("empty conversation must stay empty, got %q", got)
	}
}

func TestKeyFromUsesWorkspaceContext(t *testing.T) {
	ctx := identity.WithConversationID(context.Background(), "conv_x")
	ctx = identity.WithWorkspaceID(ctx, "ws_lab")
	if got := identity.KeyFrom(ctx); got != identity.WebKey("ws_lab") {
		t.Fatalf("KeyFrom=%q", got)
	}
}
