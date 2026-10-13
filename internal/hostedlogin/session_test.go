package hostedlogin

import (
	"testing"
	"time"
)

func TestSessionIssueAndGet(t *testing.T) {
	s := NewMemorySessions(time.Minute)
	id, csrf, err := s.Issue(Session{
		ExportKeyID:  "k1",
		ConnectorID:  "mall",
		LoginTool:    "mall.login",
		Mode:         "password_form",
		IdentityID:   "mei_1",
		PublicOrigin: "http://127.0.0.1:8080",
	})
	if err != nil || id == "" || csrf == "" {
		t.Fatalf("id=%q csrf=%q err=%v", id, csrf, err)
	}
	got, ok := s.Get(id)
	if !ok || got.ConnectorID != "mall" || got.CSRF != csrf {
		t.Fatalf("%+v ok=%v", got, ok)
	}
	if got.ExportKeyID != "k1" || got.IdentityID != "mei_1" || got.LoginTool != "mall.login" {
		t.Fatalf("%+v", got)
	}
	if got.PublicOrigin != "http://127.0.0.1:8080" {
		t.Fatalf("origin=%q", got.PublicOrigin)
	}
}

func TestSessionExpire(t *testing.T) {
	s := NewMemorySessions(time.Millisecond)
	id, _, err := s.Issue(Session{ConnectorID: "mall", LoginTool: "x", IdentityID: "i", ExportKeyID: "k"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, ok := s.Get(id); ok {
		t.Fatal("expired")
	}
}

func TestSessionDelete(t *testing.T) {
	s := NewMemorySessions(time.Minute)
	id, _, err := s.Issue(Session{ConnectorID: "mall", LoginTool: "x", IdentityID: "i", ExportKeyID: "k"})
	if err != nil {
		t.Fatal(err)
	}
	s.Delete(id)
	if _, ok := s.Get(id); ok {
		t.Fatal("deleted session still present")
	}
}
