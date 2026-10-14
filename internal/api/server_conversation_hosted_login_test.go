package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestPostConversationHostedLogin(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginConfig{
				Enabled: true, DefaultMethodID: "default",
				Methods: []store.HostedLoginMethod{{
					ID: "default", LoginTool: "login", Mode: hostedlogin.ModePasswordForm,
					Fields: []store.HostedLoginField{
						{Name: "user", Arg: "username", Input: "text"},
						{Name: "pass", Arg: "password", Input: "password"},
					},
				}},
			},
		},
	})
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	srv.CallbackPublicBase = "http://127.0.0.1:8080"
	h := srv.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/conversations/conv1/hosted-login",
		jsonBody(t, map[string]any{"connector_id": "mall"})))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		URL       string `json:"url"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(body.URL, "http://127.0.0.1:8080/v0/auth/hosted/") {
		t.Fatalf("url=%q", body.URL)
	}
	sess, ok := srv.HostedSessions.Get(body.SessionID)
	if !ok {
		t.Fatal("session missing")
	}
	if sess.ConversationID != "conv1" {
		t.Fatalf("conversation_id=%q", sess.ConversationID)
	}
	if sess.WorkspaceID == "" {
		t.Fatal("workspace_id must be set so submit persists into the Accounts pool")
	}
}
