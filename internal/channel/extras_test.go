package channel

import "testing"

func TestWithOutboundMetaSetsKindAndRunID(t *testing.T) {
	got := WithOutboundMeta(map[string]string{"context_token": "tok"}, OutboundKindNotify, "run-9")
	if got[ExtraKind] != OutboundKindNotify {
		t.Fatalf("kind=%q want %q", got[ExtraKind], OutboundKindNotify)
	}
	if got[ExtraRunID] != "run-9" {
		t.Fatalf("run_id=%q want run-9", got[ExtraRunID])
	}
	if got["context_token"] != "tok" {
		t.Fatalf("existing extras must be preserved: %+v", got)
	}
}

func TestWithOutboundMetaDoesNotMutateInput(t *testing.T) {
	src := map[string]string{"context_token": "tok"}
	_ = WithOutboundMeta(src, OutboundKindAssistant, "r")
	if _, ok := src[ExtraKind]; ok {
		t.Fatal("input map must not be mutated")
	}
}

func TestWithOutboundMetaNilAndEmpty(t *testing.T) {
	got := WithOutboundMeta(nil, "", "")
	if len(got) != 0 {
		t.Fatalf("empty kind/runID on nil should yield empty map, got %+v", got)
	}
	got = WithOutboundMeta(nil, OutboundKindOperator, "")
	if got[ExtraKind] != OutboundKindOperator {
		t.Fatalf("kind=%q want operator", got[ExtraKind])
	}
	if _, ok := got[ExtraRunID]; ok {
		t.Fatal("empty runID must not be set")
	}
}
