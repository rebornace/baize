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

func TestPayloadValidateIdempotencyAndExternalIDLength(t *testing.T) {
	ok := inbox.Payload{Input: "hi", IdempotencyKey: "k", ExternalID: "ext"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	longKey := inbox.Payload{Input: "hi", IdempotencyKey: strings.Repeat("x", 129)}
	if err := longKey.Validate(); err == nil || !strings.Contains(err.Error(), "idempotency_key") {
		t.Fatalf("err=%v", err)
	}
	longExt := inbox.Payload{Input: "hi", ExternalID: strings.Repeat("y", 257)}
	if err := longExt.Validate(); err == nil || !strings.Contains(err.Error(), "external_id") {
		t.Fatalf("err=%v", err)
	}
}
