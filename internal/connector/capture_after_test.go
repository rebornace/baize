package connector

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/identity"
)

func captureCtx(conv, workspace string) context.Context {
	ctx := identity.WithConversationID(context.Background(), conv)
	if workspace != "" {
		ctx = identity.WithWorkspaceID(ctx, workspace)
	}
	return ctx
}

func TestMaybeCaptureLoginAnnotatesMiss(t *testing.T) {
	ids := identity.NewMemoryStore()
	content := map[string]any{"code": 200, "email": "a@b.com"}
	maybeCaptureLogin(captureCtx("c1", ""), ids, identity.CaptureConfig{
		ToolNameGlob:   "*login*",
		TokenJSONPaths: []string{"accessToken"},
		HeaderTemplate: "Bearer {{token}}",
	}, "AdminAuthController_login", content, false)
	if captured, _ := content["session_captured"].(bool); captured {
		t.Fatalf("expected session_captured=false, got %+v", content)
	}
	if len(ids.List(identity.Scope("c1"))) != 0 {
		t.Fatal("must not upsert when token missing")
	}
}

func TestMaybeCaptureLoginAnnotatesHit(t *testing.T) {
	ids := identity.NewMemoryStore()
	content := map[string]any{
		"data": map[string]any{"tokenValue": "satoken-abcdefghijklmnopqrstuvwxyz"},
	}
	maybeCaptureLogin(captureCtx("c1", ""), ids, identity.CaptureConfig{
		ToolNameGlob:   "*login*",
		HeaderTemplate: "Bearer {{token}}",
		DefaultScheme:  "bearer",
	}, "AdminAuthController_login", content, false)
	if captured, _ := content["session_captured"].(bool); !captured {
		t.Fatalf("expected session_captured=true, got %+v", content)
	}
	if len(ids.List(identity.Scope("c1"))) != 1 {
		t.Fatalf("want 1 identity, got %d", len(ids.List(identity.Scope("c1"))))
	}
}

func TestMaybeCaptureLoginSharesAcrossWebConversations(t *testing.T) {
	ids := identity.NewMemoryStore()
	content := map[string]any{
		"data": map[string]any{"tokenValue": "satoken-abcdefghijklmnopqrstuvwxyz"},
	}
	maybeCaptureLogin(captureCtx("conv_first", ""), ids, identity.CaptureConfig{
		ToolNameGlob:   "*login*",
		HeaderTemplate: "Bearer {{token}}",
		DefaultScheme:  "bearer",
	}, "AdminAuthController_login", content, false)
	if len(ids.List(identity.Scope("conv_second"))) != 1 {
		t.Fatalf("new web chat should see captured login, got %d", len(ids.List(identity.Scope("conv_second"))))
	}
	if len(ids.List("weixin:acc:peer")) != 0 {
		t.Fatal("channel conversation must not inherit web login")
	}
}

func TestMaybeCaptureLoginIsolatesWorkspaces(t *testing.T) {
	ids := identity.NewMemoryStore()
	content := map[string]any{
		"data": map[string]any{"tokenValue": "satoken-abcdefghijklmnopqrstuvwxyz"},
	}
	maybeCaptureLogin(captureCtx("conv_a", "ws_lab"), ids, identity.CaptureConfig{
		ToolNameGlob:   "*login*",
		HeaderTemplate: "Bearer {{token}}",
		DefaultScheme:  "bearer",
	}, "AdminAuthController_login", content, false)
	if len(ids.List(identity.WebKey("ws_other"))) != 0 {
		t.Fatal("other workspace must not see this login")
	}
	if len(ids.List(identity.WebKey("ws_lab"))) != 1 {
		t.Fatal("same workspace should see captured login")
	}
}
