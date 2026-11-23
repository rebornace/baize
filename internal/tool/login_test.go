package tool_test

import (
	"testing"

	"github.com/rebornace/baize/internal/tool"
)

func TestMatchAuthFailureBuiltinEmpty(t *testing.T) {
	cases := []struct {
		name    string
		content map[string]any
		want    bool
	}{
		{"http_401", map[string]any{"http_status": 401}, true},
		{"http_403", map[string]any{"http_status": float64(403)}, true},
		{"http_400", map[string]any{"http_status": 400, "message": "bad request"}, false},
		{"http_404", map[string]any{"http_status": 404}, false},
		{"code_401", map[string]any{"code": 401}, true},
		{"code_unauthorized", map[string]any{"code": "Unauthorized"}, true},
		{"msg_未登录", map[string]any{"message": "用户未登录"}, true},
		{"plain", map[string]any{"message": "not found"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tool.MatchAuthFailure(tc.content, tool.LoginSignals{}); got != tc.want {
				t.Fatalf("got %v want %v for %+v", got, tc.want, tc.content)
			}
			if got := tool.LooksLikeAuthFailure(tc.content); got != tc.want {
				t.Fatalf("LooksLikeAuthFailure got %v want %v", got, tc.want)
			}
		})
	}
}

func TestMatchAuthFailureWeakStatusNeedsSignal(t *testing.T) {
	signals := tool.LoginSignals{
		HTTPStatusesNeedSignal: []int{400, 404},
		BodyCodes:              []string{"401", "AUTH_REQUIRED"},
		MessageContains:        []string{"未登录", "请登录"},
	}
	cases := []struct {
		name    string
		content map[string]any
		want    bool
	}{
		{"400_alone", map[string]any{"http_status": 400, "message": "bad request"}, false},
		{"404_alone", map[string]any{"http_status": 404}, false},
		{"400_with_body", map[string]any{"http_status": 400, "code": 401}, true},
		{"404_with_msg", map[string]any{"http_status": 404, "message": "请登录后重试"}, true},
		{"200_body", map[string]any{"code": "AUTH_REQUIRED"}, true},
		{"401_default_status", map[string]any{"http_status": 401}, true},
		{"400_wrong_body", map[string]any{"http_status": 400, "code": "invalid_param"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tool.MatchAuthFailure(tc.content, signals); got != tc.want {
				t.Fatalf("got %v want %v for %+v", got, tc.want, tc.content)
			}
		})
	}
}

func TestMatchAuthFailureCustomStatusesAlone(t *testing.T) {
	signals := tool.LoginSignals{HTTPStatuses: []int{401, 418}}
	if !tool.MatchAuthFailure(map[string]any{"http_status": 418}, signals) {
		t.Fatal("expected 418 alone to match")
	}
	if tool.MatchAuthFailure(map[string]any{"http_status": 403}, signals) {
		t.Fatal("403 should not match when statuses replaced without 403")
	}
}

func TestMatchAuthFailureAmbiguousStatusesNotAlone(t *testing.T) {
	// Misconfig seen in production: 400/404 listed as alone statuses.
	signals := tool.LoginSignals{HTTPStatuses: []int{400, 401, 403, 404}}
	if tool.MatchAuthFailure(map[string]any{"http_status": 400, "message": "bad request"}, signals) {
		t.Fatal("plain 400 must not remap to login_required")
	}
	if tool.MatchAuthFailure(map[string]any{"http_status": 404, "message": "not found"}, signals) {
		t.Fatal("plain 404 must not remap to login_required")
	}
	if !tool.MatchAuthFailure(map[string]any{"http_status": 401}, signals) {
		t.Fatal("401 alone should still match")
	}
	if !tool.MatchAuthFailure(map[string]any{"http_status": 400, "code": 401}, signals) {
		t.Fatal("400 + default body code 401 should match via need-signal")
	}
	if !tool.MatchAuthFailure(map[string]any{"http_status": 404, "message": "未登录"}, signals) {
		t.Fatal("404 + 未登录 should match via need-signal")
	}
}

func TestMatchAuthFailureBodyPath(t *testing.T) {
	signals := tool.LoginSignals{
		BodyCodePath: "data.err_code",
		BodyCodes:    []string{"10001"},
	}
	if !tool.MatchAuthFailure(map[string]any{
		"data": map[string]any{"err_code": "10001"},
	}, signals) {
		t.Fatal("expected nested body code hit")
	}
}

func TestRemapAuthFailureContent(t *testing.T) {
	signals := tool.LoginSignals{HTTPStatusesNeedSignal: []int{400}, MessageContains: []string{"未登录"}}
	out, ok := tool.RemapAuthFailureContent(map[string]any{
		"http_status": 400,
		"message":     "未登录",
	}, signals, "crm")
	if !ok || !tool.IsLoginRequired(out) {
		t.Fatalf("remap failed: ok=%v out=%v", ok, out)
	}
	if out["connector_id"] != "crm" {
		t.Fatalf("connector_id=%v", out["connector_id"])
	}
	plain, ok := tool.RemapAuthFailureContent(map[string]any{
		"http_status": 400,
		"message":     "bad",
	}, signals, "crm")
	if ok || tool.IsLoginRequired(plain) {
		t.Fatalf("should not remap plain 400: %v", plain)
	}
}
