package inbox_test

import (
	"testing"

	"github.com/rebornace/baize/internal/inbox"
)

func TestRegistryGetDisabledChannel(t *testing.T) {
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "a", AgentID: "x", Secret: "s", Enabled: false}})
	_, ok := reg.Get("a")
	if ok {
		t.Fatal("disabled must not be returned for inbound")
	}
}

func TestRegistryGetRequiresSecret(t *testing.T) {
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "a", AgentID: "x", Secret: "", Enabled: true}})
	_, ok := reg.Get("a")
	if ok {
		t.Fatal("empty secret must not be returned for inbound")
	}
}

func TestRegistryGetEnabledChannel(t *testing.T) {
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "a", AgentID: "x", Secret: "s", Enabled: true}})
	got, ok := reg.Get("a")
	if !ok || got.AgentID != "x" {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
}

func TestRegistryGetAnyIncludesDisabled(t *testing.T) {
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "a", AgentID: "x", Secret: "s", Enabled: false}})
	got, ok := reg.GetAny("a")
	if !ok || got.Enabled {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
	if _, ok := reg.Get("a"); ok {
		t.Fatal("Get must hide disabled")
	}
}

func TestSecretHint(t *testing.T) {
	if got := inbox.SecretHint("abcdefghij"); got != "ghij" {
		t.Fatalf("hint=%q want ghij", got)
	}
	if got := inbox.SecretHint("ab"); got != "ab" {
		t.Fatalf("short hint=%q want ab", got)
	}
}

func TestGenerateSecret(t *testing.T) {
	s1 := inbox.GenerateSecret()
	s2 := inbox.GenerateSecret()
	if s1 == "" || s2 == "" || s1 == s2 {
		t.Fatalf("secrets=%q %q", s1, s2)
	}
}
