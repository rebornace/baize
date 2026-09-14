package identity_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/identity"
)

const testJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbkB4LmNvbSIsImVtYWlsIjoiYWRtaW5AeC5jb20iLCJyb2xlcyI6WyJhZG1pbiJdLCJleHAiOjk5OTk5OTk5OTl9.sig"

func TestCaptureAccessToken(t *testing.T) {
	cfg := identity.CaptureConfig{
		ToolNameGlob:   "*login*",
		TokenJSONPaths: []string{"accessToken", "data.token"},
		LabelJSONPaths: []string{"email"},
		HeaderTemplate: "Bearer {{token}}",
		DefaultScheme:  "bearer",
	}
	if !identity.MatchToolName(cfg.ToolNameGlob, "AdminAuthController_login") {
		t.Fatal("glob")
	}
	headers, label, subject, claims, ok := identity.ExtractCredential(cfg, map[string]any{
		"accessToken": testJWT,
		"email":       "admin@x.com",
	})
	if !ok || headers["Authorization"] == "" || label == "" {
		t.Fatalf("extract: %+v %q %v", headers, label, ok)
	}
	if subject != "admin@x.com" {
		t.Fatalf("subject=%q", subject)
	}
	if claims["sub"] != "admin@x.com" {
		t.Fatalf("claims sub=%v", claims["sub"])
	}
	roles, _ := claims["roles"].([]any)
	if len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("claims roles=%v", claims["roles"])
	}
}

func TestCaptureNestedTokenPath(t *testing.T) {
	cfg := identity.CaptureConfig{
		TokenJSONPaths: []string{"data.token"},
		LabelJSONPaths: []string{"data.email"},
		HeaderTemplate: "Bearer {{token}}",
	}
	headers, label, _, _, ok := identity.ExtractCredential(cfg, map[string]any{
		"data": map[string]any{
			"token": testJWT,
			"email": "nested@x.com",
		},
	})
	if !ok || headers["Authorization"] != "Bearer "+testJWT || label != "nested@x.com" {
		t.Fatalf("nested extract: headers=%+v label=%q ok=%v", headers, label, ok)
	}
}

func TestCaptureTokenAlreadyBearer(t *testing.T) {
	cfg := identity.CaptureConfig{
		TokenJSONPaths: []string{"accessToken"},
		HeaderTemplate: "Bearer {{token}}",
	}
	token := "Bearer " + testJWT
	headers, _, _, _, ok := identity.ExtractCredential(cfg, map[string]any{
		"accessToken": token,
	})
	if !ok || headers["Authorization"] != token {
		t.Fatalf("bearer passthrough: headers=%+v ok=%v", headers, ok)
	}
}

func TestCaptureTokenValueAndNestedObject(t *testing.T) {
	cfg := identity.CaptureConfig{HeaderTemplate: "Bearer {{token}}"}
	headers, label, _, _, ok := identity.ExtractCredential(cfg, map[string]any{
		"code": 200,
		"data": map[string]any{
			"tokenName":  "satoken",
			"tokenValue": testJWT,
			"user":       map[string]any{"email": "admin@miao.com"},
		},
	})
	if !ok || headers["Authorization"] != "Bearer "+testJWT {
		t.Fatalf("tokenValue extract: headers=%+v ok=%v", headers, ok)
	}
	if label != "admin@miao.com" {
		t.Fatalf("label=%q", label)
	}

	headers, _, _, _, ok = identity.ExtractCredential(cfg, map[string]any{
		"data": map[string]any{
			"token": map[string]any{"accessToken": testJWT},
		},
	})
	if !ok || headers["Authorization"] != "Bearer "+testJWT {
		t.Fatalf("nested token object: headers=%+v ok=%v", headers, ok)
	}

	headers, _, _, _, ok = identity.ExtractCredential(cfg, map[string]any{
		"access_token": testJWT,
	})
	if !ok || headers["Authorization"] != "Bearer "+testJWT {
		t.Fatalf("access_token: headers=%+v ok=%v", headers, ok)
	}
}

func TestCaptureNoToken(t *testing.T) {
	cfg := identity.CaptureConfig{
		TokenJSONPaths: []string{"accessToken"},
		HeaderTemplate: "Bearer {{token}}",
	}
	_, _, _, _, ok := identity.ExtractCredential(cfg, map[string]any{"email": "a@b.com"})
	if ok {
		t.Fatal("expected false when token missing")
	}
}

func TestParseJWTClaimsSummaryInvalid(t *testing.T) {
	claims := identity.ParseJWTClaimsSummary("not-a-jwt")
	if len(claims) != 0 {
		t.Fatalf("expected empty claims, got %v", claims)
	}
}

func TestMatchToolName(t *testing.T) {
	if !identity.MatchToolName("*login*", "AdminAuthController_login") {
		t.Fatal("expected match")
	}
	if identity.MatchToolName("*login*", "AdminAuthController_logout") {
		t.Fatal("expected no match")
	}
	// OpenAPI-style PascalCase *Login* must match default *login* glob.
	if !identity.MatchToolName("*login*", "AuthController_phoneLogin") {
		t.Fatal("expected case-insensitive match for phoneLogin")
	}
	if !identity.MatchToolName("*login*", "AuthController_phonePasswordLogin") {
		t.Fatal("expected case-insensitive match for phonePasswordLogin")
	}
}

func TestContextKeys(t *testing.T) {
	ctx := context.Background()
	ctx = identity.WithConversationID(ctx, "conv-123")
	ctx = identity.WithForceIdentityID(ctx, "idt_abc")
	if got := identity.ConversationIDFrom(ctx); got != "conv-123" {
		t.Fatalf("conversationID=%q", got)
	}
	if got := identity.ForceIdentityIDFrom(ctx); got != "idt_abc" {
		t.Fatalf("forceIdentityID=%q", got)
	}
	empty := context.Background()
	if identity.ConversationIDFrom(empty) != "" || identity.ForceIdentityIDFrom(empty) != "" {
		t.Fatal("expected empty from bare context")
	}
}
