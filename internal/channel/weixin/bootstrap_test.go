package weixin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

// TestSettingsRoundTrip locks the JSON contract of the extracted
// settings file (settings.json next to creds.json).
func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Missing file: defaults are Enabled=true with an empty (non-nil) allowlist.
	got, err := LoadSettings(dir)
	if err != nil {
		t.Fatalf("LoadSettings missing: %v", err)
	}
	if !got.Enabled {
		t.Fatal("missing settings.json should default to enabled=true")
	}
	if got.Allowlist == nil || len(got.Allowlist) != 0 {
		t.Fatalf("default allowlist should be empty non-nil, got %v", got.Allowlist)
	}

	want := Settings{
		AgentID:   "agent-wx",
		Allowlist: []string{"peer-a", "peer-b"},
		Assignee:  "bob",
		Enabled:   false,
	}
	if err := SaveSettings(dir, want); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, settingsFileName))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	// JSON field names must stay identical to the old api-side contract.
	body := string(data)
	for _, key := range []string{`"agent_id"`, `"allowlist"`, `"assignee"`, `"enabled"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("settings.json missing %s: %s", key, body)
		}
	}

	got, err = LoadSettings(dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got.AgentID != want.AgentID || got.Assignee != want.Assignee || got.Enabled != want.Enabled {
		t.Fatalf("settings mismatch: got %+v want %+v", got, want)
	}
	if len(got.Allowlist) != 2 || got.Allowlist[0] != "peer-a" || got.Allowlist[1] != "peer-b" {
		t.Fatalf("allowlist mismatch: %v", got.Allowlist)
	}
}

// TestChannelBootstrap verifies the channel assembles its own Runtime from
// persisted settings: source pinning, assignee/agent defaults, allowlist and
// credentials wiring, and the start decision (enabled && creds present).
func TestChannelBootstrap(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSettings(dir, Settings{
		AgentID:   "agent-wx",
		Allowlist: []string{"peer-allowed"},
		Assignee:  "bob",
		Enabled:   true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveCreds(dir, "acct-1", "tok-1"); err != nil {
		t.Fatal(err)
	}

	ch := &Channel{ilink: NewFake(), emptyPollWait: defaultEmptyPollWait}
	ch.SetCredsDir(dir)

	deps := channel.BuildDeps{
		Store:          store.NewMemory(),
		Meta:           conversation.NewMemoryStore(),
		Messages:       conversation.NewMemoryStore(),
		DefaultAgentID: "agent-default",
	}
	rt, credsDir, start, err := ch.Bootstrap(deps)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if credsDir != dir {
		t.Fatalf("credsDir=%q want %q", credsDir, dir)
	}
	if !start {
		t.Fatal("enabled settings + creds should report start=true")
	}
	if rt.Source != SourceName {
		t.Fatalf("rt.Source=%q want %q", rt.Source, SourceName)
	}
	if rt.Assignee != "bob" {
		t.Fatalf("assignee=%q want bob", rt.Assignee)
	}
	if rt.DefaultAgentID != "agent-wx" {
		t.Fatalf("agentID=%q want agent-wx (from settings)", rt.DefaultAgentID)
	}
	if rt.Runs != deps.Store || rt.Meta != deps.Meta || rt.Messages != deps.Messages {
		t.Fatal("runtime deps not wired")
	}
	if !ch.HasCredentials() {
		t.Fatal("credentials should be loaded by Bootstrap")
	}
	if !ch.peerAllowed("peer-allowed") {
		t.Fatal("allowlist should permit peer-allowed")
	}
	if ch.peerAllowed("peer-other") {
		t.Fatal("allowlist should drop peer-other")
	}
}

// TestChannelBootstrapDefaults covers empty settings: default assignee,
// default agent from deps, no creds -> start=false.
func TestChannelBootstrapDefaults(t *testing.T) {
	dir := t.TempDir() // no settings.json, no creds.json
	ch := &Channel{ilink: NewFake(), emptyPollWait: defaultEmptyPollWait}
	ch.SetCredsDir(dir)

	rt, _, start, err := ch.Bootstrap(channel.BuildDeps{
		Store:          store.NewMemory(),
		Meta:           conversation.NewMemoryStore(),
		Messages:       conversation.NewMemoryStore(),
		DefaultAgentID: "agent-default",
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if start {
		t.Fatal("missing creds must report start=false")
	}
	if rt.Assignee != "channel:weixin" {
		t.Fatalf("default assignee=%q", rt.Assignee)
	}
	if rt.DefaultAgentID != "agent-default" {
		t.Fatalf("default agentID=%q", rt.DefaultAgentID)
	}
	if ch.HasCredentials() {
		t.Fatal("no creds on disk: channel must not hold credentials")
	}
}

var _ channel.Bootstrapper = (*Channel)(nil)
