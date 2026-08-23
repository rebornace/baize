package connector

// CloseAllMCPSessions closes every pooled stdio MCP session.
func CloseAllMCPSessions() {
	mcpPool.CloseAll()
}
