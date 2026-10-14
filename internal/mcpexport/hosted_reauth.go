package mcpexport

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
)

// hostedReauthURL issues a hosted-login session when the connector recipe is enabled.
func hostedReauthURL(deps ServerDeps, auth exportAuth, toolRow store.Tool) (string, string, bool) {
	if deps.HostedSessions == nil || deps.Store == nil {
		return "", "", false
	}
	connectorID := strings.TrimSpace(toolRow.ConnectorID)
	if connectorID == "" {
		return "", "", false
	}
	conn, err := deps.Store.GetConnector(connectorID)
	if err != nil {
		return "", "", false
	}
	cfg := conn.Auth.HostedLogin
	if !cfg.Enabled {
		return "", "", false
	}
	methodID := strings.TrimSpace(cfg.DefaultMethodID)
	if methodID == "" {
		return "", "", false
	}
	origin := strings.TrimRight(strings.TrimSpace(deps.PublicBaseURL), "/")
	if origin == "" {
		return "", "", false
	}
	// Target the identity that actually failed this call so login writes headers
	// back where the next tools/call will read them. Fall back to the key slot
	// for the default method role when the call had no identity yet.
	identityID := strings.TrimSpace(auth.Identity.ID)
	if identityID == "" {
		role := ""
		if m, ok := cfg.DefaultMethod(); ok {
			role = m.EffectiveRole()
		}
		if exportID, err := resolveExportIdentity(deps.Store, auth.Key, connectorID, role); err == nil {
			identityID = exportID.ID
		}
	}
	if identityID == "" {
		return "", "", false
	}
	seed, err := hostedlogin.MaterializeSession(cfg, methodID, hostedlogin.Session{
		ExportKeyID:  auth.Key.ID,
		ConnectorID:  connectorID,
		IdentityID:   identityID,
		PublicOrigin: origin,
		// Links appear in tool errors / transcripts — shorter than console test sessions.
		TTL: hostedlogin.MCPReauthSessionTTL,
	})
	if err != nil {
		return "", "", false
	}
	id, csrf, err := deps.HostedSessions.Issue(seed)
	if err != nil || id == "" {
		return "", "", false
	}
	_ = csrf
	return fmt.Sprintf("%s/v0/auth/hosted/%s", origin, id), id, true
}

func clientSupportsURLElicitation(ss *mcp.ServerSession) bool {
	ip := ss.InitializeParams()
	if ip == nil || ip.Capabilities == nil || ip.Capabilities.Elicitation == nil {
		return false
	}
	return ip.Capabilities.Elicitation.URL != nil
}

func maybeElicitHostedLoginURL(ctx context.Context, req *mcp.CallToolRequest, hostedURL, sessionID, identityName string) {
	if hostedURL == "" || req == nil {
		return
	}
	ss, ok := req.GetSession().(*mcp.ServerSession)
	if !ok || !clientSupportsElicitation(ss) {
		return
	}
	// Prefer protocol -32042 when the client advertises URL elicitation; this path
	// is only for clients that support elicitation but not URL mode.
	if clientSupportsURLElicitation(ss) {
		return
	}
	_, _ = ss.Elicit(ctx, &mcp.ElicitParams{
		Mode:          "url",
		Message:       HostedLoginElicitMessage(identityName),
		URL:           hostedURL,
		ElicitationID: sessionID,
	})
}

func authExpiredResult(identityName, hostedURL string) *mcp.CallToolResult {
	msg := AuthExpiredMessageWithURL(identityName, hostedURL)
	payload := map[string]any{
		"code":       HostedLoginRequiredCode,
		"message":    msg,
		"message_zh": AuthExpiredMessageZH(identityName, hostedURL),
		"message_en": AuthExpiredMessageEN(identityName, hostedURL),
	}
	if name := strings.TrimSpace(identityName); name != "" {
		payload["identity"] = name
	}
	if u := strings.TrimSpace(hostedURL); u != "" {
		payload["hosted_login_url"] = u
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: msg}},
		StructuredContent: payload,
		IsError:           true,
	}
}

func urlElicitationRequired(hostedURL, sessionID, identityName string) error {
	return mcp.URLElicitationRequiredError([]*mcp.ElicitParams{{
		Mode:          "url",
		Message:       HostedLoginElicitMessage(identityName),
		URL:           hostedURL,
		ElicitationID: sessionID,
	}})
}
