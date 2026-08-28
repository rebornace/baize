package inbox_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/inbox"
)

func TestChannelValidateRejectsBadID(t *testing.T) {
	c := inbox.Channel{ID: "Bad", AgentID: "a", Secret: "s"}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("err=%v", err)
	}
}

func TestPayloadValidateRequiresInput(t *testing.T) {
	p := inbox.Payload{}
	if err := p.Validate(); err == nil {
		t.Fatal("want input required")
	}
}
