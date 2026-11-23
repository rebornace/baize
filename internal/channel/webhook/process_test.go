package webhook

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/channel"
)

// newProcessChannel builds a Channel wired to a real supervised fake adapter
// binary plus a fake admin client. Settings start disabled so reconcileEnabled
// only ever drives the (no-op) admin Stop.
func newProcessChannel(t *testing.T, port string) (*Channel, *fakeAdmin) {
	t.Helper()
	exe := buildFakeAdapter(t)
	addr := "127.0.0.1:" + port
	adm := &fakeAdmin{}
	c := &Channel{
		cfg: instanceConfig{Name: "weixin"},
		settings: channel.ChannelSettings{
			Enabled:   false,
			Allowlist: []string{},
		},
		admin: adm,
		sup: &supervisor{
			command:    exe,
			healthzURL: "http://" + addr + "/healthz",
			env:        []string{"FAKE_ADDR=" + addr},
			timeout:    10 * time.Second,
		},
	}
	return c, adm
}

func healthzReachable(url string) bool {
	cl := &http.Client{Timeout: 300 * time.Millisecond}
	r, err := cl.Get(url)
	if err != nil {
		return false
	}
	r.Body.Close()
	return r.StatusCode == http.StatusOK
}

func TestChannelStartStopProcess(t *testing.T) {
	c, _ := newProcessChannel(t, "18095")
	ctx := context.Background()

	if err := c.StartProcess(ctx); err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	if !c.sup.running() {
		t.Fatal("adapter process not running after StartProcess")
	}
	if !healthzReachable(c.sup.healthzURL) {
		t.Fatal("healthz not reachable after StartProcess")
	}

	if err := c.StopProcess(ctx); err != nil {
		t.Fatalf("StopProcess: %v", err)
	}
	if c.sup.running() {
		t.Fatal("adapter process still tracked after StopProcess")
	}
	// Port must be released (no orphan).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !healthzReachable(c.sup.healthzURL) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("healthz still reachable after StopProcess")
}

func TestChannelRestartProcess(t *testing.T) {
	c, _ := newProcessChannel(t, "18094")
	ctx := context.Background()
	if err := c.StartProcess(ctx); err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	if err := c.RestartProcess(ctx); err != nil {
		t.Fatalf("RestartProcess: %v", err)
	}
	if !c.sup.running() {
		t.Fatal("adapter process not running after RestartProcess")
	}
	if !healthzReachable(c.sup.healthzURL) {
		t.Fatal("healthz not reachable after RestartProcess")
	}
	if err := c.StopProcess(ctx); err != nil {
		t.Fatalf("cleanup StopProcess: %v", err)
	}
}

// TestChannelStopAdoptedOrphanShutsDown: when baize adopted an orphan (no PID
// handle), StopProcess must ask it to exit via admin.Shutdown rather than
// killing a process it does not own.
func TestChannelStopAdoptedOrphanShutsDown(t *testing.T) {
	adm := &fakeAdmin{}
	c := &Channel{
		cfg:      instanceConfig{Name: "weixin"},
		settings: channel.ChannelSettings{Enabled: false, Allowlist: []string{}},
		admin:    adm,
		sup: &supervisor{
			healthzURL: "http://127.0.0.1:1/healthz", // nothing listening: waitUntilDown returns at once
			adopted:    true,
		},
	}
	if err := c.StopProcess(context.Background()); err != nil {
		t.Fatalf("StopProcess adopted: %v", err)
	}
	if !adm.shutdown {
		t.Fatal("expected admin.Shutdown to be called for an adopted orphan")
	}
	if c.sup.isAdopted() || c.sup.cmd != nil {
		t.Fatal("supervisor state should be reset for respawn after stop")
	}
}

// TestStatusReportsRestartingWhileBackingOff: while the watchdog is parked in
// a crash backoff (no live child), Status() must report {Running:false,
// Reason:"restarting"} without probing the dead admin port.
func TestStatusReportsRestartingWhileBackingOff(t *testing.T) {
	c := &Channel{
		cfg:      instanceConfig{Name: "weixin"},
		settings: channel.ChannelSettings{Enabled: true, Allowlist: []string{}},
		admin:    &fakeAdmin{},
		sup:      &supervisor{},
	}
	c.sup.backingOff = true // watchdog parked in backoff
	st := c.Status()
	if st.Running || st.Reason != "restarting" {
		t.Fatalf("expected {Running:false Reason:restarting}, got %+v", st)
	}
}
