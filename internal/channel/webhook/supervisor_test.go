package webhook

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeAdapterSource is a tiny HTTP server used as a supervised child process.
// It is compiled to a real binary (NOT `go run`) so that Process.Kill on
// Windows terminates the supervised process itself rather than leaving a
// orphaned grandchild holding the healthz port.
const fakeAdapterSource = `package main
import ("net/http"; "os")
func main() {
	addr := os.Getenv("FAKE_ADDR")
	if addr == "" { addr = "127.0.0.1:0" }
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request){ w.WriteHeader(200) })
	go http.ListenAndServe(addr, nil)
	select {}
}
`

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// buildFakeAdapter compiles fakeAdapterSource into a real executable in a
// temp directory and returns its path.
func buildFakeAdapter(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(fakeAdapterSource), 0o600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "fakeadapter"+exeSuffix())
	build := exec.Command("go", "build", "-o", exe, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build fake adapter: %v\n%s", err, out)
	}
	return exe
}

func TestSupervisorStartsAndStopsChild(t *testing.T) {
	exe := buildFakeAdapter(t)

	// Fixed loopback port for the child healthz. The child is a single
	// directly-exec'd binary, so kill releases the port with no orphan.
	addr := "127.0.0.1:18099"
	sup := &supervisor{
		command:    exe,
		args:       nil,
		healthzURL: "http://" + addr + "/healthz",
		env:        []string{"FAKE_ADDR=" + addr},
		timeout:    30 * time.Second,
	}
	ctx := context.Background()
	if err := sup.start(ctx); err != nil {
		t.Fatalf("supervisor start: %v", err)
	}
	if !sup.running() {
		t.Fatal("supervisor not running after start")
	}
	// The healthz endpoint must actually be served by the child.
	resp, err := http.Get(sup.healthzURL)
	if err != nil {
		t.Fatalf("healthz not reachable while running: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}
	if err := sup.stop(ctx); err != nil {
		t.Fatalf("supervisor stop: %v", err)
	}
	if sup.running() {
		t.Fatal("supervisor still running after stop")
	}
	// After kill the port must be released (proves no orphaned child).
	deadline := time.Now().Add(3 * time.Second)
	client := &http.Client{Timeout: 300 * time.Millisecond}
	for time.Now().Before(deadline) {
		r, err := client.Get(sup.healthzURL)
		if err != nil {
			return // port closed: child fully gone
		}
		r.Body.Close()
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("healthz still reachable after stop: child process was not killed")
}

func TestSupervisorEmptyCommand(t *testing.T) {
	sup := &supervisor{healthzURL: "http://127.0.0.1:1/healthz", timeout: time.Second}
	if err := sup.start(context.Background()); err == nil {
		t.Fatal("expected error for empty adapter command")
	}
}

// TestSupervisorAdoptsCompatibleOrphan: when an adapter is already answering
// healthz AND passes the signed compatibility check, start adopts it (no new
// process spawned).
func TestSupervisorAdoptsCompatibleOrphan(t *testing.T) {
	exe := buildFakeAdapter(t)
	addr := "127.0.0.1:18097"
	// Pre-launch a "orphan" adapter holding the port.
	orphan := exec.Command(exe)
	orphan.Env = append(os.Environ(), "FAKE_ADDR="+addr)
	if err := orphan.Start(); err != nil {
		t.Fatalf("start orphan: %v", err)
	}
	t.Cleanup(func() { _ = orphan.Process.Kill(); _, _ = orphan.Process.Wait() })
	// Wait for it to serve.
	deadline := time.Now().Add(5 * time.Second)
	cl := &http.Client{Timeout: 300 * time.Millisecond}
	for time.Now().Before(deadline) {
		if r, err := cl.Get("http://" + addr + "/healthz"); err == nil {
			r.Body.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	sup := &supervisor{
		command:    exe,
		healthzURL: "http://" + addr + "/healthz",
		env:        []string{"FAKE_ADDR=127.0.0.1:1"}, // a fresh spawn would fail to bind
		timeout:    5 * time.Second,
		compatible: func(ctx context.Context) bool { return true },
	}
	if err := sup.start(context.Background()); err != nil {
		t.Fatalf("start should adopt the compatible orphan: %v", err)
	}
	if sup.running() {
		t.Fatal("no new child should be spawned when adopting an orphan")
	}
}

// TestSupervisorRejectsStaleListener: a listener that fails the signed
// compatibility check must NOT be adopted (it holds an old/foreign secret).
func TestSupervisorRejectsStaleListener(t *testing.T) {
	exe := buildFakeAdapter(t)
	addr := "127.0.0.1:18096"
	stale := exec.Command(exe)
	stale.Env = append(os.Environ(), "FAKE_ADDR="+addr)
	if err := stale.Start(); err != nil {
		t.Fatalf("start stale: %v", err)
	}
	t.Cleanup(func() { _ = stale.Process.Kill(); _, _ = stale.Process.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	cl := &http.Client{Timeout: 300 * time.Millisecond}
	for time.Now().Before(deadline) {
		if r, err := cl.Get("http://" + addr + "/healthz"); err == nil {
			r.Body.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	sup := &supervisor{
		command:    exe,
		healthzURL: "http://" + addr + "/healthz",
		env:        []string{"FAKE_ADDR=127.0.0.1:1"},
		timeout:    2 * time.Second,
		compatible: func(ctx context.Context) bool { return false }, // stale secret
	}
	err := sup.start(context.Background())
	if err == nil {
		t.Fatal("expected error for a listening but incompatible adapter")
	}
	if !strings.Contains(err.Error(), "HMAC") {
		t.Fatalf("error should mention HMAC/auth, got: %v", err)
	}
}

func TestResolveAdapterCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(resolveExeName("weixin-adapter"), ".exe") {
			t.Fatal("windows should append .exe")
		}
		// idempotent: already suffixed names are not doubled.
		if resolveExeName("weixin-adapter.exe") != "weixin-adapter.exe" {
			t.Fatalf("already-suffixed name changed: %q", resolveExeName("weixin-adapter.exe"))
		}
	} else {
		if resolveExeName("weixin-adapter") != "weixin-adapter" {
			t.Fatal("non-windows should not append suffix")
		}
	}
}
