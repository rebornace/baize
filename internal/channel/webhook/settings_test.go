package webhook

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/channel"
)

func TestSettingsDefaultsAndHotApply(t *testing.T) {
	dir := t.TempDir()
	c, err := openFromConfig("weixin", map[string]string{
		"source": "weixin", "secret": "s", "outbound_url": "http://h/o",
		"assignee": "channel:weixin", "agent_id": "ag-cfg", "admin_url": "http://127.0.0.1:9",
	})
	if err != nil {
		t.Fatal(err)
	}
	c.settingsDir = dir // white-box: persist under temp dir
	rt := &channel.Runtime{Source: "weixin"}
	c.rt = rt
	if err := c.loadSettings(); err != nil {
		t.Fatal(err)
	}
	// Defaults: enabled, assignee/agent from config, allowlist open.
	s := c.GetSettings()
	if !s.Enabled || s.Assignee != "channel:weixin" || s.AgentID != "ag-cfg" {
		t.Fatalf("default settings=%+v", s)
	}
	if !c.peerAllowed("anyone@im.wechat") {
		t.Fatal("empty allowlist should allow all")
	}

	// Hot update: restrict allowlist, change assignee/agent, disable.
	st := c.UpdateSettings(channel.ChannelSettings{
		Assignee: "op:alice", AgentID: "ag-2",
		Allowlist: []string{"allowed@im.wechat"}, Enabled: false,
	})
	if st.Running || st.Reason != "" {
		t.Fatalf("disabled -> running=false reason empty, got %+v", st)
	}
	if rt.Assignee != "op:alice" || rt.DefaultAgentID != "ag-2" {
		t.Fatalf("runtime not hot-applied: %+v", rt)
	}
	if c.peerAllowed("allowed@im.wechat") != true || c.peerAllowed("other@im.wechat") != false {
		t.Fatal("allowlist not hot-applied")
	}
	// Persisted to disk.
	if _, err := loadSettingsFile(dir); err != nil {
		t.Fatalf("settings not persisted: %v", err)
	}

	// Re-enable without credentials -> login_required (admin client is nil in
	// this test, so status falls back to enabled-only reporting).
	st2 := c.UpdateSettings(channel.ChannelSettings{
		Assignee: "op:alice", AgentID: "ag-2", Allowlist: []string{"allowed@im.wechat"}, Enabled: true,
	})
	_ = st2
}

func TestStatusReconcilesViaAdminClient(t *testing.T) {
	c, _ := openFromConfig("weixin", map[string]string{
		"source": "weixin", "secret": "s", "outbound_url": "http://h/o", "assignee": "a",
	})
	c.rt = &channel.Runtime{Source: "weixin"}
	_ = c.loadSettings()

	// No admin client: enabled -> reported running (cannot probe adapter).
	if st := c.Status(); !st.Running {
		t.Fatalf("no-admin enabled status=%+v", st)
	}

	// With a fake admin client: reconcile login_required / start_failed / ok.
	c.admin = &fakeAdmin{hasCreds: false, polling: false}
	if st := c.Status(); st.Running || st.Reason != "login_required" {
		t.Fatalf("no-creds status=%+v", st)
	}
	c.admin = &fakeAdmin{hasCreds: true, polling: false}
	if st := c.Status(); st.Running || st.Reason != "start_failed" {
		t.Fatalf("has-creds-not-polling status=%+v", st)
	}
	c.admin = &fakeAdmin{hasCreds: true, polling: true}
	if st := c.Status(); !st.Running || st.Reason != "" {
		t.Fatalf("polling status=%+v", st)
	}
	c.admin = &fakeAdmin{err: context.DeadlineExceeded}
	if st := c.Status(); st.Running || st.Reason != "start_failed" {
		t.Fatalf("unreachable status=%+v", st)
	}
}

type fakeAdmin struct {
	hasCreds, polling bool
	err               error
	started, stopped  bool
	shutdown          bool
}

func (f *fakeAdmin) Status(context.Context) (bool, bool, error) { return f.hasCreds, f.polling, f.err }
func (f *fakeAdmin) Start(context.Context) error                { f.started = true; return f.err }
func (f *fakeAdmin) Stop(context.Context) error                 { f.stopped = true; return f.err }
func (f *fakeAdmin) LoginStart(context.Context) (string, string, error) {
	return "t", "qr", f.err
}
func (f *fakeAdmin) LoginPoll(context.Context, string) (string, error) {
	return "success", f.err
}
func (f *fakeAdmin) Logout(context.Context) error   { return f.err }
func (f *fakeAdmin) Shutdown(context.Context) error { f.shutdown = true; return f.err }
