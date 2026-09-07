package webhook

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// supervisor manages an adapter child process for autostart instances. It is
// cross-platform: it execs the command directly (no shell), polls /healthz
// until ready, and kills the process on stop (TerminateProcess on Windows,
// SIGKILL-equivalent elsewhere). Because the command is exec'd directly
// (never via `go run` or a shell), Process.Kill terminates the adapter
// itself with no orphaned grandchildren.
type supervisor struct {
	command    string
	args       []string
	env        []string
	healthzURL string
	timeout    time.Duration

	// compatible, when set, reports whether a listening adapter shares baize's
	// secret (a signed management call succeeds). A bare /healthz cannot tell a
	// compatible orphan from a stale/foreign process holding the port, so the
	// adopt decision uses this when available.
	compatible func(ctx context.Context) bool

	// adopted is true when start() attached to an already-listening adapter
	// (an orphan) rather than spawning its own process; s.cmd stays nil for an
	// adopted process, so process-level termination must go through the
	// adapter's HMAC /admin/shutdown instead of killing a PID baize does not
	// own.
	adopted bool

	cmd *exec.Cmd
}

func (s *supervisor) start(ctx context.Context) error {
	if s.command == "" {
		return fmt.Errorf("webhook supervisor: empty adapter command")
	}
	// Reuse before spawn: an adapter may already be listening (an orphan from a
	// previous baize run hard-killed before it could stop its child). Adopting
	// it avoids spawning a second process that fails to bind the port. Only
	// adopt when it is healthy AND (when checkable) shares our secret; a
	// listener that fails auth is stale/foreign and must be reclaimed manually.
	if s.adapterListening(ctx) {
		if s.compatible == nil || s.compatible(ctx) {
			s.adopted = true
			return nil
		}
		return fmt.Errorf("webhook supervisor: %s already serves an adapter that fails HMAC auth; stop the stale weixin-adapter process (it holds an old secret) and restart", s.healthzURL)
	}
	s.adopted = false
	resolved := resolveAdapterPath(s.command)
	// Use exec.Command (NOT CommandContext): the child must outlive the
	// request/start context that triggers a management-plane (re)start. The
	// process lifecycle is owned explicitly here — killed via Stop()/kill()
	// on graceful shutdown or process stop/restart; a hard baize crash leaves
	// an orphan that a later start() detects and adopts/rejects by healthz +
	// signed check. ctx is still used below only to bound the healthz wait.
	cmd := exec.Command(resolved, s.args...)
	cmd.Env = append(os.Environ(), s.env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("webhook supervisor: start %q: %w", s.command, err)
	}
	s.cmd = cmd
	timeout := s.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if err := waitHealthz(ctx, s.healthzURL, timeout); err != nil {
		_ = s.kill()
		return fmt.Errorf("webhook supervisor: adapter never became healthy: %w", err)
	}
	return nil
}

// ownsProcess reports whether baize spawned (and holds a handle to) the current
// adapter process. An adopted orphan has no handle here.
func (s *supervisor) ownsProcess() bool { return s.cmd != nil }

// waitUntilDown polls healthz until the adapter stops responding (process
// exited) or the deadline passes.
func (s *supervisor) waitUntilDown(ctx context.Context, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 400 * time.Millisecond}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.healthzURL, nil)
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			return // not listening anymore
		}
		resp.Body.Close()
		select {
		case <-ctx.Done():
			return
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// adapterListening reports whether something answers the health endpoint (no
// auth). Distinguishes "port free" from "a process is already there".
func (s *supervisor) adapterListening(ctx context.Context) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, s.healthzURL, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *supervisor) stop(ctx context.Context) error {
	_ = ctx
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	return s.kill()
}

// kill terminates the child and reaps it. Process.Kill is cross-platform:
// TerminateProcess on Windows, SIGKILL on POSIX.
func (s *supervisor) kill() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	s.cmd = nil
	return nil
}

func (s *supervisor) running() bool {
	return s.cmd != nil && s.cmd.ProcessState == nil
}

// resetForRespawn clears process state after a stop/shutdown so a subsequent
// start() spawns a fresh child instead of considering itself adopted.
func (s *supervisor) resetForRespawn() {
	s.cmd = nil
	s.adopted = false
}

// waitHealthz polls the adapter's health endpoint until it returns 200, the
// deadline passes, or ctx is cancelled. Each probe uses a short timeout so a
// dead/unlistening child does not stall the loop.
func waitHealthz(ctx context.Context, healthzURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, err := client.Get(healthzURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return fmt.Errorf("healthz timeout at %s", healthzURL)
}

// resolveExeName appends .exe on Windows when the name does not already carry
// the suffix.
func resolveExeName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}

// resolveAdapterPath locates the adapter executable: PATH lookup of the bare
// name first (Windows LookPath honours PATHEXT), then the suffixed name, then
// a ./bin/<name> fallback (build artifacts land in bin/). If nothing is found,
// the resolved (suffixed) name is returned so exec reports the failure with
// the expected filename.
func resolveAdapterPath(command string) string {
	named := resolveExeName(command)
	if p, err := exec.LookPath(command); err == nil {
		return p
	}
	if p, err := exec.LookPath(named); err == nil {
		return p
	}
	bin := "bin" + string(os.PathSeparator) + named
	if p, err := exec.LookPath(bin); err == nil {
		return p
	}
	return named
}
