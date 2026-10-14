package hostedlogin_test

import (
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
)

func TestValidateConfigRequiresDefaultAmongMethods(t *testing.T) {
	err := hostedlogin.ValidateConfig(store.HostedLoginConfig{
		Enabled: true,
		Methods: []store.HostedLoginMethod{{
			ID: "pwd", Mode: hostedlogin.ModeBasic,
		}},
		// missing DefaultMethodID
	})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestValidateRecipePasswordForm(t *testing.T) {
	err := hostedlogin.ValidateMethod(store.HostedLoginMethod{
		Mode: "password_form", LoginTool: "mall.login",
		Fields: []store.HostedLoginField{{Name: "user", Arg: "username", Input: "text"}},
	})
	if err == nil {
		t.Fatal("want error: password_form needs a password field")
	}
}

func TestDraftFromCaptureTool(t *testing.T) {
	draft := hostedlogin.DraftRecipe("mall.login", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"username": map[string]any{"type": "string"},
			"password": map[string]any{"type": "string"},
		},
		"required": []any{"username", "password"},
	})
	m, ok := draft.DefaultMethod()
	if !ok || m.Mode != "password_form" || m.LoginTool != "mall.login" {
		t.Fatalf("%+v", draft)
	}
	if draft.Enabled {
		t.Fatal("draft must not auto-enable")
	}
}

func TestValidateRecipeBasicAndHeaderPairs(t *testing.T) {
	if err := hostedlogin.ValidateMethod(store.HostedLoginMethod{
		Mode: hostedlogin.ModeBasic,
	}); err != nil {
		t.Fatalf("empty fields should default: %v", err)
	}
	if err := hostedlogin.ValidateMethod(store.HostedLoginMethod{
		Mode: hostedlogin.ModeHeaderPairs,
	}); err == nil {
		t.Fatal("header_pairs without fields should fail")
	}
	if err := hostedlogin.ValidateMethod(store.HostedLoginMethod{
		Mode:   hostedlogin.ModeHeaderPairs,
		Fields: []store.HostedLoginField{{Name: "Authorization", Arg: "Authorization", Input: "password"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDraftRecipeFromSecurity(t *testing.T) {
	basic := hostedlogin.DraftRecipeFromSecurity(map[string]any{
		"basicAuth": map[string]any{"type": "http", "scheme": "basic"},
	})
	basicM, _ := basic.DefaultMethod()
	if basicM.Mode != hostedlogin.ModeBasic || basic.Enabled {
		t.Fatalf("%+v", basic)
	}
	bearer := hostedlogin.DraftRecipeFromSecurity(map[string]any{
		"bearer": map[string]any{"type": "http", "scheme": "bearer"},
	})
	bearerM, _ := bearer.DefaultMethod()
	if bearerM.Mode != hostedlogin.ModeTokenPaste {
		t.Fatalf("%+v", bearer)
	}
	apiKey := hostedlogin.DraftRecipeFromSecurity(map[string]any{
		"api_key": map[string]any{"type": "apiKey", "in": "header", "name": "X-API-Key"},
	})
	apiKeyM, _ := apiKey.DefaultMethod()
	if apiKeyM.Mode != hostedlogin.ModeHeaderPairs || len(apiKeyM.Fields) != 1 || apiKeyM.Fields[0].Arg != "X-API-Key" {
		t.Fatalf("%+v", apiKey)
	}
}
