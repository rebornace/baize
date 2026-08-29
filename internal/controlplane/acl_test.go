package controlplane

import "testing"

func TestMinRoleTable(t *testing.T) {
	cases := []struct {
		method, path string
		want         Role
	}{
		{"GET", "/v0/ui-config", RoleNone},
		{"GET", "/v0/me", RoleOperator},
		{"POST", "/v0/runs", RoleOperator},
		{"POST", "/v0/runs/r1/resume", RoleOperator},
		{"POST", "/v0/runs/r1/plugin-callbacks", RoleNone},
		{"GET", "/v0/runs/r1", RoleOperator},
		{"GET", "/v0/runs/r1/events", RoleOperator},
		{"GET", "/v0/runs/r1/stream", RoleOperator},
		{"GET", "/v0/artifacts/art_1", RoleOperator},
		{"GET", "/v0/conversations", RoleOperator},
		{"GET", "/v0/conversations/c1/messages", RoleOperator},
		{"DELETE", "/v0/conversations/c1/messages", RoleOperator},
		{"GET", "/v0/conversations/c1/identities", RoleOperator},
		{"POST", "/v0/conversations/c1/identities", RoleOperator},
		{"POST", "/v0/conversations/c1/identities/i1/default", RoleOperator},
		{"DELETE", "/v0/conversations/c1/identities/i1", RoleOperator},
		{"DELETE", "/v0/conversations/c1/identities", RoleOperator},
		{"PUT", "/v0/agents/a1", RoleAdmin},
		{"PUT", "/v0/connectors/c1", RoleAdmin},
		{"GET", "/v0/connectors/c1", RoleAdmin},
		{"GET", "/v0/tools", RoleAdmin},
		{"PATCH", "/v0/tools/create_ticket", RoleAdmin},
		{"POST", "/v0/connectors/c1/tools", RoleAdmin},
		{"DELETE", "/v0/connectors/c1/tools/extra1", RoleAdmin},
		{"DELETE", "/v0/connectors/c1", RoleAdmin},
		{"GET", "/v0/skills", RoleOperator},
		{"GET", "/v0/skills/x", RoleAdmin},
		{"POST", "/v0/skills", RoleAdmin},
		{"DELETE", "/v0/skills/x", RoleAdmin},
		{"GET", "/v0/agents/ticket-agent", RoleAdmin},
		{"POST", "/v0/inbox/alerts", RoleNone},
		{"GET", "/v0/settings/inbox-channels", RoleAdmin},
		{"PUT", "/v0/settings/inbox-channels", RoleAdmin},
		{"POST", "/v0/settings/inbox-channels/alerts/rotate-secret", RoleAdmin},
		{"POST", "/v0/settings/inbox-channels/alerts/test", RoleAdmin},
		{"POST", "/v0/settings/channels/weixin/login/start", RoleAdmin},
		{"GET", "/v0/settings/channels/weixin/login/status", RoleAdmin},
		{"POST", "/v0/settings/channels/weixin/logout", RoleAdmin},
		{"GET", "/v0/settings/channels/weixin", RoleAdmin},
		{"PUT", "/v0/settings/channels/weixin", RoleAdmin},
		{"GET", "/v0/unknown", RoleAdmin},
		{"POST", "/v0/runs/r1/resume/extra", RoleAdmin},
	}
	for _, tc := range cases {
		got := MinRole(tc.method, tc.path)
		if got != tc.want {
			t.Errorf("%s %s: got %q want %q", tc.method, tc.path, got, tc.want)
		}
	}
}
