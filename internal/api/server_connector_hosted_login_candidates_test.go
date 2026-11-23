package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
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

func TestPostConnectorHostedLoginCandidatesBlobSpec(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	srv.Identities = identity.NewMemoryStore()
	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatalf("blob: %v", err)
	}
	srv.Blobs = blobs
	h := srv.Handler()

	specYAML, err := os.ReadFile(writeLoginGetMeAPISpec(t))
	if err != nil {
		t.Fatal(err)
	}
	putConnectorJSON(t, h, "hl-cand-blob", map[string]any{
		"type":         "openapi",
		"spec_content": string(specYAML),
		"base_url":     upstream.URL,
	})
	conn, err := st.GetConnector("hl-cand-blob")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(conn.Spec, "connectors/") {
		t.Fatalf("expected blob spec key, got %q", conn.Spec)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/connectors/hl-cand-blob/hosted-login/candidates", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Candidates []store.HostedLoginMethod `json:"candidates"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var secCand bool
	for _, c := range body.Candidates {
		if c.ID == "sec-bearer" {
			secCand = true
			break
		}
	}
	if !secCand {
		t.Fatalf("blob-backed connector must still infer security candidates: %+v", body.Candidates)
	}
}
