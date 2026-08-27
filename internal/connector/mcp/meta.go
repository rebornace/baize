package mcp

const (
	MetaKeyRunID        = "io.baize/run_id"
	MetaKeyAgentID      = "io.baize/agent_id"
	MetaKeyCallbackURLs = "io.baize/callback_urls"
)

func BuildCallMeta(runID, agentID, callbackEventURL string) map[string]any {
	meta := map[string]any{}
	if runID != "" {
		meta[MetaKeyRunID] = runID
	}
	if agentID != "" {
		meta[MetaKeyAgentID] = agentID
	}
	if callbackEventURL != "" {
		meta[MetaKeyCallbackURLs] = map[string]any{"event": callbackEventURL}
	}
	return meta
}
