package mockticket

import (
	"testing"
)

func TestListenAddrDefault(t *testing.T) {
	t.Setenv("BAIZE_MOCK_TICKET_LISTEN", "")
	if got := ListenAddr(); got != ":18080" {
		t.Fatalf("ListenAddr()=%q want :18080", got)
	}
}

func TestListenAddrEnvOverride(t *testing.T) {
	t.Setenv("BAIZE_MOCK_TICKET_LISTEN", ":19091")
	if got := ListenAddr(); got != ":19091" {
		t.Fatalf("ListenAddr()=%q", got)
	}
}
