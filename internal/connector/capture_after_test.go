package connector

import (
	"testing"

	"github.com/rebornace/baize/internal/identity"
)

func TestMaybeCaptureLoginAnnotatesMiss(t *testing.T) {
	ids := identity.NewMemoryStore()
	content := map[string]any{"code": 200, "email": "admin@miao.com"}
	maybeCaptureLogin("c1", ids, identity.CaptureConfig{
		ToolNameGlob:   "*login*",
		TokenJSONPaths: []string{"accessToken"},
		HeaderTemplate: "Bearer {{token}}",
	}, "AdminAuthController_login", content, false)
	if captured, _ := content["session_captured"].(bool); captured {
		t.Fatalf("expected session_captured=false, got %+v", content)
	}
	if len(ids.List("c1")) != 0 {
		t.Fatal("must not upsert when token missing")
	}
}

func TestMaybeCaptureLoginAnnotatesHit(t *testing.T) {
	ids := identity.NewMemoryStore()
	content := map[string]any{
		"data": map[string]any{"tokenValue": "satoken-abcdefghijklmnopqrstuvwxyz"},
	}
	maybeCaptureLogin("c1", ids, identity.CaptureConfig{
		ToolNameGlob:   "*login*",
		HeaderTemplate: "Bearer {{token}}",
		DefaultScheme:  "bearer",
	}, "AdminAuthController_login", content, false)
	if captured, _ := content["session_captured"].(bool); !captured {
		t.Fatalf("expected session_captured=true, got %+v", content)
	}
	if len(ids.List("c1")) != 1 {
		t.Fatalf("want 1 identity, got %d", len(ids.List("c1")))
	}
}
