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
	recipe := conn.Auth.HostedLogin
	if !recipe.Enabled || strings.TrimSpace(recipe.LoginTool) == "" {
		return "", "", false
	}
	if err := hostedlogin.ValidateRecipe(recipe); err != nil {
		return "", "", false
	}
	origin := strings.TrimRight(strings.TrimSpace(deps.PublicBaseURL), "/")
	if origin == "" {
		return "", "", false
	}
	identityID := strings.TrimSpace(auth.Identity.ID)
	if exportID, err := resolveExportIdentity(deps.Store, auth.Key, connectorID, ""); err == nil {
		identityID = exportID.ID
	}
	if identityID == "" {
		return "", "", false
	}
	id, csrf, err := deps.HostedSessions.Issue(hostedlogin.Session{
		ExportKeyID:  auth.Key.ID,
		ConnectorID:  connectorID,
		LoginTool:    recipe.LoginTool,
		Mode:         recipe.Mode,
		IdentityID:   identityID,
		Fields:       recipe.Fields,
		PublicOrigin: origin,
	})
	if err != nil || id == "" {
		return "", "", false
	}
	_ = csrf
	return fmt.Sprintf("%s/v0/auth/hosted/%s", origin, id), id, true
}

func maybeElicitHostedLoginURL(ctx context.Context, req *mcp.CallToolRequest, hostedURL, sessionID string) {
	if hostedURL == "" || req == nil {
		return
	}
	ss, ok := req.GetSession().(*mcp.ServerSession)
	if !ok || !clientSupportsElicitation(ss) {
		return
	}
	_, _ = ss.Elicit(ctx, &mcp.ElicitParams{
		Mode:          "url",
		Message:       "Sign in to continue using Baize export tools for this system. Do not paste passwords into chat.",
		URL:           hostedURL,
		ElicitationID: sessionID,
	})
}

func authExpiredResult(identityName, hostedURL string) *mcp.CallToolResult {
	msg := AuthExpiredMessageWithURL(identityName, hostedURL)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
}
