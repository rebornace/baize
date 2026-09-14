package connector

import (
	"log"

	"github.com/rebornace/baize/internal/identity"
)

func maybeCaptureLogin(conv string, ids identity.Store, cfg identity.CaptureConfig, toolName string, content map[string]any, isError bool) {
	if conv == "" || isError || ids == nil || !identity.MatchToolName(cfg.ToolNameGlob, toolName) {
		return
	}
	h, label, sub, claims, ok := identity.ExtractCredential(cfg, content)
	if !ok {
		if content != nil {
			content["session_captured"] = false
			content["session_capture_error"] = "login succeeded but no token was found in the result; later API calls will be unauthenticated"
		}
		log.Printf("connector: capture missed for %s (conversation %s): no token in tool result", toolName, conv)
		return
	}
	_, err := ids.Upsert(conv, identity.Identity{
		Label:             label,
		Scheme:            cfg.DefaultScheme,
		Subject:           sub,
		CredentialHeaders: h,
		Source:            identity.SourceLoginCapture,
		ClaimsSummary:     claims,
		IsDefault:         true,
	})
	if content != nil {
		content["session_captured"] = err == nil
		if err != nil {
			content["session_capture_error"] = err.Error()
		}
	}
	if err != nil {
		log.Printf("connector: capture upsert failed for %s: %v", toolName, err)
	}
}
