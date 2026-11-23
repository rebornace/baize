package identity

import "strings"

// WorkspaceScope is the identity-store key for the default Web UI workspace.
// Existing rows migrated from per-chat keys live here.
const WorkspaceScope = "workspace"

const (
	mcpExportPrefix    = "mcp-export-id:"
	webWorkspacePrefix = "ws:"
	defaultWorkspaceID = "default"
)

// IsolatedConversation reports channel / MCP-export threads that must not
// share Web UI workspace logins.
func IsolatedConversation(conversationID string) bool {
	return isolatedIdentityConversation(conversationID)
}

// WebKey is the identity-store key for a Web UI workspace.
func WebKey(workspaceID string) string {
	id := strings.TrimSpace(workspaceID)
	if id == "" || id == defaultWorkspaceID {
		return WorkspaceScope
	}
	if strings.HasPrefix(id, webWorkspacePrefix) {
		return id
	}
	return webWorkspacePrefix + id
}

// Key maps a conversation onto an identity-store key.
// Channel and MCP-export ids stay isolated; Web chats share their workspace.
func Key(conversationID, workspaceID string) string {
	if conversationID == "" || isolatedIdentityConversation(conversationID) {
		return conversationID
	}
	return WebKey(workspaceID)
}

// Scope is Key(conversationID, "") — Web chats without a workspace id use the
// default workspace pool.
func Scope(conversationID string) string {
	return Key(conversationID, "")
}

func isolatedIdentityConversation(conversationID string) bool {
	if strings.HasPrefix(conversationID, mcpExportPrefix) {
		return true
	}
	if strings.HasPrefix(conversationID, webWorkspacePrefix) {
		return true
	}
	if conversationID == WorkspaceScope {
		return true
	}
	// Channel ConvID is "<source>:<account>:<peer>" (≥2 colons).
	return strings.Count(conversationID, ":") >= 2
}
