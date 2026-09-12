package connector

import (
	"github.com/rebornace/baize/internal/identity"
)

func maybeCaptureLogin(conv string, ids identity.Store, cfg identity.CaptureConfig, toolName string, content map[string]any, isError bool) {
	if conv == "" || isError || ids == nil || !identity.MatchToolName(cfg.ToolNameGlob, toolName) {
		return
	}
	h, label, sub, claims, ok := identity.ExtractCredential(cfg, content)
	if !ok {
		return
	}
	_, _ = ids.Upsert(conv, identity.Identity{
		Label:             label,
		Scheme:            cfg.DefaultScheme,
		Subject:           sub,
		CredentialHeaders: h,
		Source:            identity.SourceLoginCapture,
		ClaimsSummary:     claims,
		IsDefault:         true,
	})
}
