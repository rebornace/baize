package identity

import (
	"strings"
	"testing"
)

func TestCredentialFromUserTokenBearer(t *testing.T) {
	h, sub, claims := CredentialFromUserToken("Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1MUB4LmNvbSJ9.sig")
	if h == nil || h["Authorization"] != "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1MUB4LmNvbSJ9.sig" {
		t.Fatalf("headers=%v", h)
	}
	if sub != "u1@x.com" {
		t.Fatalf("subject=%q", sub)
	}
	if claims["sub"] != "u1@x.com" {
		t.Fatalf("claims=%v", claims)
	}
}

func TestExtractAndStripTokenLine(t *testing.T) {
	in := "token: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhIn0.sig\n设备号：CDB9EB2210D3"
	cleaned, h, ok := ExtractAndStripToken(in)
	if !ok || h == nil {
		t.Fatal("expected token extracted")
	}
	if !strings.Contains(cleaned, "CDB9EB2210D3") {
		t.Fatalf("cleaned=%q", cleaned)
	}
	if strings.Contains(cleaned, "eyJ") {
		t.Fatalf("token should be stripped: %q", cleaned)
	}
}

func TestExtractAndStripTokenMidSentence(t *testing.T) {
	// Real user habit: paste JWT inline after Chinese prose.
	tok := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOjk3LCJpYXQiOjE3NzkwNjg3ODcsImV4cCI6MTgxMDYwNDc4N30.2TDbhE4M--Ncjvi5vVbElIQa02s44X0YeFiMOCVoiIw"
	in := "我直接给你token " + tok
	cleaned, h, ok := ExtractAndStripToken(in)
	if !ok || h == nil {
		t.Fatal("expected mid-sentence JWT extracted")
	}
	if !strings.HasPrefix(h["Authorization"], "Bearer ") {
		t.Fatalf("auth=%q", h["Authorization"])
	}
	if strings.Contains(cleaned, "eyJ") {
		t.Fatalf("jwt left in cleaned: %q", cleaned)
	}
}

func TestPrepareSessionAuthAddsHint(t *testing.T) {
	st := NewMemoryStore()
	tok := "eyJhbGciOiJIUzI1NiJ9.eyJ1c2VySWQiOjF9.sigABCDEF"
	cleaned, id, err := PrepareSessionAuth(st, "c1", "我直接给你token "+tok+"\n查一下 step", "")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("missing identity id")
	}
	if !strings.Contains(cleaned, SessionAuthReadyHint) {
		t.Fatalf("cleaned missing hint: %q", cleaned)
	}
	if !strings.Contains(cleaned, "step") {
		t.Fatalf("cleaned lost intent: %q", cleaned)
	}
	if !ConversationHasSessionAuth(st, "c1") {
		t.Fatal("expected session auth")
	}
}
