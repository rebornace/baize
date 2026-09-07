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

	cmd *exec.Cmd
}

func (s *supervisor) start(ctx context.Context) error {
	if s.command == "" {
		return fmt.Errorf("webhook supervisor: empty adapter command")
	}
	resolved := resolveAdapterPath(s.command)
	cmd := exec.CommandContext(ctx, resolved, s.args...)
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
