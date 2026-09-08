# 适配器 supervisor 硬化 + 生产部署工件 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 给 webhook 渠道的 autostart 适配器 supervisor 增加"崩溃看门狗（退避自动重启）"与"优雅关停（HMAC/SIGTERM 先礼后兵）"，并补齐生产部署工件（Dockerfile 构建适配器、独立 compose、systemd 单元、部署文档）。

**架构：** supervisor（`internal/channel/webhook/supervisor.go`）新增 `wantRunning/stopCh/backingOff` 状态机：spawn 后起 `watch` goroutine 调 `cmd.Wait()`，意外崩溃则指数退避（1s→30s 封顶）无限重启，有意停止则退出。关停统一走 `terminate()`：优先 HMAC `/admin/shutdown`（跨平台），POSIX 再兜底 SIGTERM，最后 `Kill()` 强杀；只有 `watch` 调 `cmd.Wait()` 回收。`Channel.Status()` 在退避期间报 `restarting`。部署侧：同一镜像构建 baize + weixin-adapter，提供独立 compose 与 systemd 范本，文档说明 autostart vs 独立部署取舍。

**技术栈：** Go 标准库（`os/exec`、`os/signal`、`syscall`、`net/http`、`sync`、`time`）；Docker / docker-compose；systemd unit；Markdown。

**规格：** `docs/superpowers/specs/2026-09-08-adapter-supervisor-hardening-design.md`

**测试命令（包目录）：** `go test ./internal/channel/webhook/...`

---

## 文件结构

- 修改：`internal/channel/webhook/supervisor.go`
  - 新增字段（mu/wantRunning/stopCh/restarts/failStreak/backingOff/backoff/grace/gracefulShutdown/lifecycleCtx）；抽取 `spawn()`；新增 `watch()`、`terminate()`、`restarting()`、`defaultBackoff()`；`start()`/`stop()` 改造。
- 修改：`internal/channel/webhook/channel.go`
  - Channel 新增 `lifeCtx context.Context`/`lifeCancel context.CancelFunc`；Bootstrap 创建生命周期 ctx、注入 `sup.lifecycleCtx` 与 `sup.gracefulShutdown`；`Stop()` cancel 生命周期并走 terminate。
- 修改：`internal/channel/webhook/process.go`
  - owned 进程的 stop/restart 从 `sup.stop`（强杀）改为 `sup.terminate`。
- 修改：`internal/channel/webhook/settings.go`
  - `Status()` 在 admin 探活前增加 `restarting` reason。
- 测试：`internal/channel/webhook/supervisor_test.go`、`process_test.go`、`settings_test.go`
- 修改：`Dockerfile`（构建+拷贝 weixin-adapter）
- 新增：`docker-compose.weixin.yml`、`deploy/systemd/baize.service`、`deploy/systemd/weixin-adapter.service`
- 新增：`docs/deployment.md`（公开仓导出）；修改 `README.md` 与 `README.zh-CN.md` 加入口

---

## 任务 1：supervisor 崩溃看门狗（退避自动重启）

**文件：**
- 修改：`internal/channel/webhook/supervisor.go`
- 测试：`internal/channel/webhook/supervisor_test.go`

- [ ] **步骤 1：编写失败的测试**

在 `supervisor_test.go` 末尾追加。先加一个"启动后很快崩溃"的假适配器源码与构建助手，再加三个用例：

```go
// fakeCrashSource serves healthz briefly then exits non-zero, simulating an
// adapter that crashes after becoming healthy (drives the watchdog respawn).
const fakeCrashSource = `package main
import ("net/http";"os";"time")
func main() {
	addr := os.Getenv("FAKE_ADDR")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request){ w.WriteHeader(200) })
	go http.ListenAndServe(addr, nil)
	time.Sleep(200 * time.Millisecond)
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
	// The child crashes every ~200ms; the watchdog must respawn it repeatedly.
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
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/ -run TestWatchdog -v`
预期：编译失败，`sup.restartCount` / `sup.restarting` / `sup.lifecycleCtx` / `sup.backoff` 未定义。

- [ ] **步骤 3：实现最少代码**

在 `supervisor.go` 中：

(a) 新增 import：`"log"`、`"sync"`。

(b) 结构体改为（新增字段，保留现有字段）：

```go
type supervisor struct {
	command    string
	args       []string
	env        []string
	healthzURL string
	timeout    time.Duration

	compatible       func(ctx context.Context) bool
	gracefulShutdown func(ctx context.Context) error

	// lifecycleCtx bounds the watch goroutine and backoff loop; cancelled on
	// baize shutdown so a parked watchdog exits without respawning.
	lifecycleCtx context.Context

	mu          sync.Mutex
	cmd         *exec.Cmd
	adopted     bool
	wantRunning bool
	stopCh      chan struct{}
	restarts    int
	failStreak  int
	backingOff  bool
	lastExit    string
	backoff     func(failStreak int) time.Duration
	grace       time.Duration
}
```

(c) 把 `start()` 中"解析路径 + exec + 等 healthz"抽成 `spawn`，并改造 `start`：

```go
func (s *supervisor) spawn(ctx context.Context) (*exec.Cmd, error) {
	resolved := resolveAdapterPath(s.command)
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
		_ = cmd.Process.Kill()
		_, _ = cmd.Wait()
		return nil, fmt.Errorf("adapter never became healthy: %w", err)
	}
	return cmd, nil
}

func (s *supervisor) start(ctx context.Context) error {
	if s.command == "" {
		return fmt.Errorf("webhook supervisor: empty adapter command")
	}
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
	go s.watch(cmd)
	return nil
}
```

(d) 新增 `watch` / `respawnLoop` / `defaultBackoff` / 访问器：

```go
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
	if !s.wantRunning {
		s.cmd = nil
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
}

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
		cmd, err := s.spawn(ctx)
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
			s.backingOff = false
			s.mu.Unlock()
			_ = cmd.Process.Kill()
			_, _ = cmd.Wait()
			return
		}
		s.cmd = cmd
		s.failStreak = 0
		s.backingOff = false
		s.mu.Unlock()
		go s.watch(cmd)
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
```

(e) 给 `running()`/`ownsProcess()` 加锁，新增 `isAdopted()`，并更新 `resetForRespawn`：

```go
func (s *supervisor) running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil && s.cmd.ProcessState == nil
}

func (s *supervisor) ownsProcess() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil
}

func (s *supervisor) isAdopted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.adopted
}

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
```

(f) **改造 `stop()`/`kill()`**：有意停止必须先宣告 `wantRunning=false` 并关闭 `stopCh`，
否则 watcher 在 `Wait()` 返回后会以为是崩溃而重启。把"杀+回收"抽成 `killCmd`：

```go
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
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return killCmd(cmd)
}

// killCmd force-terminates and reaps a process. Process.Kill is cross-platform:
// TerminateProcess on Windows, SIGKILL on POSIX.
func killCmd(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return nil
}

func (s *supervisor) kill() error {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	return killCmd(cmd)
}
```

> 说明：`watch` 与 `killCmd` 都可能 `cmd.Wait()` 同一个进程；二次 `Wait()` 只返回
> 相同结果，安全。强杀后 watcher 的 `Wait()` 返回，看到 `wantRunning==false` 即退出、
> 不重启。优雅关停（terminate）在任务 2 引入，本任务的 `stop` 仍是强杀语义。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/channel/webhook/ -run TestWatchdog -v`
预期：PASS（三个用例）。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/webhook/supervisor.go internal/channel/webhook/supervisor_test.go
git commit -m "feat(webhook): supervisor 崩溃看门狗——退避自动重启适配器子进程"
```

---

## 任务 2：优雅关停（HMAC `/admin/shutdown` → SIGTERM → 强杀）

**文件：**
- 修改：`internal/channel/webhook/supervisor.go`
- 测试：`internal/channel/webhook/supervisor_test.go`

> 关键约定：**只有 `watch` 调用 `cmd.Wait()` 回收进程**。对一个已被 watch 托管的
> cmd，关停路径只 `Kill()`/发信号，然后轮询 `ownsProcess()` 等 watch 回收置空；
> `killCmd`（kill+wait）仅用于"还没起 watch"的 cmd（spawn 失败清理、respawn 后不再需要）。

- [ ] **步骤 1：编写失败的测试**

在 `supervisor_test.go` 追加通用构建助手与三个用例：

```go
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
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/ -run TestTerminate -v`
预期：编译失败，`sup.terminate` 未定义。

- [ ] **步骤 3：实现最少代码**

在 `supervisor.go` import 块加 `"syscall"`（`runtime` 已在）。新增：

```go
// terminate stops the supervised child gracefully, escalating to a force kill.
// Order: (1) HMAC /admin/shutdown via the injected callback (cross-platform),
// (2) SIGTERM on POSIX, (3) Process.Kill. The watch goroutine owns Wait();
// terminate only signals and polls until the handle is reaped.
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
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if grace <= 0 {
		grace = 5 * time.Second
	}
	gctx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	if gs != nil {
		_ = gs(gctx)
		if s.waitReaped(gctx) {
			return nil
		}
	}
	if runtime.GOOS != "windows" {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		if s.waitReaped(gctx) {
			return nil
		}
	}
	_ = cmd.Process.Kill()
	s.waitReaped(shortCtx())
	return nil
}

// waitReaped polls until watch has reaped the child (cmd handle cleared) or ctx
// expires. watch owns cmd.Wait(); terminate must not call Wait concurrently.
func (s *supervisor) waitReaped(ctx context.Context) bool {
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !s.ownsProcess() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func shortCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}
```

并把任务 1 的 `stop()` 从 `killCmd(cmd)` 改为"发 kill 信号 + 等回收"（watch 负责 Wait）：

```go
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
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	wctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.waitReaped(wctx)
	return nil
}
```

> `killCmd`（kill+Wait）保留，仅供 `spawn()` 健康检查失败清理与 `respawnLoop`
> 中"spawn 成功但已不再需要"的、**尚未起 watch** 的 cmd 使用。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/channel/webhook/ -run "TestTerminate|TestWatchdog|TestSupervisor|TestChannel" -v`
预期：PASS（含既有收养/拒绝用例不回归）。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/webhook/supervisor.go internal/channel/webhook/supervisor_test.go
git commit -m "feat(webhook): supervisor 优雅关停——/admin/shutdown→SIGTERM→强杀"
```

---

## 任务 3：接入 Channel 生命周期、process 控制与 `restarting` 状态

**文件：**
- 修改：`internal/channel/webhook/channel.go`（Channel 字段、Bootstrap 注入、Stop）
- 修改：`internal/channel/webhook/process.go`（stop/restart 走 terminate，加锁访问器）
- 修改：`internal/channel/webhook/settings.go`（Status 增加 restarting）
- 测试：`internal/channel/webhook/process_test.go`

- [ ] **步骤 1：编写失败的测试**

在 `process_test.go` 末尾追加（白盒，直接置 `backingOff` 验证 Status 分支；admin 探活
不应被触发）：

```go
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
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/ -run TestStatusReportsRestarting -v`
预期：FAIL，`Status()` 返回的 Reason 不是 `"restarting"`（当前会走到 admin 探活）。

- [ ] **步骤 3：实现最少代码**

(a) `channel.go`：Channel 结构体（`procMu sync.Mutex` 附近）新增生命周期字段：

```go
	// lifeCtx/lifeCancel bound the supervisor watchdog to baize's lifetime;
	// cancelled in Stop() so a parked backoff loop exits without respawning.
	lifeCtx    context.Context
	lifeCancel context.CancelFunc
```

(b) `channel.go` Bootstrap 中，构造 supervisor 的 `sup.compatible = ...` 块之后、
`c.sup = sup` 之前，注入生命周期 ctx 与优雅关停回调：

```go
		c.lifeCtx, c.lifeCancel = context.WithCancel(context.Background())
		sup.lifecycleCtx = c.lifeCtx
		if c.admin != nil {
			sup.gracefulShutdown = func(ctx context.Context) error {
				return c.admin.Shutdown(ctx)
			}
		}
```

(c) `channel.go` 的 `Stop` 改为 cancel 生命周期并走 terminate：

```go
func (c *Channel) Stop(ctx context.Context) error {
	if c.admin != nil {
		_ = c.admin.Stop(ctx)
	}
	if c.sup != nil {
		if c.lifeCancel != nil {
			c.lifeCancel()
		}
		_ = c.sup.terminate(ctx)
	}
	return nil
}
```

(d) `process.go`：`StopProcess` 的 owned 分支把 `c.sup.stop(ctx)` 改为
`c.sup.terminate(ctx)`：

```go
	if c.sup.ownsProcess() {
		err := c.sup.terminate(ctx)
		c.sup.waitUntilDown(ctx, 8*time.Second)
		c.setManualStop(true)
		return err
	}
```

`RestartProcess` 的关停 switch 改为（owned 走 terminate，adopted 走 Shutdown，
并用加锁访问器 `isAdopted()`）：

```go
	switch {
	case c.sup.ownsProcess():
		_ = c.sup.terminate(ctx)
	case c.admin != nil && c.sup.isAdopted():
		_ = c.admin.Shutdown(ctx)
	}
```

(e) `settings.go` 的 `Status()`，在 `manualStop` 分支之后、`if c.admin == nil` 之前
插入：

```go
	if c.sup != nil && c.sup.restarting() {
		// Watchdog is parked in a backoff/respawn loop after a crash.
		return channel.AdapterStatus{Running: false, Reason: "restarting"}
	}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/channel/webhook/... -v`
预期：PASS（新用例 + 既有 start/stop/restart/adopted 用例全绿；SIGTERM 对无信号处理
的假适配器是默认终止，Windows 走 Kill，均被 watch 回收）。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/webhook/channel.go internal/channel/webhook/process.go internal/channel/webhook/settings.go internal/channel/webhook/process_test.go
git commit -m "feat(webhook): 接入看门狗生命周期与优雅关停，Status 报 restarting"
```

---

## 任务 4：生产部署工件（Dockerfile / compose / systemd / 部署文档）

**文件：**
- 修改：`Dockerfile`
- 新增：`docker-compose.weixin.yml`
- 新增：`deploy/systemd/baize.service`、`deploy/systemd/weixin-adapter.service`
- 新增：`docs/deployment.md`
- 修改：`README.md`、`README.zh-CN.md`（加入口链接）

此任务为基础设施/文档，无 Go 单测；验证靠构建与配置校验（见步骤 4）。

- [ ] **步骤 1：Dockerfile 构建适配器**

把 `Dockerfile` 的 build RUN 改为三行，并在运行阶段拷贝适配器：

```dockerfile
RUN go build -o /out/baize ./cmd/baize \
 && go build -o /out/mock-ticket ./examples/mock-ticket/cmd/mock-ticket \
 && go build -o /out/weixin-adapter ./cmd/weixin-adapter
```

运行阶段在 `COPY --from=build /out/mock-ticket /app/mock-ticket` 之后加：

```dockerfile
COPY --from=build /out/weixin-adapter /app/weixin-adapter
```

- [ ] **步骤 2：新增 `docker-compose.weixin.yml`**（适配器独立服务 = E 模式范本）

```yaml
# 独立部署（生产）范本：baize 与 weixin-adapter 作为两个服务，适配器由
# compose 负责重启（restart policy），baize 不再 autostart 子进程。
# baize 配置需声明 webhook 实例（见 docs/deployment.md）：
#   adapter_autostart: "false"
#   admin_url:  http://weixin-adapter:8090
#   outbound_url: http://weixin-adapter:8090/outbound
#   secret:     与 WEIXIN_ADAPTER_SECRET 相同
services:
  baize:
    build: .
    ports:
      - "8080:8080"
    environment:
      BAIZE_API_KEY: ${BAIZE_API_KEY:?set BAIZE_API_KEY for production compose}
    volumes:
      - baize-data:/app/data
  weixin-adapter:
    build: .
    command:
      - /app/weixin-adapter
      - -baize=http://baize:8080/v0/channels/weixin/inbound
      - -secret=${WEIXIN_ADAPTER_SECRET:?set WEIXIN_ADAPTER_SECRET, shared with baize channel config}
      - -addr=:8090
      - -creds=/data/channels/weixin
    volumes:
      - weixin-data:/data/channels/weixin
    depends_on:
      - baize
    restart: unless-stopped
volumes:
  baize-data:
  weixin-data:
```

- [ ] **步骤 3：新增 systemd 单元**

`deploy/systemd/baize.service`：

```ini
[Unit]
Description=Baize agent runtime
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/baize
ExecStart=/opt/baize/baize serve -config /opt/baize/config.yaml
Restart=always
RestartSec=2
# 桌面/autostart 模式下 baize 自己托管适配器子进程，无需 weixin-adapter.service。

[Install]
WantedBy=multi-user.target
```

`deploy/systemd/weixin-adapter.service`：

```ini
[Unit]
Description=Baize WeChat (iLink) adapter (independent deployment)
After=network-online.target baize.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/baize
ExecStart=/opt/baize/weixin-adapter \
  -baize=http://127.0.0.1:8080/v0/channels/weixin/inbound \
  -secret=REPLACE_WITH_SHARED_SECRET \
  -addr=127.0.0.1:8090 \
  -creds=/opt/baize/data/channels/weixin
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

> 仅"独立部署"需要 `weixin-adapter.service`；autostart 模式不要启用它（baize 会自己拉起并托管适配器）。

- [ ] **步骤 4：新增 `docs/deployment.md`** 并在两个 README 加入口

`docs/deployment.md` 写清两种部署模式（autostart vs 独立部署）、各自的看门狗/日志/资源责任归属、docker compose 与 systemd 起步命令、secret 与凭据目录挂载约定、配置片段（上面 compose 注释中的 webhook 实例 YAML）。要点：

- **Autostart（桌面/demo/单机，含 Windows）**：baize 托管适配器子进程；具备崩溃退避自动重启与优雅关停；零额外进程。适合双击 demo / 单机。
- **独立部署（生产/Linux/容器）**：适配器作为独立服务，systemd `Restart=always` 或容器 restart policy 负责看门，日志走 journald/容器日志，资源限制交给 systemd/cgroup；baize 配置 `adapter_autostart: "false"`，经 HTTP 连接适配器。
- 明确不做日志轮转/cgroup（YAGNI，归外部编排）。

在 `README.md` 与 `README.zh-CN.md` 的部署/文档小节各加一行链接：
- README.md：`- [Deployment guide](docs/deployment.md) — autostart vs. standalone adapter deployment.`
- README.zh-CN.md：`- [部署指南](docs/deployment.md) —— 适配器 autostart 托管 vs 独立部署。`

- [ ] **步骤 5：验证**

运行：
```bash
go build ./...
go build -o bin/weixin-adapter ./cmd/weixin-adapter
docker compose -f docker-compose.weixin.yml config   # 有 docker 时；无则人工核对 YAML
```
预期：`go build ./...` 与适配器构建成功；compose `config` 正常渲染两服务（无 docker 环境则跳过并人工核对缩进/字段）。

- [ ] **步骤 6：Commit**

```bash
git add Dockerfile docker-compose.weixin.yml deploy/systemd/ docs/deployment.md README.md README.zh-CN.md
git commit -m "docs(deploy): 适配器生产部署工件——镜像构建、独立 compose、systemd、部署指南"
```

---

## 自检结果

- **规格覆盖**：看门狗（任务 1）、优雅关停（任务 2）、Channel/process/Status 接入（任务 3）、Dockerfile/compose/systemd/部署文档（任务 4）一一对应规格 §3–§6；§7 风险（watcher 竞争、无限重启风暴、关停卡死、Windows、孤儿边界）由 `wantRunning`+`stopCh`+`procMu` 串行、退避封顶 30s、grace 5s+最终 Kill、HMAC 跨平台主路径、文档说明分别覆盖。
- **类型/命名一致性**：`supervisor` 字段与方法在任务 1–3 中命名统一（`wantRunning/stopCh/backingOff/failStreak/restarts/lifecycleCtx/backoff/grace/gracefulShutdown`、`watch/respawnLoop/spawn/terminate/waitReaped/restarting/restartCount/isAdopted/setBackingOff/killCmd/defaultBackoff`）；`c.admin.Shutdown(ctx)` 签名与 `adminClient` 接口一致；`AdapterStatus.Reason` 为自由字符串，新增 `"restarting"` 无需改结构。
- **Wait 回收约定**：全文一致——只有 `watch` 调 `cmd.Wait()`；`terminate`/`stop` 发信号后轮询 `ownsProcess()` 等回收；`killCmd`（kill+Wait）仅用于未起 watch 的 cmd（spawn 失败清理、respawn 放弃）。

## 执行交接

计划已完成并保存到 `docs/superpowers/plans/2026-09-08-adapter-supervisor-hardening.md`。




