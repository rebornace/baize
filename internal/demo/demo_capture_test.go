package demo

import (
	"testing"

	"github.com/rebornace/baize/internal/identity"
)

func TestWithCaptureDefaultsFillsEmpty(t *testing.T) {
	got := withCaptureDefaults(identity.CaptureConfig{})
	if got.ToolNameGlob != "*login*" {
		t.Fatalf("ToolNameGlob=%q", got.ToolNameGlob)
	}
	if len(got.TokenJSONPaths) == 0 || got.HeaderTemplate == "" {
		t.Fatalf("defaults incomplete: %+v", got)
	}
	if !identity.MatchToolName(got.ToolNameGlob, "AdminAuthController_login") {
		t.Fatal("default glob should match AdminAuthController_login")
	}
}

func TestWithCaptureDefaultsKeepsExplicit(t *testing.T) {
	got := withCaptureDefaults(identity.CaptureConfig{
		ToolNameGlob:   "__none__",
		TokenJSONPaths: []string{"tok"},
	})
	if got.ToolNameGlob != "__none__" || got.TokenJSONPaths[0] != "tok" {
		t.Fatalf("%+v", got)
	}
}
