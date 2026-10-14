package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestPostConnectorHostedLoginCandidates(t *testing.T) {
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

	putConnectorJSON(t, h, "hl-cand", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
	})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/connectors/hl-cand/hosted-login/candidates", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Candidates []store.HostedLoginMethod `json:"candidates"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Candidates) < 2 {
		t.Fatalf("expected tool + security candidates, got %+v", body.Candidates)
	}
	var toolCand, secCand bool
	for _, c := range body.Candidates {
		switch c.ID {
		case "tool-login":
			toolCand = true
			if c.Mode != hostedlogin.ModePasswordForm {
				t.Fatalf("login tool candidate: %+v", c)
			}
		case "sec-bearer":
			secCand = true
			if c.Mode != hostedlogin.ModeTokenPaste {
				t.Fatalf("bearer candidate: %+v", c)
			}
		}
	}
	if !toolCand || !secCand {
		t.Fatalf("missing expected ids: %+v", body.Candidates)
	}
}
