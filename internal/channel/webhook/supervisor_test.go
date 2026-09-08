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

// fakeCrashSource serves healthz briefly then exits non-zero, simulating an
// adapter that crashes after becoming healthy (drives the watchdog respawn).
// The ~700ms healthy lifetime keeps the first spawn's waitHealthz (150ms poll
// cadence + process startup) reliably green before the crash.
const fakeCrashSource = `package main
import ("net/http";"os";"time")
func main() {
	addr := os.Getenv("FAKE_ADDR")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request){ w.WriteHeader(200) })
	go http.ListenAndServe(addr, nil)
	time.Sleep(700 * time.Millisecond)
	os.Exit(1)
}
`

func buildFakeCrashAdapter(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(fakeCrashSource), 0o600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "fakecrash"+exeSuffix())
	build := exec.Command("go", "build", "-o", exe, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build fake crash adapter: %v\n%s", err, out)
	}
	return exe
}

func TestWatchdogRestartsCrashedChild(t *testing.T) {
	exe := buildFakeCrashAdapter(t)
	addr := "127.0.0.1:18089"
	lifeCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sup := &supervisor{
		command:      exe,
		healthzURL:   "http://" + addr + "/healthz",
		env:          []string{"FAKE_ADDR=" + addr},
		timeout:      5 * time.Second,
		lifecycleCtx: lifeCtx,
		backoff:      func(int) time.Duration { return 20 * time.Millisecond },
	}
	if err := sup.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	// The child crashes every ~700ms; the watchdog must respawn it repeatedly.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sup.restartCount() >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := sup.restartCount(); got < 2 {
		t.Fatalf("watchdog did not respawn crashed child; restarts=%d", got)
	}
	if err := sup.stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// Port must stay down after intentional stop (no further respawn).
	time.Sleep(400 * time.Millisecond)
	if healthzReachable2(sup.healthzURL) {
		t.Fatal("healthz reachable after stop: watchdog respawned despite intentional stop")
	}
}

func TestWatchdogNoRestartOnIntentionalStop(t *testing.T) {
	exe := buildFakeAdapter(t) // long-lived (select{})
	addr := "127.0.0.1:18088"
	sup := &supervisor{
		command:    exe,
		healthzURL: "http://" + addr + "/healthz",
		env:        []string{"FAKE_ADDR=" + addr},
		timeout:    5 * time.Second,
		backoff:    func(int) time.Duration { return 20 * time.Millisecond },
	}
	if err := sup.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := sup.stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if got := sup.restartCount(); got != 0 {
		t.Fatalf("intentional stop must not trigger restart, got restarts=%d", got)
	}
	time.Sleep(400 * time.Millisecond)
	if healthzReachable2(sup.healthzURL) {
		t.Fatal("healthz reachable after stop: child not gone / respawned")
	}
}

func TestWatchdogStopsOnLifecycleCancel(t *testing.T) {
	exe := buildFakeCrashAdapter(t)
	addr := "127.0.0.1:18087"
	lifeCtx, cancel := context.WithCancel(context.Background())
	sup := &supervisor{
		command:      exe,
		healthzURL:   "http://" + addr + "/healthz",
		env:          []string{"FAKE_ADDR=" + addr},
		timeout:      5 * time.Second,
		lifecycleCtx: lifeCtx,
		backoff:      func(int) time.Duration { return 30 * time.Second }, // long: park in backoff
	}
	if err := sup.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Wait until the first crash parks the watchdog in backoff.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sup.restarting() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sup.restarting() {
		t.Fatal("expected watchdog to enter restarting state after crash")
	}
	cancel() // baize shutdown
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !sup.restarting() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("watchdog still restarting after lifecycle context cancelled")
}

// TestWatchdogStopDuringBackoffReapsAndDoesNotRespawn: stopping while the
// watchdog is parked in a long backoff (crashed child, no live cmd) must not
// deadlock and must not respawn afterwards. This exercises stop()'s
// waitReaped path when watch is not blocked on cmd.Wait().
func TestWatchdogStopDuringBackoffReapsAndDoesNotRespawn(t *testing.T) {
	exe := buildFakeCrashAdapter(t)
	addr := "127.0.0.1:18086"
	lifeCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sup := &supervisor{
		command:      exe,
		healthzURL:   "http://" + addr + "/healthz",
		env:          []string{"FAKE_ADDR=" + addr},
		timeout:      5 * time.Second,
		lifecycleCtx: lifeCtx,
		backoff:      func(int) time.Duration { return 30 * time.Second }, // long: park in backoff
	}
	if err := sup.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Wait until the first crash parks the watchdog in backoff.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sup.restarting() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sup.restarting() {
		t.Fatal("expected watchdog to enter restarting state after crash")
	}
	if sup.ownsProcess() {
		t.Fatal("expected no live child while parked in backoff")
	}
	// stop() must return promptly (watch owns the Wait; nothing to kill).
	done := make(chan error, 1)
	go func() { done <- sup.stop(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop during backoff: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop() deadlocked while watchdog was parked in backoff")
	}
	if sup.ownsProcess() {
		t.Fatal("ownsProcess after stop: process handle not cleared")
	}
	if sup.running() {
		t.Fatal("running() after stop: supervisor should be down")
	}
	// The backoff must have been cancelled: no respawn even after waiting.
	time.Sleep(400 * time.Millisecond)
	if sup.restartCount() < 1 {
		t.Fatalf("expected at least the initial crash to be counted, got %d", sup.restartCount())
	}
	if healthzReachable2(sup.healthzURL) {
		t.Fatal("healthz reachable after stop: watchdog respawned despite intentional stop")
	}
}

// healthzReachable2 is a local probe to avoid importing process_test helpers.
func healthzReachable2(url string) bool {
	cl := &http.Client{Timeout: 200 * time.Millisecond}
	r, err := cl.Get(url)
	if err != nil {
		return false
	}
	r.Body.Close()
	return r.StatusCode == http.StatusOK
}

func buildFakeExe(t *testing.T, source, name string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, name+exeSuffix())
	if out, err := exec.Command("go", "build", "-o", exe, src).CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", name, err, out)
	}
	return exe
}

// fakeShutdownSource exits 0 on POST /admin/shutdown after writing a marker
// (cross-platform graceful path driven by the gracefulShutdown callback).
const fakeShutdownSource = `package main
import ("net/http";"os")
func main() {
	addr := os.Getenv("FAKE_ADDR"); marker := os.Getenv("MARKER_PATH")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request){ w.WriteHeader(200) })
	http.HandleFunc("/admin/shutdown", func(w http.ResponseWriter, r *http.Request){
		os.WriteFile(marker, []byte("shutdown"), 0o644)
		w.WriteHeader(200)
		go os.Exit(0)
	})
	go http.ListenAndServe(addr, nil)
	select{}
}
`

// fakeSignalSource exits 0 on SIGTERM/interrupt after writing a marker
// (POSIX fallback when no gracefulShutdown callback is wired).
const fakeSignalSource = `package main
import ("net/http";"os";"os/signal";"syscall")
func main() {
	addr := os.Getenv("FAKE_ADDR"); marker := os.Getenv("MARKER_PATH")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request){ w.WriteHeader(200) })
	go http.ListenAndServe(addr, nil)
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	<-c
	os.WriteFile(marker, []byte("sigterm"), 0o644)
	os.Exit(0)
}
`

// fakeIgnoreSource swallows SIGTERM (relayed to an undrained channel) and has
// no shutdown endpoint, so terminate must escalate to a force kill.
const fakeIgnoreSource = `package main
import ("net/http";"os";"os/signal";"syscall")
func main() {
	addr := os.Getenv("FAKE_ADDR")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request){ w.WriteHeader(200) })
	go http.ListenAndServe(addr, nil)
	signal.Notify(make(chan os.Signal, 1), syscall.SIGTERM, os.Interrupt)
	select{}
}
`

func waitPortDown(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	cl := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		r, err := cl.Get(url)
		if err != nil {
			return
		}
		r.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("healthz still reachable: child not terminated: %s", url)
}

func TestTerminateGracefulViaShutdownEndpoint(t *testing.T) {
	exe := buildFakeExe(t, fakeShutdownSource, "fakeshutdown")
	addr := "127.0.0.1:18086"
	marker := filepath.Join(t.TempDir(), "graceful.marker")
	sup := &supervisor{
		command:    exe,
		healthzURL: "http://" + addr + "/healthz",
		env:        []string{"FAKE_ADDR=" + addr, "MARKER_PATH=" + marker},
		timeout:    5 * time.Second,
		grace:      3 * time.Second,
		gracefulShutdown: func(ctx context.Context) error {
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/admin/shutdown", nil)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
			}
			return err
		},
	}
	if err := sup.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := sup.terminate(context.Background()); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("graceful marker missing: %v", err)
	}
	if string(got) != "shutdown" {
		t.Fatalf("marker = %q, want shutdown (graceful path not taken)", string(got))
	}
	waitPortDown(t, sup.healthzURL)
}

func TestTerminateSigtermFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM fallback is POSIX-only; Windows uses /admin/shutdown")
	}
	exe := buildFakeExe(t, fakeSignalSource, "fakesignal")
	addr := "127.0.0.1:18085"
	marker := filepath.Join(t.TempDir(), "sigterm.marker")
	sup := &supervisor{
		command:    exe,
		healthzURL: "http://" + addr + "/healthz",
		env:        []string{"FAKE_ADDR=" + addr, "MARKER_PATH=" + marker},
		timeout:    5 * time.Second,
		grace:      3 * time.Second,
		// gracefulShutdown intentionally nil: exercises SIGTERM fallback.
	}
	if err := sup.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := sup.terminate(context.Background()); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "sigterm" {
		t.Fatalf("SIGTERM marker missing/wrong: %v %q", err, string(got))
	}
	waitPortDown(t, sup.healthzURL)
}

func TestTerminateForceKillsIgnoringChild(t *testing.T) {
	exe := buildFakeExe(t, fakeIgnoreSource, "fakeignore")
	addr := "127.0.0.1:18084"
	sup := &supervisor{
		command:    exe,
		healthzURL: "http://" + addr + "/healthz",
		env:        []string{"FAKE_ADDR=" + addr},
		timeout:    5 * time.Second,
		grace:      300 * time.Millisecond, // short: escalate to force kill quickly
	}
	if err := sup.start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := sup.terminate(context.Background()); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	if sup.running() {
		t.Fatal("supervisor still running after force kill")
	}
	waitPortDown(t, sup.healthzURL)
}
