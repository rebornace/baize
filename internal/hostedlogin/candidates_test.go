package hostedlogin_test

import (
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
)

func TestBuildCandidatesFromLoginToolAndSecurity(t *testing.T) {
	tools := []store.Tool{{
		Name: "mall.login", ConnectorID: "mall",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username": map[string]any{"type": "string"},
				"password": map[string]any{"type": "string"},
			},
		},
	}}
	schemes := map[string]any{
		"bearer": map[string]any{"type": "http", "scheme": "bearer"},
	}
	cands := hostedlogin.BuildCandidates(tools, "*login*", schemes)
	if len(cands) < 2 {
		t.Fatalf("got %d", len(cands))
	}
	var toolID, secID bool
	for _, c := range cands {
		switch c.ID {
		case "tool-mall.login":
			toolID = true
			if c.Mode != hostedlogin.ModePasswordForm || c.LoginTool != "mall.login" {
				t.Fatalf("tool candidate: %+v", c)
			}
		case "sec-bearer":
			secID = true
			if c.Mode != hostedlogin.ModeTokenPaste {
				t.Fatalf("security candidate: %+v", c)
			}
		}
	}
	if !toolID || !secID {
		t.Fatalf("ids missing: %+v", cands)
	}
}
