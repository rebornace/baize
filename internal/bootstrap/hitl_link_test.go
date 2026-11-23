package bootstrap

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/hitllink"
	"github.com/rebornace/baize/internal/store"
)

func TestNewHITLLinkURLFunc(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	wait := time.Now().UTC().Add(-time.Minute)
	p := &store.HITLPayload{Kind: store.HITLKindApproval, WaitingSince: wait}
	_ = st.SetHITL(r.ID, p)
	_ = st.UpdateRun(r.ID, store.StatusWaitingHuman, "", "")
	secret := []byte("link-secret")
	fn := newHITLLinkURLFunc(secret, func() string { return "https://baize.example/" }, 0, st)
	link := fn(r, p)
	if !strings.HasPrefix(link, "https://baize.example/v0/hitl/approvals/") {
		t.Fatalf("link=%q", link)
	}
	token := strings.TrimPrefix(link, "https://baize.example/v0/hitl/approvals/")
	claims, err := hitllink.Verify(secret, token, time.Now())
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.RunID != r.ID || claims.WaitUnixNano != wait.UnixNano() {
		t.Fatalf("claims=%+v", claims)
	}
	if got := claims.ExpiresAt.Sub(wait); got > hitllink.DefaultTTL || got < hitllink.DefaultTTL-time.Second {
		t.Fatalf("ttl=%v want ~%v", got, hitllink.DefaultTTL)
	}
}

func TestNewHITLLinkURLFuncSkips(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	wait := time.Now().UTC().Add(-time.Minute)
	p := &store.HITLPayload{Kind: store.HITLKindApproval, WaitingSince: wait}
	_ = st.SetHITL(r.ID, p)
	_ = st.UpdateRun(r.ID, store.StatusWaitingHuman, "", "")

	cases := []struct {
		name   string
		secret []byte
		base   func() string
		ttl    time.Duration
		kind   string
	}{
		{"no-base", []byte("s"), func() string { return "" }, time.Hour, store.HITLKindApproval},
		{"nil-base", []byte("s"), nil, time.Hour, store.HITLKindApproval},
		{"no-secret", nil, func() string { return "https://x" }, time.Hour, store.HITLKindApproval},
		{"login", []byte("s"), func() string { return "https://x" }, time.Hour, store.HITLKindLogin},
		{"child", []byte("s"), func() string { return "https://x" }, time.Hour, store.HITLKindWaitingChild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := *p
			payload.Kind = tc.kind
			fn := newHITLLinkURLFunc(tc.secret, tc.base, tc.ttl, st)
			if got := fn(r, &payload); got != "" {
				t.Fatalf("link=%q want empty", got)
			}
		})
	}
}

func TestNewHITLLinkURLFuncExpiredWait(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	wait := time.Now().UTC().Add(-25 * time.Hour)
	p := &store.HITLPayload{Kind: store.HITLKindApproval, WaitingSince: wait}
	_ = st.SetHITL(r.ID, p)
	_ = st.UpdateRun(r.ID, store.StatusWaitingHuman, "", "")
	fn := newHITLLinkURLFunc([]byte("s"), func() string { return "https://x" }, 24*time.Hour, st)
	if got := fn(r, p); got != "" {
		t.Fatalf("expired wait link=%q", got)
	}
}
