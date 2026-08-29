package controlplane

import "strings"

type routeRule struct {
	method   string
	segments []string
	role     Role
}

var aclRules = []routeRule{
	{method: "GET", segments: []string{"v0", "me"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "runs"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "runs", "{id}", "resume"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "runs", "{id}", "plugin-callbacks"}, role: RoleNone},
	{method: "GET", segments: []string{"v0", "runs", "{id}", "stream"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "runs", "{id}", "events"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "runs", "{id}"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "artifacts", "{id}"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "conversations"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "conversations", "{id}", "messages"}, role: RoleOperator},
	{method: "DELETE", segments: []string{"v0", "conversations", "{id}", "messages"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "conversations", "{id}", "identities"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "conversations", "{id}", "identities", "{iid}", "default"}, role: RoleOperator},
	{method: "DELETE", segments: []string{"v0", "conversations", "{id}", "identities", "{iid}"}, role: RoleOperator},
	{method: "DELETE", segments: []string{"v0", "conversations", "{id}", "identities"}, role: RoleOperator},
	{method: "PUT", segments: []string{"v0", "agents", "{id}"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "connectors", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "connectors", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "tools"}, role: RoleAdmin},
	{method: "PATCH", segments: []string{"v0", "tools", "{name}"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "connectors", "{id}", "tools"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "connectors", "{id}", "tools", "{name}"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "connectors", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "skills"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "skills", "{id}"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "skills"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "skills", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "agents", "{id}"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "events-webhook"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "settings", "events-webhook"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "events-webhook", "test"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "events-webhook", "deliveries"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "events-webhook", "deliveries", "{id}", "retry"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "inbox", "{id}"}, role: RoleNone},
	{method: "GET", segments: []string{"v0", "settings", "inbox-channels"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "settings", "inbox-channels"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "inbox-channels", "{id}", "rotate-secret"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "inbox-channels", "{id}", "test"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "store"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "settings", "store"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "store", "restart"}, role: RoleAdmin},
}

func MinRole(method, path string) Role {
	path = normalizePath(path)
	method = strings.ToUpper(method)

	if method == "GET" && path == "/v0/ui-config" {
		return RoleNone
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
