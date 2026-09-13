package webhook

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// supervisor manages an adapter child process for autostart instances. It is
// cross-platform: it execs the command directly (no shell), polls /healthz
// until ready, and kills the process on stop (TerminateProcess on Windows,
// SIGKILL-equivalent elsewhere). Because the command is exec'd directly
// (never via `go run` or a shell), Process.Kill terminates the adapter
// itself with no orphaned grandchildren. A watchdog goroutine reaps the
// child and, on an unexpected exit, respawns it with exponential backoff;
// an intentional stop (wantRunning=false) is never respawned.
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

	// gracefulShutdown, when set, asks a baize-owned child to exit gracefully
	// (HMAC /admin/shutdown) before the SIGTERM/force-kill fallbacks. Wired by
	// channel Bootstrap; nil makes terminate() skip straight to escalation.
	gracefulShutdown func(ctx context.Context) error

	// lifecycleCtx bounds the watch goroutine and backoff loop; cancelled on
	// baize shutdown so a parked watchdog exits without respawning.
	lifecycleCtx context.Context

	mu sync.Mutex
	// cmd is the currently supervised process (nil for an adopted orphan).
	cmd *exec.Cmd
	// adopted is true when start() attached to an already-listening adapter
	// (an orphan) rather than spawning its own process; s.cmd stays nil for an
	// adopted process, so process-level termination must go through the
	// adapter's HMAC /admin/shutdown instead of killing a PID baize does not
	// own.
	adopted     bool
	wantRunning bool
	stopCh      chan struct{}
	restarts    int
	failStreak  int
	backingOff  bool
	lastExit    string
	backoff     func(failStreak int) time.Duration
	// grace is the window allowed for a graceful child shutdown (HMAC
	// /admin/shutdown, then POSIX SIGTERM) before the force-kill fallback.
	// Zero defaults to 5s in terminate().
	grace time.Duration
	// wg tracks every live watchdog goroutine (watch + its respawnLoop).
	// stop()/terminate() wait on it (bounded via waitWatchQuiet) so that an
	// in-flight respawn — a respawnLoop parked in spawn()/waitHealthz — is
	// cancelled by stopCh and fully unwound before the supervisor reports
	// quiescent; otherwise that transient spawn could birth an orphan. wg is
	// zero for an adopted orphan (no watch goroutine at all).
	wg sync.WaitGroup
}

// spawn resolves and starts a fresh adapter process, then waits for its
// healthz endpoint. On a healthz failure the started process is killed and
// reaped so no orphan holds the port.
func (s *supervisor) spawn(ctx context.Context) (*exec.Cmd, error) {
	resolved := resolveAdapterPath(s.command)
	// Use exec.Command (NOT CommandContext): the child must outlive the
	// request/start context that triggers a management-plane (re)start. The
	// process lifecycle is owned explicitly here — killed via stop() on an
	// intentional stop; a hard baize crash leaves an orphan that a later
	// start() detects and adopts/rejects by healthz + signed check. ctx is
	// only used to bound the healthz wait (and the watchdog respawn loop).
	cmd := exec.Command(resolved, s.args...)
	cmd.Env = append(os.Environ(), s.env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %q: %w", s.command, err)
	}
	timeout := s.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if err := waitHealthz(ctx, s.healthzURL, timeout); err != nil {
		// No watch goroutine exists for this cmd yet, so kill+Wait here is
		// safe (see killCmd's contract).
		_ = killCmd(cmd)
		return nil, fmt.Errorf("adapter never became healthy: %w", err)
	}
	return cmd, nil
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
			s.mu.Lock()
			s.adopted = true
			s.wantRunning = true
			s.mu.Unlock()
			return nil
		}
		return fmt.Errorf("webhook supervisor: %s already serves an adapter that fails HMAC auth; stop the stale weixin-adapter process (it holds an old secret) and restart", s.healthzURL)
	}
	// Retire the previous watchdog generation before spawning a new one.
	// StartProcess can land while the old watchdog is parked in a crash
	// backoff (respawnLoop alive, s.cmd nil, no listener). Overwriting stopCh
	// here would leak that goroutine: its stopCh never closes, and setting
	// wantRunning=true suppresses its !wantRunning cleanup branch — both
	// generations would then race to spawn on the same port, and the old one
	// becomes unreachable to any future terminate() (a permanent leak). Tear
	// it down exactly like an intentional stop (close stopCh, kill a live
	// child, join watch+respawnLoop via waitWatchQuiet). An adopted orphan
	// never created a stopCh, so this is a no-op for it (and an adopted
	// orphan is healthy, so it takes the adopt branch above anyway).
	if s.stopCh != nil {
		if err := s.terminate(ctx); err != nil {
			return fmt.Errorf("webhook supervisor: retire previous watchdog: %w", err)
		}
	}
	s.mu.Lock()
	s.adopted = false
	s.stopCh = make(chan struct{})
	s.wantRunning = true
	s.backingOff = false
	s.mu.Unlock()
	cmd, err := s.spawn(ctx)
	if err != nil {
		s.mu.Lock()
		s.wantRunning = false
		s.mu.Unlock()
		return fmt.Errorf("webhook supervisor: %w", err)
	}
	s.mu.Lock()
	s.cmd = cmd
	s.failStreak = 0
	s.mu.Unlock()
	// wg.Add MUST precede `go`; the deferred Done covers watch() and the
	// respawnLoop it runs synchronously (a successful respawn starts a
	// separately-counted watch goroutine).
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.watch(cmd)
	}()
	return nil
}

// watch reaps the supervised process. On an unexpected exit it drives a
// backoff respawn loop; on an intentional stop (wantRunning=false) it just
// clears the cmd handle so callers waiting for reap observe shutdown.
func (s *supervisor) watch(cmd *exec.Cmd) {
	err := cmd.Wait()
	s.mu.Lock()
	if s.cmd != cmd { // a newer process superseded this one (should not happen)
		s.mu.Unlock()
		return
	}
	if err != nil {
		s.lastExit = err.Error()
	} else {
		s.lastExit = "exited 0"
	}
	// The reaped process is no longer owned regardless of what happens next:
	// drop the handle so ownsProcess()/running()/stop()'s waitReaped reflect
	// reality. While the watchdog is parked in backoff there is deliberately
	// no live cmd; a successful respawn installs the replacement in s.cmd.
	// (A newer process could only be here after a respawn, in which case
	// s.cmd != cmd was caught above.)
	s.cmd = nil
	if !s.wantRunning {
		s.backingOff = false
		s.mu.Unlock()
		return
	}
	// Unexpected crash.
	s.restarts++
	s.failStreak++
	s.backingOff = true
	stopCh := s.stopCh
	lifeCtx := s.lifecycleCtx
	s.mu.Unlock()
	log.Printf("webhook supervisor: adapter exited (%s); restarting (restarts=%d)", s.lastExit, s.restarts)
	if lifeCtx == nil {
		lifeCtx = context.Background()
	}
	s.respawnLoop(lifeCtx, stopCh)
	// respawnLoop has either installed a replacement process (s.cmd points at
	// it, reaped by a fresh watch goroutine) or aborted on stop/lifecycle-cancel
	// with no live child (s.cmd already nil); it clears backingOff in both
	// cases, so nothing further to do here.
}

// respawnLoop waits out the backoff (bounded by stopCh and the lifecycle
// context) and spawns a replacement child. Failed spawns are retried with a
// growing streak; a successful spawn hands the new process to a fresh watch
// goroutine.
func (s *supervisor) respawnLoop(ctx context.Context, stopCh <-chan struct{}) {
	bo := s.backoff
	if bo == nil {
		bo = defaultBackoff
	}
	for {
		s.mu.Lock()
		streak := s.failStreak
		s.mu.Unlock()
		select {
		case <-stopCh:
			s.setBackingOff(false)
			return
		case <-ctx.Done():
			s.setBackingOff(false)
			return
		case <-time.After(bo(streak)):
		}
		// Bind this spawn to the stop signal: waitHealthz inside spawn()
		// otherwise ignores stopCh/lifecycleCtx, so a stop that lands while a
		// respawn is mid-spawn would not cancel the in-flight healthz wait and
		// a transient child could be born after the supervisor reported down.
		// stopCh cancellation makes waitHealthz fail fast; spawn() then kills
		// and Waits the unhealthy child itself, and the loop's next select
		// observes stopCh and returns without spawning again.
		spawnCtx, cancelSpawn := context.WithCancel(ctx)
		go func() {
			select {
			case <-stopCh:
				cancelSpawn()
			case <-spawnCtx.Done():
			}
		}()
		cmd, err := s.spawn(spawnCtx)
		cancelSpawn()
		if err != nil {
			s.mu.Lock()
			s.failStreak++
			s.restarts++
			s.backingOff = true
			s.mu.Unlock()
			log.Printf("webhook supervisor: adapter respawn failed: %v", err)
			continue
		}
		s.mu.Lock()
		if !s.wantRunning {
			s.mu.Unlock()
			// No watch goroutine has been started for this fresh cmd yet, so
			// kill+Wait here is safe (see killCmd's contract). Reap it BEFORE
			// clearing backingOff so a concurrent stop()'s waitReaped waits
			// for this transient child to be gone (no post-stop respawn).
			_ = killCmd(cmd)
			s.setBackingOff(false)
			return
		}
		s.cmd = cmd
		s.failStreak = 0
		s.backingOff = false
		s.mu.Unlock()
		// Count the replacement watchdog goroutine before launching it (same
		// Add-before-go invariant as start()).
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.watch(cmd)
		}()
		return
	}
}

// defaultBackoff is exponential 1s,2s,4s,... capped at 30s.
func defaultBackoff(failStreak int) time.Duration {
	if failStreak < 1 {
		failStreak = 1
	}
	d := time.Duration(1<<uint(failStreak-1)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func (s *supervisor) setBackingOff(v bool) {
	s.mu.Lock()
	s.backingOff = v
	s.mu.Unlock()
}

// restarting reports whether the watchdog is parked in a backoff/respawn loop.
func (s *supervisor) restarting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backingOff
}

func (s *supervisor) restartCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restarts
}

// ownsProcess reports whether baize spawned (and holds a handle to) the current
// adapter process. An adopted orphan has no handle here.
func (s *supervisor) ownsProcess() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil
}

// isAdopted reports whether the supervisor attached to an already-listening
// orphan rather than spawning its own process.
func (s *supervisor) isAdopted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.adopted
}

// running reports whether a supervised child is currently live. It reads
// only s.cmd under the mutex: s.cmd is nil for an adopted orphan, after the
// watch goroutine has reaped the process, and after resetForRespawn. It must
// not read cmd.ProcessState — that field is written by Wait(), which only
// the watch goroutine calls, so reading it here would race.
func (s *supervisor) running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil
}

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

// stop intentionally halts the supervised child with force-kill semantics
// (no graceful /admin/shutdown). It must declare wantRunning=false (and close
// stopCh) BEFORE killing: otherwise the watchdog reaping the killed process
// would classify the exit as a crash and respawn. It only sends Process.Kill —
// process reaping (cmd.Wait) belongs exclusively to the watch goroutine
// (os/exec: Wait may be called at most once per Cmd) — then waits for watch
// to reap. If the watchdog is parked in backoff (no live cmd, ownsProcess
// already false) there is nothing to kill. Channel.Stop uses terminate()
// (graceful escalation) instead; stop() remains for callers that want the
// immediate force-kill path.
func (s *supervisor) stop(ctx context.Context) error {
	_ = ctx
	s.mu.Lock()
	s.wantRunning = false
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
	cmd := s.cmd
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		// Kill only; do NOT Wait here — watch owns the sole Wait on this cmd.
		_ = cmd.Process.Kill()
	}
	reapCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if !s.waitReaped(reapCtx) {
		return fmt.Errorf("webhook supervisor: adapter process not reaped within timeout")
	}
	// Also wait for the watchdog goroutine itself (incl. any in-flight respawn
	// cancelled by stopCh above) to fully exit, so no transient spawn can be
	// born after stop returns. Bounded; never blocks shutdown indefinitely.
	if !s.waitWatchQuiet(3 * time.Second) {
		return fmt.Errorf("webhook supervisor: watchdog did not stop within timeout")
	}
	return nil
}

// terminate stops the supervised child gracefully, escalating to a force kill.
// Order: (1) HMAC /admin/shutdown via the injected callback (cross-platform),
// (2) SIGTERM on POSIX, (3) Process.Kill. The watch goroutine owns Wait();
// terminate only signals and polls waitReaped until the handle is reaped. It
// announces the intent first (wantRunning=false + close stopCh) exactly like
// stop(), so a child that exits during escalation is never classified as a
// crash and respawned. An adopted orphan has no cmd handle here; its graceful
// shutdown is driven by the channel layer's signed call (wired in task 3).
func (s *supervisor) terminate(ctx context.Context) error {
	s.mu.Lock()
	s.wantRunning = false
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
	cmd := s.cmd
	gs := s.gracefulShutdown
	grace := s.grace
	s.mu.Unlock()
	if grace <= 0 {
		grace = 5 * time.Second
	}
	// No owned handle merges two cases: (a) an adopted orphan — no watch
	// goroutine, wg already zero, returns immediately; (b) the watchdog is
	// parked in backoff or mid-respawn inside s.spawn()/waitHealthz. Closing
	// stopCh cancels that in-flight spawn (see respawnLoop), so wait for the
	// watchdog goroutine to unwind — otherwise a transient child could be born
	// after terminate returns (an orphan on process exit, or a brief port
	// holder on restart). Bounded so a wedged watchdog never blocks shutdown.
	if cmd == nil || cmd.Process == nil {
		quiet := grace
		if quiet <= 0 {
			quiet = 3 * time.Second
		}
		// An adopted orphan has no watch goroutine (wg already zero), so this
		// returns true at once; a parked backoff/in-flight-spawn watchdog must
		// unwind within the bound. Surface a timeout instead of silently
		// returning: StopProcess/RestartProcess propagate it, Channel.Stop
		// ignores errors best-effort.
		if !s.waitWatchQuiet(quiet) {
			return fmt.Errorf("webhook supervisor: watchdog did not stop within %s (adapter process/backoff loop not quiescent)", quiet)
		}
		return nil
	}
	gctx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	if gs != nil {
		_ = gs(gctx)
		if s.waitReaped(gctx) {
			if !s.waitWatchQuiet(3 * time.Second) {
				return fmt.Errorf("webhook supervisor: watchdog did not stop after graceful shutdown")
			}
			return nil
		}
	}
	if runtime.GOOS != "windows" {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		if s.waitReaped(gctx) {
			if !s.waitWatchQuiet(3 * time.Second) {
				return fmt.Errorf("webhook supervisor: watchdog did not stop after SIGTERM")
			}
			return nil
		}
	}
	// Graceful/SIGTERM window elapsed with the child still alive: force kill.
	// Kill only; do NOT Wait here — watch owns the sole Wait on this cmd.
	_ = cmd.Process.Kill()
	reapCtx, reapCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer reapCancel()
	if !s.waitReaped(reapCtx) {
		return fmt.Errorf("webhook supervisor: adapter process not reaped after force kill")
	}
	// Wait for the watchdog goroutine itself to exit so no respawn follows.
	if !s.waitWatchQuiet(3 * time.Second) {
		return fmt.Errorf("webhook supervisor: watchdog did not stop after force kill")
	}
	return nil
}

// waitReaped polls until the supervised process is fully gone AND the watchdog
// is quiescent — no live cmd (ownsProcess false; watch has reaped it or the
// watchdog was parked in backoff) and no backoff/respawn still in flight
// (restarting false) — or ctx is cancelled/times out. Returns true once
// quiescent, false on timeout. stop() uses it so that once it returns there is
// no child left and no possibility of a further respawn.
func (s *supervisor) waitReaped(ctx context.Context) bool {
	quiescent := func() bool { return !s.ownsProcess() && !s.restarting() }
	if quiescent() {
		return true
	}
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if quiescent() {
				return true
			}
		}
	}
}

// waitWatchQuiet blocks until every watchdog goroutine (watch + respawnLoop)
// has exited, or until timeout. It is bounded so a wedged watchdog can never
// deadlock shutdown: it returns false on timeout. waitReaped waits for the cmd
// handle to clear and backingOff to drop; this waits for the goroutine itself
// to return (wg zero), which also covers a respawnLoop parked inside spawn().
// It is zero-cost for an adopted orphan (no watch goroutine, wg already zero).
func (s *supervisor) waitWatchQuiet(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// killCmd force-terminates AND reaps a process that has no watch goroutine
// attached: the healthz-failure cleanup inside spawn(), and the freshly-spawned
// cmd in respawnLoop when wantRunning turned false before watch was started.
// It must never be called for the live s.cmd process — that one is waited on
// exclusively by watch (a second Wait races on ProcessState and, on POSIX,
// the underlying waitpid is non-idempotent). Process.Kill is cross-platform:
// TerminateProcess on Windows, SIGKILL on POSIX.
func killCmd(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return nil
}

// resetForRespawn clears process state after a stop/shutdown so a subsequent
// start() spawns a fresh child instead of considering itself adopted.
func (s *supervisor) resetForRespawn() {
	s.mu.Lock()
	s.cmd = nil
	s.adopted = false
	s.backingOff = false
	s.wantRunning = false
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
	s.mu.Unlock()
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
