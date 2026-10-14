package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type fakeSuggestLLM struct {
	reply string
}

func (f *fakeSuggestLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	return llm.Message{Role: llm.RoleAssistant, Content: f.reply}, nil
}

func (f *fakeSuggestLLM) SupportsVision() bool { return false }

func TestPostConnectorHostedLoginSuggestLLMUnavailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	srv.Identities = identity.NewMemoryStore()
	h := srv.Handler()

	putConnectorJSON(t, h, "hl-suggest", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
	})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/connectors/hl-suggest/hosted-login/suggest", nil))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"llm_unavailable"`) || !strings.Contains(body, "LLM is not configured") {
		t.Fatalf("body=%s", body)
	}
}

func TestPostConnectorHostedLoginSuggest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	srv.Identities = identity.NewMemoryStore()
	srv.LLM = &fakeSuggestLLM{reply: `{"suggestions":[{"id":"tool-login","label":"账号密码登录","recommend_default":true},{"id":"sec-bearer","label":"Bearer 令牌"}]}`}
	h := srv.Handler()

	putConnectorJSON(t, h, "hl-suggest-ok", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
	})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/connectors/hl-suggest-ok/hosted-login/suggest", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Suggestions []struct {
			ID               string `json:"id"`
			Label            string `json:"label"`
			RecommendDefault bool   `json:"recommend_default"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Suggestions) < 1 || body.Suggestions[0].Label != "账号密码登录" {
		t.Fatalf("suggestions=%+v", body.Suggestions)
	}

	got, err := st.GetConnector("hl-suggest-ok")
	if err != nil {
		t.Fatal(err)
	}
	if got.Auth.HostedLogin.Enabled || len(got.Auth.HostedLogin.Methods) > 0 {
		t.Fatalf("suggest must not write store: %+v", got.Auth.HostedLogin)
	}
}
