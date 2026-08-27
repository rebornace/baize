package mcp_test

import (
	"testing"

	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
)

func TestBuildCallMetaWithCallback(t *testing.T) {
	meta := mcpbridge.BuildCallMeta("run_1", "agent_1", "https://runtime.example/v0/runs/run_1/plugin-callbacks?token=abc")
	urls, ok := meta["io.baize/callback_urls"].(map[string]any)
	if !ok {
		t.Fatalf("meta=%+v", meta)
	}
	if urls["event"] != "https://runtime.example/v0/runs/run_1/plugin-callbacks?token=abc" {
		t.Fatalf("event=%v", urls["event"])
	}
	if meta["io.baize/run_id"] != "run_1" {
		t.Fatalf("run_id=%v", meta["io.baize/run_id"])
	}
}

func TestBuildCallMetaOmitsCallbackWhenEmpty(t *testing.T) {
	meta := mcpbridge.BuildCallMeta("run_1", "agent_1", "")
	if _, ok := meta["io.baize/callback_urls"]; ok {
		t.Fatalf("meta=%+v", meta)
	}
}
