package controlplane

import "strings"

type routeRule struct {
	method   string
	segments []string
	role     Role
}

var aclRules = []routeRule{
	// Viewer: read-only chat, run history, cost reports and audit.
	{method: "GET", segments: []string{"v0", "me"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "conversations"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "conversations", "search"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "conversations", "{id}", "export"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "conversations", "{id}", "messages"}, role: RoleViewer},
	{method: "PUT", segments: []string{"v0", "conversations", "{id}", "read"}, role: RoleViewer},
	{method: "POST", segments: []string{"v0", "conversations", "read-all"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "runs", "{id}"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "runs", "{id}", "events"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "runs", "{id}", "stream"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "runs", "{id}", "children"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "artifacts", "{id}"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "channels", "media", "{conv}", "{object}"}, role: RoleViewer},
	{method: "GET", segments: []string{"v0", "workspaces"}, role: RoleViewer},
	{method: "POST", segments: []string{"v0", "runs"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "runs", "{id}", "resume"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "runs", "{id}", "cancel"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "runs", "{id}", "plugin-callbacks"}, role: RoleNone},
	{method: "GET", segments: []string{"v0", "hitl", "pending"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "hitl", "approvals", "{token}"}, role: RoleNone},
	{method: "POST", segments: []string{"v0", "hitl", "approvals", "{token}"}, role: RoleNone},
	{method: "GET", segments: []string{"v0", "runs", "{id}", "stream"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "runs", "{id}", "events"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "runs", "{id}", "binding"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "runs", "{id}"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "runs", "{id}", "children"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "artifacts", "{id}"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "channels", "media", "{conv}", "{object}"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "conversations"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "conversations", "search"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "conversations", "{id}", "export"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "workspaces"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "workspaces"}, role: RoleOperator},
	{method: "DELETE", segments: []string{"v0", "conversations", "{id}"}, role: RoleOperator},
	{method: "PATCH", segments: []string{"v0", "conversations", "{id}"}, role: RoleOperator},
	{method: "PUT", segments: []string{"v0", "conversations", "{id}", "pin"}, role: RoleOperator},
	{method: "PUT", segments: []string{"v0", "conversations", "{id}", "archived"}, role: RoleOperator},
	{method: "PUT", segments: []string{"v0", "conversations", "{id}", "read"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "read-all"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "batch", "workspace"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "batch", "delete"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "batch", "archived"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "conversations", "{id}", "messages"}, role: RoleOperator},
	{method: "DELETE", segments: []string{"v0", "conversations", "{id}", "messages"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "conversations", "{id}", "goal"}, role: RoleOperator},
	{method: "PUT", segments: []string{"v0", "conversations", "{id}", "goal"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "{id}", "goal", "push"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "{id}", "goal", "pop"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "{id}", "goal", "stop"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "{id}", "goal", "resume"}, role: RoleOperator},
	{method: "PATCH", segments: []string{"v0", "conversations", "{id}", "goal", "tip"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "conversations", "{id}", "identities"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "{id}", "identities"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "{id}", "identities", "{iid}", "default"}, role: RoleOperator},
	{method: "DELETE", segments: []string{"v0", "conversations", "{id}", "identities", "{iid}"}, role: RoleOperator},
	{method: "DELETE", segments: []string{"v0", "conversations", "{id}", "identities"}, role: RoleOperator},
	{method: "PUT", segments: []string{"v0", "agents", "{id}"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "connectors", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "connectors"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "connectors", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "connectors", "{id}", "catalog-revisions"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "connectors", "{id}", "mcp", "oauth", "start"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "connectors", "{id}", "mcp", "oauth", "callback"}, role: RoleNone},
	{method: "POST", segments: []string{"v0", "connectors", "{id}", "mcp", "oauth", "disconnect"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "connectors", "{id}", "mcp", "oauth", "status"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "tools"}, role: RoleOperator},
	{method: "PATCH", segments: []string{"v0", "tools", "{name}"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "connectors", "{id}", "tools"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "connectors", "{id}", "tools", "{name}"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "connectors", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "skills"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "skills", "{id}"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "skills"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "skills", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "skills", "{id}", "eval"}, role: RoleOperator},
	{method: "PUT", segments: []string{"v0", "skills", "{id}", "eval"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "skills", "{id}", "eval"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "skills", "{id}", "eval", "run"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "agents", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "events-webhook"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "settings", "events-webhook"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "events-webhook", "test"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "events-webhook", "deliveries"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "events-webhook", "deliveries", "{id}", "retry"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "inbox", "{id}"}, role: RoleNone},
	{method: "POST", segments: []string{"v0", "channels", "{id}", "inbound"}, role: RoleNone},
	{method: "GET", segments: []string{"v0", "settings", "inbox-channels"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "settings", "inbox-channels"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "connector-groups"}, role: RoleOperator},
	{method: "PUT", segments: []string{"v0", "settings", "connector-groups"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "inbox-channels", "{id}", "rotate-secret"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "inbox-channels", "{id}", "test"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "store"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "settings", "store"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "store", "restart"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "reload"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "channels", "inbound-jobs"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "channels", "inbound-jobs", "{id}", "retry"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "channels", "{name}", "login", "start"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "settings", "channels", "{name}", "login", "status"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "settings", "channels", "{name}", "logout"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "channels", "{name}", "process", "start"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "channels", "{name}", "process", "stop"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "channels", "{name}", "process", "restart"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "channels", "{name}", "outbound-deliveries"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "channels", "{name}", "outbound-deliveries", "{id}", "retry"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "channels", "{name}"}, role: RoleOperator},
	{method: "PUT", segments: []string{"v0", "settings", "channels", "{name}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "mcp-export"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "mcp-export", "identities"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "mcp-export", "identities"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "mcp-export", "identities", "{id}"}, role: RoleAdmin},
	{method: "PATCH", segments: []string{"v0", "settings", "mcp-export", "identities", "{id}"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "settings", "mcp-export", "identities", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "mcp-export", "keys"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "mcp-export", "keys"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "settings", "mcp-export", "keys", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "mcp-export", "invocations"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "runtime"}, role: RoleOperator},
	{method: "PATCH", segments: []string{"v0", "settings", "runtime"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "logs"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "eval", "runs"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "eval", "sample"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "eval", "runs", "{id}", "skill-draft"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "eval", "runs", "{id}", "skill-draft", "polish"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "eval", "runs", "{id}", "skill-draft", "install"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "conversations", "{id}", "skill-draft"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "eval", "cases"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "eval", "cases"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "eval", "cases", "{id}"}, role: RoleAdmin},
	{method: "PATCH", segments: []string{"v0", "settings", "eval", "cases", "{id}"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "eval", "cases", "{id}", "score"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "eval", "cases", "{id}", "check-state"}, role: RoleAdmin},
	{method: "PATCH", segments: []string{"v0", "settings", "eval", "cases", "{id}", "calibration"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "eval", "calibration-summary"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "eval", "cases", "{id}", "export"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "settings", "eval", "cases", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "tool-retrieval"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "settings", "tool-retrieval", "enable"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "tool-retrieval", "disable"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "tool-retrieval", "cleanup"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "settings", "tool-retrieval", "paths"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "credentials"}, role: RoleAdmin},
	{method: "PATCH", segments: []string{"v0", "settings", "credentials"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "models"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "settings", "models"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "models", "discover"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "models", "batch"}, role: RoleAdmin},
	{method: "PATCH", segments: []string{"v0", "settings", "models", "{id}"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "settings", "models", "{id}"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "models", "{id}", "default"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "memory"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "settings", "memory"}, role: RoleOperator},
	{method: "PATCH", segments: []string{"v0", "settings", "memory", "{id}"}, role: RoleOperator},
	{method: "DELETE", segments: []string{"v0", "settings", "memory", "{id}"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "settings", "schedules"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "settings", "schedules"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "settings", "schedules", "{id}", "pause"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "settings", "schedules", "{id}", "resume"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "settings", "schedules", "{id}", "run"}, role: RoleOperator},
	{method: "DELETE", segments: []string{"v0", "settings", "schedules", "{id}"}, role: RoleOperator},
}

// ViewerAllowedRead lists admin-level read endpoints that a viewer may call.
// It keeps operators out of cost/audit data while still letting a read-only
// auditor see them.
func ViewerAllowedRead(method, path string) bool {
	if method != "GET" {
		return false
	}
	path = normalizePath(path)
	if path == "/v0/audit/events" || path == "/v0/audit/export" {
		return true
	}
	return strings.HasPrefix(path, "/v0/settings/cost-")
}

func MinRole(method, path string) Role {
	path = normalizePath(path)
	method = strings.ToUpper(method)

	if method == "GET" && path == "/v0/ui-config" {
		return RoleNone
	}
	if path == "/v0/mcp/export" || strings.HasPrefix(path, "/v0/mcp/export/") {
		return RoleNone
	}
	if strings.HasPrefix(path, "/v0/auth/hosted/") {
		return RoleNone
	}
	// Prometheus scrape endpoint (non-/v0 root path); documented explicitly.
	if method == "GET" && path == "/metrics" {
		return RoleAdmin
	}
	if !strings.HasPrefix(path, "/v0") {
		return RoleAdmin
	}

	segs := splitPath(path)
	for _, rule := range aclRules {
		if rule.method != method {
			continue
		}
		if matchSegments(segs, rule.segments) {
			return rule.role
		}
	}
	return RoleAdmin
}

func normalizePath(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return path
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func matchSegments(pathSegs, ruleSegs []string) bool {
	if len(pathSegs) != len(ruleSegs) {
		return false
	}
	for i, rule := range ruleSegs {
		if strings.HasPrefix(rule, "{") && strings.HasSuffix(rule, "}") {
			if pathSegs[i] == "" {
				return false
			}
			continue
		}
		if pathSegs[i] != rule {
			return false
		}
	}
	return true
}
