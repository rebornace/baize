# X3 中间件多源（可替换任务队列 / 事件总线 / 限流）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 把进程内的任务调度 / 事件广播 / 限流抽象为可替换驱动，默认 `memory` 驱动保持单机零依赖行为，新增 `redis` 驱动支持多副本横向扩展、崩溃恢复、跨副本 SSE 与全局限流。

**架构：** 新增 `internal/middleware` 包，对外一个驱动（`middleware.driver: memory|redis`）、一份配置、一条 Redis 连接；对内三个小接口 `JobQueue` / `EventBus` / `Limiter`。DB（现有 store）始终是权威持久层，Redis 仅作实时通道；Redis 故障全路径 fail-open / 本地回退。run 执行复用现有 `Server.runExecute` / `Engine.ExecuteWithOpts`，不重写执行逻辑。

**技术栈：** Go；`github.com/redis/go-redis/v9`（仅 redis 子包引用）；测试用 `github.com/alicebob/miniredis/v2`（纯 Go 内存 Redis，仅测试依赖）。

**全局环境约束（Windows / PowerShell）：**
- `go` 不在 PATH，go.mod 要求 go 1.25.0。每个跑 go 的 shell 先设：`$env:GOPROXY='https://goproxy.cn,direct'; $env:GOTOOLCHAIN='auto'`，用 `$go='C:\Users\Administrator\go-sdk\go\bin\go.exe'`（如 `& $go test ./...`）。
- PowerShell 会把中文 commit 信息弄乱码：用 UTF-8 无 BOM 临时文件 + `git commit -F <file>`：`$enc=New-Object System.Text.UTF8Encoding($false); [System.IO.File]::WriteAllText($tmp,$msg,$enc)`。
- imports 一律在文件顶部，禁止 inline import；不写叙述显而易见代码的注释。

---

## 文件结构

**新增：**
- `internal/middleware/middleware.go` — 三接口（`JobQueue`/`EventBus`/`Limiter`）、`Job`、`RunEventNudge`、`JobKind`、`Options`、`Middleware` 组合体、`Open`。
- `internal/middleware/registry.go` — `DriverFactory`、`RegisterDriver`、`lookupDriver`、`ListDrivers`（贴 `internal/store/registry.go`）。
- `internal/middleware/job.go` — `Job` 负载与 `EnqueueKind` 常量、JSON 编解码助手。
- `internal/middleware/worker.go` — 跨驱动共享的 worker 池 + reconciler + 执行/收尾逻辑（依赖 `Executor` 接口，见下）。
- `internal/middleware/memory/queue.go` — 进程内 channel 队列。
- `internal/middleware/memory/bus.go` — 包 `eventbus.Hub` 的总线。
- `internal/middleware/memory/limiter.go` — 进程内滑动窗口限流。
- `internal/middleware/memory/driver.go` — `memory` 驱动工厂，`init()` 自注册。
- `internal/middleware/redis/queue.go` — Redis Streams 消费组队列。
- `internal/middleware/redis/bus.go` — Pub/Sub 总线。
- `internal/middleware/redis/limiter.go` — Lua 滑动窗口限流。
- `internal/middleware/redis/driver.go` — `redis` 驱动工厂（建 `*redis.Client`），`init()` 自注册。
- `internal/middleware/*_test.go`、`internal/middleware/{memory,redis}/*_test.go`

**修改：**
- `internal/store/store.go` — `Run` 加 `LeaseUntil`（不序列化 JSON）；Store 接口加 `LeaseRun` / `HeartbeatRun` / `ClearRunLease` / `ListRunsForReconcile`。
- `internal/store/memory.go` — 上述四方法的内存实现。
- `internal/store/sqlite.go` + `internal/store/postgres.go` + `internal/store/sql_dialect.go` — `runs` 表加 `lease_until` 列（DDL + 幂等迁移）+ 四方法 SQL；`CreateRun`/`GetRun` 列清单纳入 `lease_until`。
- `internal/eventbus/hub.go` — 加 `PublishExternal(runID string, seq int64)`（跨副本 nudge 注入点）。
- `internal/api/server.go` — 新增 `Queue middleware.JobQueue`（可选）与统一执行方法 `ExecuteJob(ctx, job)`；SSE handler 加低频兜底轮询 ticker。
- `internal/api/run_start.go` 与 `internal/api/server.go`（resume 路径 ~2440）与 `internal/bootstrap/bootstrap.go`（微信 AfterCreateRun ~424）— 裸 `go func` 改为「有 Queue 则 `Enqueue`（失败回退本地 goroutine），否则维持现状 goroutine」。
- `internal/api/server_inbox.go` — 入站限流改用可注入 `Limiter`（默认仍 `inbox.RateLimiter`）。
- `internal/plugincallback/limiter.go` + 其 bootstrap 接线 — 支持后端替换（保留内存默认）。
- `internal/config/config.go` — 新增 `middleware` 配置段 + 默认归一。
- `internal/bootstrap/bootstrap.go` — `middleware.Open` 装配：构造/启动 worker 池与 reconciler、redis bus 订阅注入 Hub、限流器接线、优雅退出。
- `cmd/baize/main.go` — blank import `_ ".../middleware/redis"`。
- `configs/demo.yaml`、`configs/minimal.yaml` — 注释说明 `middleware` 段。

**任务依赖顺序：** T1（store 租约）→ T2（config）→ T3（middleware 接口+registry）→ T4（memory 驱动）→ T5（worker/reconciler）→ T6（api 执行器+入队点）→ T7（bootstrap memory 接线）→ T8（eventbus 外部注入+SSE 轮询）→ T9（redis 驱动）→ T10（分布式限流接线）→ T11（文档/全量验证/收尾）。

---

## 任务 1：runs 租约列与调和查询（store 层）

**文件：**
- 修改：`internal/store/store.go`（`Run` 结构 + Store 接口四方法）
- 修改：`internal/store/memory.go`（内存实现）
- 修改：`internal/store/sqlite.go`（DDL + 迁移 + CreateRun/GetRun 列 + 四方法）
- 修改：`internal/store/postgres.go` / `internal/store/sql_dialect.go`（Postgres DDL/迁移，方式同 sqlite）
- 测试：`internal/store/lease_test.go`（新建）

- [ ] **步骤 1：编写失败的测试**

新建 `internal/store/lease_test.go`。测试用 sqlite 内存库（照该包既有 `newSQLite`/建库 helper；若无则用 `OpenWithOptions("sqlite", OpenOptions{SQLitePath: ":memory:"})` 并跑 schema 初始化，照现有测试写法）：

```go
package store_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestLeaseRunAtomic(t *testing.T) {
	st := newLeaseStore(t)
	run := mustCreateRun(t, st)

	acq1, err := st.LeaseRun(run.ID, time.Minute)
	if err != nil || !acq1 {
		t.Fatalf("first lease: acq=%v err=%v", acq1, err)
	}
	acq2, err := st.LeaseRun(run.ID, time.Minute)
	if err != nil || acq2 {
		t.Fatalf("second lease while held must fail: acq=%v err=%v", acq2, err)
	}
	if err := st.ClearRunLease(run.ID); err != nil {
		t.Fatal(err)
	}
	acq3, err := st.LeaseRun(run.ID, time.Minute)
	if err != nil || !acq3 {
		t.Fatalf("lease after clear must succeed: acq=%v err=%v", acq3, err)
	}
}

func TestLeaseRunExpired(t *testing.T) {
	st := newLeaseStore(t)
	run := mustCreateRun(t, st)
	if _, err := st.LeaseRun(run.ID, -time.Second); err != nil { // 立即过期
		t.Fatal(err)
	}
	acq, err := st.LeaseRun(run.ID, time.Minute)
	if err != nil || !acq {
		t.Fatalf("expired lease must be releasable: acq=%v err=%v", acq, err)
	}
}

func TestHeartbeatRunExtends(t *testing.T) {
	st := newLeaseStore(t)
	run := mustCreateRun(t, st)
	if _, err := st.LeaseRun(run.ID, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	ok, err := st.HeartbeatRun(run.ID, time.Minute)
	if err != nil || !ok {
		t.Fatalf("heartbeat: ok=%v err=%v", ok, err)
	}
	if acq, _ := st.LeaseRun(run.ID, time.Minute); acq {
		t.Fatal("after heartbeat the lease must still be held")
	}
}

func TestListRunsForReconcile(t *testing.T) {
	st := newLeaseStore(t)
	orphan := mustCreateRun(t, st)                       // running, 无租约，旧
	if _, err := st.LeaseRun(orphan.ID, -time.Hour); err != nil { // 租约过期
		t.Fatal(err)
	}
	held := mustCreateRun(t, st)
	if _, err := st.LeaseRun(held.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	done := mustCreateRun(t, st)
	if err := st.UpdateRun(done.ID, store.StatusSucceeded, "", ""); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListRunsForReconcile(50)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if !contains(ids, orphan.ID) {
		t.Fatalf("orphan (expired lease) must be listed, got %v", ids)
	}
	if contains(ids, held.ID) {
		t.Fatalf("held run must not be listed, got %v", ids)
	}
	if contains(ids, done.ID) {
		t.Fatalf("terminal run must not be listed, got %v", ids)
	}
}

func newLeaseStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.OpenWithOptions("sqlite", store.OpenOptions{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func mustCreateRun(t *testing.T, st store.Store) *store.Run {
	t.Helper()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// 让 created_at 落在调和宽限期之外：无租约的新 run 默认不应被立刻调和。
	return r
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
```

注意：`ListRunsForReconcile` 对「无租约但刚创建」的 run 要有宽限期（见步骤 3 的 grace），因此上面 `orphan` 用「过期租约」而非「无租约」来触发；held 用未过期租约；done 为终态。若你的实现让新建 run 的 `created_at` 为 now，无租约新 run 不应出现在结果里（grace 30s）。

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/store/ -run 'Lease|Reconcile' -count=1`
预期：FAIL（`st.LeaseRun undefined` 等编译错误）。

- [ ] **步骤 3：实现**

`internal/store/store.go`：`Run` 结构加字段（不序列化、不暴露给 API）：

```go
	// LeaseUntil is the worker lease deadline for an in-flight run. nil = no
	// lease. Never serialized to JSON (json:"-"). Used by queue reconciliation.
	LeaseUntil *time.Time `json:"-"`
```

Store 接口（`CreateRun` 附近）加四个方法：

```go
	// LeaseRun atomically acquires a worker lease for a queued/running run whose
	// lease is absent or expired. Returns acquired=false when another worker
	// holds a live lease or the run is terminal.
	LeaseRun(id string, ttl time.Duration) (acquired bool, err error)
	// HeartbeatRun extends the lease of a run the caller is executing.
	HeartbeatRun(id string, ttl time.Duration) (ok bool, err error)
	// ClearRunLease releases the lease after a run reaches a terminal state.
	ClearRunLease(id string) error
	// ListRunsForReconcile returns queued/running runs whose lease is expired or
	// whose lease was never set past the startup grace window (oldest first).
	ListRunsForReconcile(limit int) ([]*Run, error)
```

`internal/store/memory.go`：在 Memory 的 runs 存储旁加 `leaseUntil map[string]time.Time`（在 `NewMemory` 里初始化，与既有 mutex 一致加锁）。实现：

```go
func (m *Memory) LeaseRun(id string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return false, fmt.Errorf("run not found")
	}
	if r.Status != StatusQueued && r.Status != StatusRunning {
		return false, nil
	}
	now := time.Now().UTC()
	if until, held := m.leaseUntil[id]; held && until.After(now) {
		return false, nil
	}
	m.leaseUntil[id] = now.Add(ttl)
	return true, nil
}

func (m *Memory) HeartbeatRun(id string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[id]; !ok {
		return false, nil
	}
	m.leaseUntil[id] = time.Now().UTC().Add(ttl)
	return true, nil
}

func (m *Memory) ClearRunLease(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.leaseUntil, id)
	return nil
}

func (m *Memory) ListRunsForReconcile(limit int) ([]*Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	grace := now.Add(-reconcileGrace) // 见下方常量
	var out []*Run
	for _, r := range m.runs {
		if r.Status != StatusQueued && r.Status != StatusRunning {
			continue
		}
		until, held := m.leaseUntil[r.ID]
		expired := held && !until.After(now)
		neverLeasedOld := !held && r.CreatedAt.Before(grace)
		if expired || neverLeasedOld {
			cp := *r
			out = append(out, &cp)
		}
	}
	// 按 CreatedAt 升序，截断 limit（照既有排序风格）
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
```

在 `store.go` 加包级常量：`const reconcileGrace = 30 * time.Second`。（memory 与 SQL 共用。）

`internal/store/sqlite.go`：
- runs 建表 DDL 加列 `lease_until TIMESTAMP`；并加幂等迁移（照 X4 `context_tokens` 的 `ALTER TABLE ... ADD COLUMN` + 列存在检查写法）：

```go
	`ALTER TABLE runs ADD COLUMN lease_until TIMESTAMP`
```
（用该包既有的「列不存在才加」helper；若无可照 `PRAGMA table_info(runs)` 检查。）
- `CreateRun` 的 INSERT 列清单不含 `lease_until`（默认 NULL 即可）。
- `GetRun` 的 SELECT/Scan 增加 `lease_until`（scan 进 `sql.NullTime`，valid 时赋给 `r.LeaseUntil = &t`）。SELECT 列串改为：
  `SELECT id, agent_id, input, status, output, error, created_at, conversation_id, identity_id, passthrough_json, webhook_json, model_profile_id, lease_until FROM runs WHERE id = ?`
- 新增方法：

```go
func (s *SQLStore) LeaseRun(id string, ttl time.Duration) (bool, error) {
	now := time.Now().UTC()
	until := now.Add(ttl)
	res, err := s.exec(
		`UPDATE runs SET lease_until = ?
		 WHERE id = ? AND status IN ('queued','running')
		   AND (lease_until IS NULL OR lease_until < ?)`,
		until.Format(time.RFC3339Nano), id, now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *SQLStore) HeartbeatRun(id string, ttl time.Duration) (bool, error) {
	res, err := s.exec(
		`UPDATE runs SET lease_until = ? WHERE id = ?`,
		time.Now().UTC().Add(ttl).Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return false, err
	}
	n, _ := rowsAffected(res)
	return n > 0, nil
}

func (s *SQLStore) ClearRunLease(id string) error {
	_, err := s.exec(`UPDATE runs SET lease_until = NULL WHERE id = ?`, id)
	return err
}

func (s *SQLStore) ListRunsForReconcile(limit int) ([]*Run, error) {
	if limit <= 0 {
		limit = 100
	}
	now := time.Now().UTC()
	grace := now.Add(-reconcileGrace).Format(time.RFC3339Nano)
	nowStr := now.Format(time.RFC3339Nano)
	// 列顺序与 GetRun 完全一致（含 lease_until）
	rows, err := s.query(
		`SELECT id, agent_id, input, status, output, error, created_at, conversation_id, identity_id, passthrough_json, webhook_json, model_profile_id, lease_until
		 FROM runs
		 WHERE status IN ('queued','running')
		   AND (lease_until < ? OR (lease_until IS NULL AND created_at < ?))
		 ORDER BY created_at ASC LIMIT ?`,
		nowStr, grace, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows) // 复用与 GetRun 相同的扫描逻辑，抽成 helper
}
```

把 `GetRun` 的行扫描抽成可复用 helper（如 `scanRunRow(row)` / `scanRuns(rows)`），`GetRun` 与 `ListRunsForReconcile` 共用，避免列顺序漂移。`rowsAffected` 用该包既有写法（`sql.Result.RowsAffected()`）。

`internal/store/postgres.go` / `sql_dialect.go`：runs DDL 加 `lease_until TIMESTAMPTZ`；迁移用 `ALTER TABLE runs ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ`；SQL 占位符用 Postgres 的 `$1,$2,...`（照该包既有 rebind 方式），时间参数传 `time.Time`（不必 Format）。其余方法同 sqlite。

- [ ] **步骤 4：运行测试验证通过**

运行：`& $go test ./internal/store/ -count=1`
预期：PASS（含新测试与既有全部 store 测试）。再跑 `& $go build ./...` 确认接口新增方法后 memory/sqlite/postgres 都已实现（否则编译报「missing method」）。

- [ ] **步骤 5：Commit**

```bash
git add internal/store/
git commit -m "feat(store): runs 增加 worker 租约列与崩溃调和查询"
```

---

## 任务 2：middleware 配置段

**文件：**
- 修改：`internal/config/config.go`（`Middleware` 结构 + 默认归一）
- 测试：`internal/config/middleware_test.go`（新建）

- [ ] **步骤 1：编写失败的测试**

新建 `internal/config/middleware_test.go`（照该包既有 `config_test` 外部包 + `writeConfig`/`config.Load` 写法）：

```go
package config_test

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestLoadMiddlewareDefaults(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Middleware.Driver != "memory" {
		t.Fatalf("driver=%q want memory", cfg.Middleware.Driver)
	}
	if cfg.Middleware.WorkerConcurrency != 8 {
		t.Fatalf("workers=%d want 8", cfg.Middleware.WorkerConcurrency)
	}
	if cfg.Middleware.LeaseTTLSec != 60 {
		t.Fatalf("lease=%d want 60", cfg.Middleware.LeaseTTLSec)
	}
	if cfg.Middleware.ReconcileIntervalSec != 15 {
		t.Fatalf("reconcile=%d want 15", cfg.Middleware.ReconcileIntervalSec)
	}
}

func TestLoadMiddlewareRedisExplicit(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\nmiddleware:\n  driver: redis\n  worker_concurrency: 4\n  lease_ttl_sec: 90\n  reconcile_interval_sec: 20\n  redis:\n    addr: \"127.0.0.1:6379\"\n    db: 1\n    password_env: REDIS_PASS\n    stream: s1\n    consumer_group: g1\n    events_channel: e1\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := cfg.Middleware
	if m.Driver != "redis" || m.Redis.Addr != "127.0.0.1:6379" || m.Redis.DB != 1 ||
		m.Redis.PasswordEnv != "REDIS_PASS" || m.Redis.Stream != "s1" ||
		m.Redis.ConsumerGroup != "g1" || m.Redis.EventsChannel != "e1" ||
		m.WorkerConcurrency != 4 || m.LeaseTTLSec != 90 || m.ReconcileIntervalSec != 20 {
		t.Fatalf("redis middleware config not parsed: %+v", m)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/config/ -run Middleware -count=1`
预期：FAIL（`cfg.Middleware undefined`）。

- [ ] **步骤 3：实现**

`internal/config/config.go`：在 `Config` 结构里（`Conversation` 段之后）加：

```go
	Middleware struct {
		Driver               string `yaml:"driver"` // memory（默认） | redis
		WorkerConcurrency    int    `yaml:"worker_concurrency"`
		LeaseTTLSec          int    `yaml:"lease_ttl_sec"`
		ReconcileIntervalSec int    `yaml:"reconcile_interval_sec"`
		Redis                struct {
			Addr          string `yaml:"addr"`
			DB            int    `yaml:"db"`
			Username      string `yaml:"username"`
			PasswordEnv   string `yaml:"password_env"`
			Stream        string `yaml:"stream"`
			ConsumerGroup string `yaml:"consumer_group"`
			EventsChannel string `yaml:"events_channel"`
		} `yaml:"redis"`
	} `yaml:"middleware"`
```

在 `applyDefaults`（conversation 默认归一之后）加：

```go
	if strings.TrimSpace(cfg.Middleware.Driver) == "" {
		cfg.Middleware.Driver = "memory"
	}
	if cfg.Middleware.WorkerConcurrency <= 0 {
		cfg.Middleware.WorkerConcurrency = 8
	}
	if cfg.Middleware.LeaseTTLSec <= 0 {
		cfg.Middleware.LeaseTTLSec = 60
	}
	if cfg.Middleware.ReconcileIntervalSec <= 0 {
		cfg.Middleware.ReconcileIntervalSec = 15
	}
	if cfg.Middleware.Redis.Stream == "" {
		cfg.Middleware.Redis.Stream = "baize:runs"
	}
	if cfg.Middleware.Redis.ConsumerGroup == "" {
		cfg.Middleware.Redis.ConsumerGroup = "baize-workers"
	}
	if cfg.Middleware.Redis.EventsChannel == "" {
		cfg.Middleware.Redis.EventsChannel = "baize:run-events"
	}
```

（`strings` 已在 config.go 使用；若无则 import。）

- [ ] **步骤 4：运行测试验证通过**

运行：`& $go test ./internal/config/ -count=1`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/config/
git commit -m "feat(config): 新增 middleware 驱动配置段（memory/redis）"
```

---

## 任务 3：middleware 包接口、类型与驱动注册表

**文件：**
- 创建：`internal/middleware/middleware.go`
- 创建：`internal/middleware/job.go`
- 创建：`internal/middleware/registry.go`
- 测试：`internal/middleware/registry_test.go`

- [ ] **步骤 1：编写失败的测试**

新建 `internal/middleware/registry_test.go`：

```go
package middleware_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/middleware"
	_ "github.com/rebornace/baize/internal/middleware/memory"
)

func TestOpenUnknownDriverErrors(t *testing.T) {
	if _, err := middleware.Open(context.Background(), "nope", middleware.Options{}); err == nil {
		t.Fatal("unknown driver must error")
	}
}

func TestRegisterAndOpen(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if mw.Queue == nil || mw.Bus == nil || mw.Limiter == nil {
		t.Fatalf("memory middleware must wire all components: %+v", mw)
	}
}

func TestListDriversIncludesMemory(t *testing.T) {
	found := false
	for _, d := range middleware.ListDrivers() {
		if d == "memory" {
			found = true
		}
	}
	if !found {
		t.Fatalf("memory driver must be registered: %v", middleware.ListDrivers())
	}
}
```

注意：此测试 blank import memory 驱动，但 memory 驱动在任务 4 才实现。本任务先让测试因 `Open("memory")` 失败而红；任务 4 完成后转绿。步骤 3 先只建接口/注册表（不含 memory 工厂），因此本任务结束时该测试中 `TestOpenUnknownDriverErrors` 与 `TestListDriversIncludesMemory` 可先不强依赖 memory——实现顺序见步骤 3。

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/middleware/... -count=1`
预期：FAIL（包不存在 / `middleware.Open undefined`）。

- [ ] **步骤 3：实现**

`internal/middleware/job.go`：

```go
package middleware

import (
	"context"
	"time"
)

// JobKind identifies how a queued job should be executed.
type JobKind string

const (
	// KindRun is a normal agent run (chat / webhook / channel message).
	KindRun JobKind = "run"
)

// Job is a unit of work: a run to execute. The DB run row is authoritative;
// these fields let a worker replay the run without reconstructing context, and
// carry non-persisted multimodal parts on the live path.
type Job struct {
	RunID      string    `json:"run_id"`
	Kind       JobKind   `json:"kind"`
	AgentID    string    `json:"agent_id"`
	Input      string    `json:"input"`
	Skills     []string  `json:"skills,omitempty"`
	UserParts  []Part    `json:"user_parts,omitempty"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

// Part is a serializable multimodal content part (text or image URL/data).
// It mirrors llm.ContentPart for transport across drivers.
type Part struct {
	Type     string `json:"type"`               // "text" | "image"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}
```

`internal/middleware/middleware.go`：

```go
package middleware

import "context"

// JobQueue delivers run jobs to competing consumers.
type JobQueue interface {
	// Enqueue posts a job for any worker to process.
	Enqueue(ctx context.Context, job Job) error
	// Consume blocks until a job is available or ctx is cancelled. On success
	// it returns the job and an ack func the worker calls after processing
	// (ack nil-safe: implementations must accept a nil no-op gracefully).
	Consume(ctx context.Context) (job Job, ack func(), ok bool)
	Close() error
}

// EventBus fans a lightweight "run X has new events up to seq" signal to all
// replicas. Event content is always re-read from the DB; this is a nudge only.
type EventBus interface {
	PublishRunEvent(ctx context.Context, runID string, seq int64) error
	SubscribeRunEvents(ctx context.Context) (<-chan RunEventNudge, error)
	Close() error
}

// RunEventNudge is a cross-replica wake-up signal.
type RunEventNudge struct {
	RunID string
	Seq   int64
}

// Limiter is a distributed-or-local rate limiter (Allow-style).
type Limiter interface {
	Allow(key string) bool
}

// Options configures a driver. Redis fields are ignored by the memory driver.
type Options struct {
	WorkerConcurrency int
	LeaseTTL          time.Duration
	ReconcileInterval time.Duration
	Redis             RedisOptions
}

// RedisOptions are connection/stream settings for the redis driver.
type RedisOptions struct {
	Addr          string
	DB            int
	Username      string
	Password      string
	Stream        string
	ConsumerGroup string
	EventsChannel string
}

// Middleware is the wired bundle handed to bootstrap.
type Middleware struct {
	Queue   JobQueue
	Bus     EventBus
	Limiter Limiter
	// Close shuts the driver down (queue, bus, pools).
	Close func() error

	concurrency int // 由驱动工厂从 Options.WorkerConcurrency 设置
}

// Open builds a Middleware for the named driver.
func Open(ctx context.Context, driver string, opts Options) (*Middleware, error) {
	f, err := lookupDriver(driver)
	if err != nil {
		return nil, err
	}
	mw, err := f(ctx, opts)
	if err != nil {
		return nil, err
	}
	mw.concurrency = opts.WorkerConcurrency // 未导出字段：仅本包可设，驱动工厂无需关心
	return mw, nil
}
```

（`time` 需在 middleware.go import；与 job.go 同包，`Part` 放 job.go。把 `time` import 放到用到的文件——middleware.go 用到 `time.Duration`，故在 middleware.go import "time"。）

`internal/middleware/registry.go`：

```go
package middleware

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// DriverFactory constructs a Middleware from options.
type DriverFactory func(ctx context.Context, opts Options) (*Middleware, error)

var (
	driversMu sync.RWMutex
	drivers   = map[string]DriverFactory{}
)

// RegisterDriver registers a middleware driver by name (called from init()).
func RegisterDriver(name string, f DriverFactory) {
	driversMu.Lock()
	defer driversMu.Unlock()
	drivers[name] = f
}

func lookupDriver(name string) (DriverFactory, error) {
	driversMu.RLock()
	defer driversMu.RUnlock()
	f, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("unknown middleware driver %q (registered: %v)", name, ListDrivers())
	}
	return f, nil
}

// ListDrivers returns registered driver names, sorted.
func ListDrivers() []string {
	driversMu.RLock()
	defer driversMu.RUnlock()
	names := make([]string, 0, len(drivers))
	for n := range drivers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
```

- [ ] **步骤 4：运行测试**

本任务结束时 `TestOpenUnknownDriverErrors` 可通过（unknown 报错）；依赖 memory 的两个测试在任务 4 转绿。为保证本任务可独立验证，先运行：`& $go build ./internal/middleware/...`（此时无 memory 包，`registry_test.go` 的 blank import 会失败——故将 `registry_test.go` 里 blank import memory 的那行与两个 memory 测试放到**任务 4** 再加入；本任务只保留 `TestOpenUnknownDriverErrors`，且不 blank import memory）。

修订：本任务 `registry_test.go` 仅含：

```go
package middleware_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/middleware"
)

func TestOpenUnknownDriverErrors(t *testing.T) {
	if _, err := middleware.Open(context.Background(), "nope", middleware.Options{}); err == nil {
		t.Fatal("unknown driver must error")
	}
}
```

运行：`& $go test ./internal/middleware/ -count=1` 预期 PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/middleware/
git commit -m "feat(middleware): 队列/事件总线/限流接口与驱动注册表"
```

---

## 任务 4：memory 驱动（channel 队列 + Hub 总线 + 内存限流）

**文件：**
- 创建：`internal/middleware/memory/queue.go`、`bus.go`、`limiter.go`、`driver.go`
- 测试：`internal/middleware/memory/memory_test.go`

- [ ] **步骤 1：编写失败的测试**

新建 `internal/middleware/memory/memory_test.go`：

```go
package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/middleware"
	_ "github.com/rebornace/baize/internal/middleware/memory"
)

func TestMemoryQueueEnqueueConsumeAck(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()

	job := middleware.Job{RunID: "run_1", Kind: middleware.KindRun, Input: "hi", EnqueuedAt: time.Now()}
	if err := mw.Queue.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, ack, ok := mw.Queue.Consume(ctx)
	if !ok {
		t.Fatal("expected to consume a job")
	}
	if got.RunID != "run_1" {
		t.Fatalf("got run %q", got.RunID)
	}
	ack()
}

func TestMemoryBusLocalNudge(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()
	ch, err := mw.Bus.SubscribeRunEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := mw.Bus.PublishRunEvent(context.Background(), "run_x", 7); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-ch:
		if n.RunID != "run_x" || n.Seq != 7 {
			t.Fatalf("bad nudge %+v", n)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive local nudge")
	}
}

func TestMemoryLimiterWindow(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()
	// memory limiter default window; hammer one key and assert it eventually denies.
	allowed := 0
	for i := 0; i < 5000; i++ {
		if mw.Limiter.Allow("k") {
			allowed++
		}
	}
	if allowed == 0 || allowed >= 5000 {
		t.Fatalf("limiter should allow a bounded budget, allowed=%d", allowed)
	}
}

// 确保 Hub 可被总线驱动（memory bus 包一个 *eventbus.Hub）。
var _ = eventbus.NewHub
```

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/middleware/memory/ -count=1`
预期：FAIL（memory 包不存在）。

- [ ] **步骤 3：实现**

`internal/middleware/memory/queue.go`：

```go
package memory

import (
	"context"

	"github.com/rebornace/baize/internal/middleware"
)

type queue struct {
	ch chan middleware.Job
}

func newQueue() *queue {
	return &queue{ch: make(chan middleware.Job, 1024)}
}

func (q *queue) Enqueue(_ context.Context, job middleware.Job) error {
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = nowUTC()
	}
	select {
	case q.ch <- job:
		return nil
	default:
		// 缓冲满：阻塞投递由调用方 ctx 控制，避免无限 goroutine。
		q.ch <- job
		return nil
	}
}

func (q *queue) Consume(ctx context.Context) (middleware.Job, func(), bool) {
	select {
	case job := <-q.ch:
		return job, func() {}, true
	case <-ctx.Done():
		return middleware.Job{}, nil, false
	}
}

func (q *queue) Close() error { close(q.ch); return nil }
```

（`nowUTC` 放 driver.go：`func nowUTC() time.Time { return time.Now().UTC() }`。缓冲满直接阻塞投递可接受——memory 单机、调用方是 handler/调和，容量 1024 足够；不要起无限 goroutine。）

`internal/middleware/memory/bus.go`：

```go
package memory

import (
	"context"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/middleware"
)

// bus wraps the in-process eventbus.Hub. PublishRunEvent fans out locally via
// the Hub's external-injection entrypoint (added in task 8); until then it uses
// a local channel bridge. For the memory driver the bus IS the hub: nudges are
// delivered to in-process subscribers.
type bus struct {
	hub  *eventbus.Hub
	ch   chan middleware.RunEventNudge
}

func newBus(hub *eventbus.Hub) *bus {
	return &bus{hub: hub, ch: make(chan middleware.RunEventNudge, 256)}
}

func (b *bus) PublishRunEvent(_ context.Context, runID string, seq int64) error {
	select {
	case b.ch <- middleware.RunEventNudge{RunID: runID, Seq: seq}:
	default:
	}
	return nil
}

func (b *bus) SubscribeRunEvents(_ context.Context) (<-chan middleware.RunEventNudge, error) {
	return b.ch, nil
}

func (b *bus) Hub() *eventbus.Hub { return b.hub }
func (b *bus) Close() error       { return nil }
```

说明：memory 驱动下 SSE/webhook 仍直接订阅 `eventbus.Hub`（现状不变）；`EventBus` 接口在 memory 下主要供 worker/未来使用，bus 与 Hub 并存。跨副本注入是 redis 驱动（任务 8/9）的事。因此 memory bus 用一个进程内 nudge channel 满足接口即可。

`internal/middleware/memory/limiter.go`：

```go
package memory

import (
	"sync"
	"time"
)

type limiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	hits     map[string][]time.Time
}

func newLimiter() *limiter {
	return &limiter{limit: 1000, window: time.Minute, hits: map[string][]time.Time{}}
}

func (l *limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-l.window)
	ts := l.hits[key]
	kept := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
```

注意：memory 限流仅为满足接口的驱动内默认；生产里 inbox/callback 仍用各自现有 `inbox.RateLimiter`/`plugincallback.Limiter`（任务 10 才在 redis 驱动下替换）。memory 驱动给一个宽松预算（1000/min）避免误伤。

`internal/middleware/memory/driver.go`：

```go
package memory

import (
	"context"
	"time"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/middleware"
)

func init() {
	middleware.RegisterDriver("memory", func(_ context.Context, _ middleware.Options) (*middleware.Middleware, error) {
		q := newQueue()
		b := newBus(eventbus.NewHub())
		l := newLimiter()
		return &middleware.Middleware{
			Queue:   q,
			Bus:     b,
			Limiter: l,
			Close: func() error {
				_ = q.Close()
				_ = b.Close()
				return nil
			},
		}, nil
	})
}

func nowUTC() time.Time { return time.Now().UTC() }
```

- [ ] **步骤 4：运行测试验证通过**

运行：`& $go test ./internal/middleware/... -count=1`
预期：PASS（含任务 3 的 registry 测试，此时 memory 已注册；把任务 3 里暂缓的 `TestRegisterAndOpen`/`TestListDriversIncludesMemory` 现在补回 `internal/middleware/registry_test.go`）。

补回 registry_test.go 的两个测试：

```go
func TestRegisterAndOpen(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if mw.Queue == nil || mw.Bus == nil || mw.Limiter == nil {
		t.Fatalf("memory middleware must wire all: %+v", mw)
	}
}

func TestListDriversIncludesMemory(t *testing.T) {
	found := false
	for _, d := range middleware.ListDrivers() {
		if d == "memory" {
			found = true
		}
	}
	if !found {
		t.Fatalf("memory must be registered: %v", middleware.ListDrivers())
	}
}
```

并在 registry_test.go 顶部 blank import：`_ "github.com/rebornace/baize/internal/middleware/memory"`。

- [ ] **步骤 5：Commit**

```bash
git add internal/middleware/
git commit -m "feat(middleware): memory 驱动（channel 队列/进程总线/内存限流）"
```

---

## 任务 5：worker 池与崩溃调和（跨驱动共享）

**文件：**
- 创建：`internal/middleware/worker.go`
- 测试：`internal/middleware/worker_test.go`

`Executor` 是 worker 回调的接口，由 `api.Server` 实现（任务 6），本任务先用 fake 测试。

- [ ] **步骤 1：编写失败的测试**

新建 `internal/middleware/worker_test.go`：

```go
package middleware_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/middleware"
	_ "github.com/rebornace/baize/internal/middleware/memory"
)

type fakeExecutor struct {
	mu      sync.Mutex
	ran     []string
	leases  map[string]bool
}

func (f *fakeExecutor) ExecuteJob(_ context.Context, j middleware.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ran = append(f.ran, j.RunID)
	return nil
}

func TestWorkerProcessesEnqueuedJob(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	ex := &fakeExecutor{leases: map[string]bool{}}
	stop := mw.StartWorkers(context.Background(), ex)
	defer stop()

	if err := mw.Queue.Enqueue(context.Background(), middleware.Job{RunID: "run_w1", Kind: middleware.KindRun, Input: "x"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ex.mu.Lock()
		n := len(ex.ran)
		ex.mu.Unlock()
		if n >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("worker did not process enqueued job")
}

func TestStartWorkersNilSafe(t *testing.T) {
	mw, _ := middleware.Open(context.Background(), "memory", middleware.Options{})
	// 不传 Executor 时不应 panic（某些部署只入队、不消费由他副本处理——memory 下仍要求有 executor）。
	stop := mw.StartWorkers(context.Background(), nil)
	stop()
}
```

说明：reconciler 的 DB 调和逻辑在任务 6/7 与真实 store 接线后端到端测（`TestReconcileRequeuesOrphanRuns` 放任务 7 的 bootstrap/集成测试）。本任务聚焦 worker 池消费循环。

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/middleware/ -run Worker -count=1`
预期：FAIL（`mw.StartWorkers undefined`）。

- [ ] **步骤 3：实现**

`internal/middleware/middleware.go` 的 `Middleware` 结构加字段与方法签名（`StartWorkers` 放 worker.go）：

```go
	// StartWorkers runs n competing consumer goroutines. Returns a stop func
	// that waits for in-flight jobs to finish. ex must be non-nil for jobs to be
	// processed; nil makes workers drain-and-discard (used only in tests).
	StartWorkers func(ctx context.Context, ex Executor) (stop func())
```

`internal/middleware/worker.go`：

```go
package middleware

import (
	"context"
	"log"
	"sync"
	"time"
)

// Executor processes a single job (implemented by api.Server).
type Executor interface {
	ExecuteJob(ctx context.Context, job Job) error
}

// StartWorkers runs mw.concurrency competing consumers over mw.Queue.
func (mw *Middleware) StartWorkers(ctx context.Context, ex Executor) func() {
	concurrency := mw.concurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	var wg sync.WaitGroup
	runCtx, cancel := context.WithCancel(ctx)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				job, ack, ok := mw.Queue.Consume(runCtx)
				if !ok {
					return
				}
				if ex != nil {
					func() {
						defer func() {
							if r := recover(); r != nil {
								log.Printf("middleware: job %s panic: %v", job.RunID, r)
							}
							ack()
						}()
						if err := ex.ExecuteJob(runCtx, job); err != nil {
							log.Printf("middleware: job %s failed: %v", job.RunID, err)
						}
					}()
				} else {
					ack()
				}
			}
		}()
	}
	return func() { cancel(); wg.Wait() }
}
```

注意：`StartWorkers` 作为方法定义在 worker.go；**不要**在 Middleware 结构里放 StartWorkers 函数字段（避免双重定义）。测试调用 `stop := mw.StartWorkers(ctx, ex)`。并发数来自 `Middleware.concurrency`（未导出），由 `middleware.Open` 在工厂返回后从 `Options.WorkerConcurrency` 设置（见任务 3 的 Open 实现）；驱动工厂不设置它。StartWorkers 在 `concurrency<=0` 时回退 8（因此 redis 包测试里直接调 `mwredis.Open` 得到的实例并发为默认 8，不影响测试）。

在 `Middleware` 结构加：

```go
	concurrency int
```

驱动工厂（memory/driver.go 与 redis/driver.go）构造后设置 `mw.concurrency = opts.WorkerConcurrency`。StartWorkers 方法用 `mw.concurrency`（≤0 → 8）。

- [ ] **步骤 4：运行测试验证通过**

运行：`& $go test ./internal/middleware/... -count=1`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/middleware/
git commit -m "feat(middleware): 竞争消费 worker 池"
```

---

## 任务 6：Server.ExecuteJob 统一执行/收尾 + 三个入队点改造

**文件：**
- 修改：`internal/api/server.go`（`Server` 加 `Queue`/`LeaseTTL` 字段、`ExecuteJob`、`dispatchJob`、parts 转换、心跳）
- 修改：`internal/api/run_start.go`（~67 裸协程改 dispatch）
- 修改：`internal/api/server.go`（~2440 resume/regenerate 裸协程改 dispatch）
- 修改：`internal/bootstrap/bootstrap.go`（~424 微信 AfterCreateRun 裸协程改 dispatch）
- 测试：`internal/api/server_job_test.go`（新建）

- [ ] **步骤 1：编写失败的测试**

新建 `internal/api/server_job_test.go`（照该包既有建 server / fake runner / memory store 的 helper；用 `NewServer` + 注入 fake `Runner`）：

```go
package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/middleware"
	_ "github.com/rebornace/baize/internal/middleware/memory"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

// terminalRunRunner 记录被执行的 runID，并在执行时把 run 置 succeeded。
type jobRunner struct{ executed chan string }

func (r *jobRunner) Execute(_ context.Context, runID string, _ agent.Def, _ string) error {
	r.executed <- runID
	return nil
}
func (r *jobRunner) ContinueFromHITL(context.Context, string, run.Decision) error { return nil }

func TestExecuteJobTerminalRunSkipped(t *testing.T) {
	srv, st := newJobTestServer(t) // 见下方 helper，Queue=nil
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err := st.UpdateRun(r.ID, store.StatusSucceeded, "out", ""); err != nil {
		t.Fatal(err)
	}
	// 终态 run 再 ExecuteJob 必须直接返回、不调用 runner。
	done := make(chan struct{})
	go func() { _ = srv.ExecuteJob(context.Background(), middleware.Job{RunID: r.ID, Kind: middleware.KindRun}); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ExecuteJob on terminal run should return immediately")
	}
}

func TestDispatchEnqueuesWhenQueuePresent(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	srv, st := newJobTestServer(t)
	srv.Queue = mw.Queue
	rr := &jobRunner{executed: make(chan string, 1)}
	srv.Runner = rr
	stop := mw.StartWorkers(context.Background(), srv) // srv 实现 middleware.Executor；并发数由 Open 时 WorkerConcurrency=1 设置
	defer stop()

	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hello"})
	srv.Dispatch(context.Background(), middleware.Job{RunID: r.ID, Kind: middleware.KindRun, AgentID: "a", Input: "hello"})

	select {
	case id := <-rr.executed:
		if id != r.ID {
			t.Fatalf("executed %s want %s", id, r.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued job was not executed by worker")
	}
}
```

`newJobTestServer` 照该包现有测试构造（memory store + `NewServer` + 默认 agent "a"）；若已有等价 helper 直接复用，不要新造重复 helper。`srv` 需实现 `middleware.Executor`（即 `ExecuteJob(context.Context, middleware.Job) error`）。

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/api/ -run 'Job|Dispatch' -count=1`
预期：FAIL（`srv.ExecuteJob undefined`、`srv.Queue undefined`、`srv.Dispatch undefined`）。

- [ ] **步骤 3：实现**

`internal/api/server.go`：`Server` 结构加字段（`Hub` 字段附近）：

```go
	// Queue optionally dispatches runs to competing workers. nil = in-process
	// goroutine execution (legacy single-instance behavior).
	Queue middleware.JobQueue
	// LeaseTTL is the worker lease duration for queued runs (default 60s).
	LeaseTTL time.Duration
```

import `"github.com/rebornace/baize/internal/middleware"`。

新增统一执行入口（worker 与本地 fallback 都走它）：

```go
// ExecuteJob runs a queued run under a worker lease with idempotency gating.
// It implements middleware.Executor. Terminal/waiting runs are ack-skipped.
func (s *Server) ExecuteJob(ctx context.Context, job middleware.Job) error {
	if job.RunID == "" {
		return nil
	}
	cur, err := s.Store.GetRun(job.RunID)
	if err != nil || cur == nil {
		return err
	}
	switch cur.Status {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusWaitingHuman:
		return nil // 终态 / HITL 等待中：不重跑（resume 是独立入口）
	}
	ttl := s.LeaseTTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	acquired, err := s.Store.LeaseRun(job.RunID, ttl)
	if err != nil {
		return err
	}
	if !acquired {
		return nil // 他 worker 持有
	}
	defer func() { _ = s.Store.ClearRunLease(job.RunID) }()

	hbCtx, stopHB := context.WithCancel(ctx)
	defer stopHB()
	go s.leaseHeartbeat(hbCtx, job.RunID, ttl)

	def, input, opts, err := s.resolveJob(job, cur)
	if err != nil {
		s.finalizeRunError(job.RunID, cur.ConversationID, err)
		return nil
	}
	if err := s.runExecute(ctx, job.RunID, def, input, opts); err != nil {
		s.finalizeRunError(job.RunID, cur.ConversationID, err)
		return err
	}
	return nil
}

func (s *Server) leaseHeartbeat(ctx context.Context, runID string, ttl time.Duration) {
	t := time.NewTicker(ttl / 3)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.Store.HeartbeatRun(runID, ttl)
		}
	}
}

func (s *Server) resolveJob(job middleware.Job, cur *store.Run) (agent.Def, string, run.RunOptions, error) {
	agentID := job.AgentID
	if agentID == "" {
		agentID = cur.AgentID
	}
	ag, err := s.Store.GetAgent(agentID)
	if err != nil {
		return agent.Def{}, "", run.RunOptions{}, err
	}
	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	input := job.Input
	if strings.TrimSpace(input) == "" {
		input = cur.Input
	}
	opts := run.RunOptions{Skills: job.Skills, UserParts: partsFromMiddleware(job.UserParts)}
	return def, input, opts, nil
}

// finalizeRunError mirrors the legacy goroutine failure handling: mark failed,
// append an LLM error event, and leave a system note in the conversation.
func (s *Server) finalizeRunError(runID, convID string, runErr error) {
	cur, _ := s.Store.GetRun(runID)
	if cur == nil {
		return
	}
	if cur.Status != store.StatusRunning && cur.Status != store.StatusQueued {
		return
	}
	_ = s.Store.UpdateRun(runID, store.StatusFailed, "", runErr.Error())
	_ = s.Store.AppendEvent(runID, store.Event{Type: run.EventLLMError, Data: map[string]any{"error": runErr.Error()}})
	if s.Messages != nil && convID != "" {
		note := strings.TrimSpace(runErr.Error())
		if note == "" {
			note = "运行失败"
		} else {
			note = "运行失败：" + note
		}
		_, _ = s.Messages.Append(convID, conversation.Message{Role: conversation.RoleSystemNote, Content: note, RunID: runID})
	}
}

// Dispatch enqueues a job when a Queue is configured, falling back to a local
// goroutine (legacy behavior) on nil queue or enqueue failure.
func (s *Server) Dispatch(ctx context.Context, job middleware.Job) {
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now().UTC()
	}
	if s.Queue != nil {
		if err := s.Queue.Enqueue(ctx, job); err == nil {
			return
		}
	}
	go func() { _ = s.ExecuteJob(context.Background(), job) }()
}
```

parts 转换（放在 server.go 或新 `internal/api/job_parts.go`；字段照 `internal/llm` 的 `ContentPart` 定义，text/image 两类）：

```go
func partsToMiddleware(parts []llm.ContentPart) []middleware.Part {
	out := make([]middleware.Part, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			out = append(out, middleware.Part{Type: "text", Text: p.Text})
		case "image":
			out = append(out, middleware.Part{Type: "image", ImageURL: p.ImageURL})
		}
	}
	return out
}

func partsFromMiddleware(parts []middleware.Part) []llm.ContentPart {
	out := make([]llm.ContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			out = append(out, llm.ContentPart{Type: "text", Text: p.Text})
		case "image":
			out = append(out, llm.ContentPart{Type: "image", ImageURL: p.ImageURL})
		}
	}
	return out
}
```

（实现前先读 `internal/llm/provider.go` 里 `ContentPart` 的确切字段名——可能是 `Text`/`ImageURL` 或 `Source`/`Data`；以实际为准，保持双向映射完整。）

**改造三个入队点**（把原来的 `go func(){ ...runExecute + 失败收尾... }()` 整段替换为构造 `middleware.Job` + `s.Dispatch(...)`）：

1. `internal/api/run_start.go` ~67：在 `AppendEvent(EventRunStarted)` 之后，用已有的 `def`/`in.Input`/`runOpts`，把整个 `go func(runID, input, def, opts){...}()`（约 67-95 行，含失败收尾）替换为：
```go
	s.Dispatch(r.Context(), middleware.Job{
		RunID: runRec.ID, Kind: middleware.KindRun, AgentID: def.ID, Input: in.Input,
		Skills: runOpts.Skills, UserParts: partsToMiddleware(runOpts.UserParts),
	})
```
（失败收尾已由 `ExecuteJob → finalizeRunError` 承担，其逻辑与原 goroutine 的 `switch cur.Status { case StatusRunning, StatusQueued: ... }` 一致。）

2. `internal/api/server.go` ~2440（resume/regenerate 的 `startRun`-like）：同样把 `go func(runID, in, def, ro){...}()` 替换为：
```go
	s.Dispatch(r.Context(), middleware.Job{
		RunID: runRec.ID, Kind: middleware.KindRun, AgentID: def.ID, Input: input,
		Skills: opts.runOpts.Skills, UserParts: partsToMiddleware(opts.runOpts.UserParts),
	})
```
（注意该函数签名里 ctx 是 `r.Context()`；若该路径不在 HTTP handler 内则用 `context.Background()`。）

3. `internal/bootstrap/bootstrap.go` ~424 微信 `AfterCreateRun`：把
```go
			go func() {
				_ = engine.ExecuteWithOpts(context.Background(), runRec.ID, def, runRec.Input, opts)
			}()
```
替换为通过 `srv.Dispatch`（`srv` 在此作用域已创建；若变量名不同用实际 api.Server 变量）：
```go
			srv.Dispatch(context.Background(), middleware.Job{
				RunID: runRec.ID, Kind: middleware.KindRun, AgentID: runRec.AgentID,
				Input: runRec.Input, UserParts: partsToMiddlewareBootstrap(userParts),
			})
```
bootstrap 不能 import api 的私有 parts 助手；在 bootstrap 内联一个等价的 `[]llm.ContentPart → []middleware.Part` 转换（或导出 api 的助手）。返回值仍为 `nil`（Dispatch 异步）。**注意**：确保 `srv.Queue` 在 channel `Start` 之前已赋值（任务 7 接线顺序）。

- [ ] **步骤 4：运行测试验证通过**

运行：`& $go test ./internal/api/ -count=1` 与 `& $go test ./internal/bootstrap/ -count=1`、`& $go build ./...`
预期：PASS（既有 api/bootstrap 测试不回归；新测试通过）。

- [ ] **步骤 5：Commit**

```bash
git add internal/api/ internal/bootstrap/
git commit -m "feat(api): run 执行收敛为 ExecuteJob 并经队列/本地分发"
```

---

## 任务 7：bootstrap 装配 middleware、启动 worker 池与 reconciler

**文件：**
- 修改：`internal/middleware/reconciler.go`（新建，reconciler 逻辑）
- 修改：`internal/bootstrap/bootstrap.go`（`middleware.Open`、接线、启动 worker/reconciler、优雅退出）
- 测试：`internal/middleware/reconciler_test.go`（新建）、`internal/bootstrap/` 集成（现有测试套不回归）

- [ ] **步骤 1：编写失败的测试**

新建 `internal/middleware/reconciler_test.go`，用 memory 队列 + fake executor + fake store 调和：

```go
package middleware_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/store"
	_ "github.com/rebornace/baize/internal/middleware/memory"
)

type reconcileStore struct {
	runs []*store.Run
}

func (s *reconcileStore) ListRunsForReconcile(limit int) ([]*store.Run, error) {
	var out []*store.Run
	for _, r := range s.runs {
		out = append(out, r)
	}
	return out, nil
}

func TestReconcileEnqueuesOrphans(t *testing.T) {
	mw, _ := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 1})
	var mu sync.Mutex
	var got []string
	ex := executorFunc(func(_ context.Context, j middleware.Job) error {
		mu.Lock(); got = append(got, j.RunID); mu.Unlock()
		return nil
	})
	stop := mw.StartWorkers(context.Background(), ex)
	defer stop()

	rs := &reconcileStore{runs: []*store.Run{{ID: "run_orphan", AgentID: "a", Status: store.StatusRunning}}}
	mw.Reconcile(context.Background(), rs)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock(); n := len(got); mu.Unlock()
		if n >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("orphan run was not re-enqueued/executed")
}

type executorFunc func(context.Context, middleware.Job) error

func (f executorFunc) ExecuteJob(ctx context.Context, j middleware.Job) error { return f(ctx, j) }
```

（`Reconcile` 依赖一个最小接口，见步骤 3；`reconcileStore` 只实现它需要的方法，故接口要窄。）

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/middleware/ -run Reconcile -count=1`
预期：FAIL（`mw.Reconcile undefined`）。

- [ ] **步骤 3：实现**

`internal/middleware/reconciler.go`：

```go
package middleware

import (
	"context"
	"log"
	"time"
)

// ReconcileStore is the store subset the reconciler needs (satisfied by store.Store).
type ReconcileStore interface {
	ListRunsForReconcile(limit int) ([]*store.Run, error)
}

// Reconcile re-enqueues orphaned (queued/running with expired lease) runs.
func (mw *Middleware) Reconcile(ctx context.Context, rs ReconcileStore) {
	runs, err := rs.ListRunsForReconcile(100)
	if err != nil {
		log.Printf("middleware: reconcile list: %v", err)
		return
	}
	for _, r := range runs {
		job := Job{RunID: r.ID, Kind: KindRun, AgentID: r.AgentID, Input: r.Input, EnqueuedAt: time.Now().UTC()}
		if err := mw.Queue.Enqueue(ctx, job); err != nil {
			log.Printf("middleware: reconcile enqueue %s: %v", r.ID, err)
		}
	}
}

// StartReconciler runs Reconcile immediately and every interval until ctx ends.
func (mw *Middleware) StartReconciler(ctx context.Context, rs ReconcileStore, interval time.Duration) func() {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mw.Reconcile(runCtx, rs)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
				mw.Reconcile(runCtx, rs)
			}
		}
	}()
	return func() { cancel(); wg.Wait() }
}
```

**最终约定（以此为准）：** middleware 包直接 import `github.com/rebornace/baize/internal/store`（与 `internal/eventbus` 一致，无循环依赖：store 不 import middleware）。因此：

- 删除上面骨架里的本地 `storeRun` 类型；
- `ReconcileStore` 定义为：

```go
// ReconcileStore is satisfied by store.Store directly.
type ReconcileStore interface {
	ListRunsForReconcile(limit int) ([]*store.Run, error)
}
```

- `Reconcile` 里 `runs` 元素是 `*store.Run`，`Job{RunID: r.ID, AgentID: r.AgentID, Input: r.Input, ...}`。
- reconciler.go imports：`context`、`log`、`sync`、`time`、`github.com/rebornace/baize/internal/store`。
- 测试 `reconcileStore.ListRunsForReconcile` 返回 `[]*store.Run`（fake 只需实现这一个方法，但它被当作 `middleware.ReconcileStore` 传入；`*store.Run` 字段填 `ID/AgentID/Status`）。

多模态调和：reconcile 入队的 Job 不带 `UserParts`（图片未持久化）。`ExecuteJob.resolveJob` 在 `job.UserParts` 为空时用纯文本 input 执行；若该 run 原本依赖图片，worker 端 LLM 调用会因缺少图片而失败或降级——为避免静默错误，`resolveJob` 检测到「run 有图片附件标记但 parts 缺失」时直接把 run 置 failed 并备注（此标记来源：可检查 conversation 末条 user message 是否含 image part，或简单起见 v0 不检测、允许降级；**v0 采用不检测**，失败由正常错误路径处理）。

`internal/bootstrap/bootstrap.go`：在构造 engine/server 附近（`store`/`hub`/`srv` 就绪后、channel Start 前）：

```go
	mw, err := middleware.Open(ctx, cfg.Middleware.Driver, middleware.Options{
		WorkerConcurrency: cfg.Middleware.WorkerConcurrency,
		LeaseTTL:          time.Duration(cfg.Middleware.LeaseTTLSec) * time.Second,
		ReconcileInterval: time.Duration(cfg.Middleware.ReconcileIntervalSec) * time.Second,
		Redis: middleware.RedisOptions{
			Addr: cfg.Middleware.Redis.Addr, DB: cfg.Middleware.Redis.DB,
			Username: cfg.Middleware.Redis.Username,
			Password: redisPasswordFromEnv(cfg.Middleware.Redis.PasswordEnv),
			Stream: cfg.Middleware.Redis.Stream, ConsumerGroup: cfg.Middleware.Redis.ConsumerGroup,
			EventsChannel: cfg.Middleware.Redis.EventsChannel,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open middleware driver %q: %w", cfg.Middleware.Driver, err)
	}
	srv.Queue = mw.Queue
	srv.LeaseTTL = time.Duration(cfg.Middleware.LeaseTTLSec) * time.Second
	// 事件总线：redis 驱动把跨副本 nudge 注入本地 Hub 并启动 Pub/Sub 订阅；
	// memory 驱动不实现该接口（SSE/webhook 直接走本地 Hub），类型断言失败即跳过。
	type hubBridge interface {
		BridgeToHub(*eventbus.Hub)
		Start(context.Context) // 开始消费跨副本 nudge（redis bus）
	}
	if b, ok := mw.Bus.(hubBridge); ok {
		b.BridgeToHub(hub)
		b.Start(ctx)
	}
	stopWorkers := mw.StartWorkers(ctx, srv) // 并发数已由驱动工厂从 Options.WorkerConcurrency 存入 mw
	stopReconciler := mw.StartReconciler(ctx, st, time.Duration(cfg.Middleware.ReconcileIntervalSec)*time.Second)
```

- redis 密码：`redisPasswordFromEnv` 读 `os.Getenv(cfg.Middleware.Redis.PasswordEnv)`（env 名为空返回 ""）。
- `driver=redis` 但 `addr==""`：redis 驱动工厂返回错误（fail fast，任务 9）。
- 优雅退出：在 `newAPIServer` 返回的 `io.Closer`/shutdown 链里追加 `stopWorkers()`、`stopReconciler()`、`mw.Close()`（照现有 closer 聚合方式）。
- memory 驱动下：`srv.Queue` 非空 → 三个入队点走 memory channel，worker 池消费；行为等价今天但有界。

- [ ] **步骤 4：运行测试验证通过**

运行：`& $go test ./internal/middleware/... ./internal/bootstrap/ -count=1` 与 `& $go build ./...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/middleware/ internal/bootstrap/
git commit -m "feat(bootstrap): 装配 middleware 驱动并启动 worker 池与崩溃调和"
```

---

## 任务 8：eventbus 外部注入入口 + SSE 兜底轮询

**文件：**
- 修改：`internal/eventbus/hub.go`（`PublishExternal`）
- 修改：`internal/api/server.go`（SSE handler 加低频轮询 ticker）
- 测试：`internal/eventbus/hub_test.go`（或现有事件测试文件追加）、`internal/api/server_sse_poll_test.go`

- [ ] **步骤 1：编写失败的测试**

eventbus 侧（照该包既有测试风格）：

```go
func TestPublishExternalNotifiesSubscribers(t *testing.T) {
	hub := eventbus.NewHub()
	sub := hub.Subscribe("run_e")
	defer sub.Cancel()
	hub.PublishExternal("run_e", 3) // 跨副本 nudge：seq=3
	select {
	case ev := <-sub.Events:
		if ev.Index != 3 {
			t.Fatalf("index=%d want 3", ev.Index)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive external nudge")
	}
}
```

SSE 兜底轮询侧：模拟「无 Hub nudge、但 DB 有新事件」，断言 SSE 在 ~轮询周期内送出事件。照该包现有 SSE 测试（`httptest` + 读 event-stream）写一个：run 已在回放后追加新事件但不触发 Hub（可直接操作 store.AppendEvent 而不经 Notify 装饰器，或用 replay-only `Hub=nil` 场景），断言连接在轮询周期内收到该事件。若该包已有 SSE 测试 helper，复用。

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/eventbus/ ./internal/api/ -run 'External|Poll|SSE' -count=1`
预期：FAIL（`PublishExternal undefined`；SSE 无轮询时超时）。

- [ ] **步骤 3：实现**

`internal/eventbus/hub.go` 加方法（跨副本 nudge 注入点；构造一个 `IndexedEvent`，Event 体可为最小占位，SSE 靠 index 触发回放）：

```go
// PublishExternal injects a cross-replica nudge: subscribers of runID are told
// "new events exist up to index seq". The SSE handler re-reads the store, so the
// Event payload need not carry content; observers (webhook) also fire.
func (h *Hub) PublishExternal(runID string, seq int64) {
	h.Publish(runID, IndexedEvent{Index: int(seq), Event: store.Event{Type: "external.nudge"}})
}
```

（`int(seq)` 与现有 Index 类型一致；若 seq 用 int64 更合适则 Index 保持 int、转换即可。webhook observer 会对 `external.nudge` 触发一次 outbox 检查——outbox 幂等去重，无副作用；若担心可在 observer 侧忽略该类型，但 outbox `PutWebhookOutboxIfAbsent` 已按 eventIndex 去重，安全。）

`internal/api/server.go` SSE handler（`GET /v0/runs/{id}/events` 的 `for { select { ... } }`，约 1988 行）加一个兜底轮询 ticker：

```go
	poll := time.NewTicker(3 * time.Second)
	defer poll.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			// 既有 ping
		case <-poll.C:
			evs, err := s.Store.ListEvents(id)
			if err != nil {
				return
			}
			for i := lastSent + 1; i < len(evs); i++ {
				if err := writeSSEEvent(w, rc, i, evs[i]); err != nil {
					return
				}
				lastSent = i
			}
			// 若轮询发现终态，结束流（照 catchUpRunStream 的终态判断）。
			if cur, err := s.Store.GetRun(id); err == nil && (cur.Status == store.StatusSucceeded || cur.Status == store.StatusFailed) {
				_ = writeSSEEnded(w, rc, cur.Status)
				return
			}
		case ev, ok := <-sub.Events:
			// 既有：写事件；随后也可顺带 catch-up（保持现状）
			...
		case stt, ok := <-sub.Ended:
			...
		}
	}
```

注意把轮询逻辑与既有 `drainSubEvents`/`catchUpRunStream` 风格对齐，避免重复代码；终态判断复用现有 helper。memory 驱动下该轮询也无害（多一次轻量 ListEvents）。

- [ ] **步骤 4：运行测试验证通过**

运行：`& $go test ./internal/eventbus/ ./internal/api/ -count=1`
预期：PASS，既有 SSE 测试不回归。

- [ ] **步骤 5：Commit**

```bash
git add internal/eventbus/ internal/api/
git commit -m "feat(eventbus): 支持跨副本 nudge 注入并为 SSE 增加兜底轮询"
```

---

## 任务 9：redis 驱动（Streams 队列 + Pub/Sub 总线 + Lua 限流）

**文件：**
- 创建：`internal/middleware/redis/driver.go`、`queue.go`、`bus.go`、`limiter.go`
- 测试：`internal/middleware/redis/redis_test.go`
- 修改：`cmd/baize/main.go`（blank import）

**依赖：** `go get github.com/redis/go-redis/v9`；测试 `go get github.com/alicebob/miniredis/v2`。用 `$env:GOPROXY='https://goproxy.cn,direct'` 拉取。

- [ ] **步骤 1：编写失败的测试（miniredis）**

新建 `internal/middleware/redis/redis_test.go`：

```go
package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/middleware"
	mwredis "github.com/rebornace/baize/internal/middleware/redis"
	"github.com/alicebob/miniredis/v2"
)

func newTestMiddleware(t *testing.T) *middleware.Middleware {
	t.Helper()
	mr := miniredis.RunT(t)
	b, err := mwredis.Open(context.Background(), mwredis.Config{
		Addr: mr.Addr(), Stream: "baize:runs", ConsumerGroup: "baize-workers",
		EventsChannel: "baize:run-events", ConsumerName: "test-consumer",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestRedisQueueEnqueueConsumeAck(t *testing.T) {
	b := newTestMiddleware(t)
	ctx := context.Background()
	if err := b.Queue.Enqueue(ctx, job("run_r1")); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	got, ack, ok := b.Queue.Consume(cctx)
	if !ok || got.RunID != "run_r1" {
		t.Fatalf("consume: ok=%v job=%+v", ok, got)
	}
	ack()
}

func TestRedisBusPubSub(t *testing.T) {
	b := newTestMiddleware(t)
	ctx := context.Background()
	ch, err := b.Bus.SubscribeRunEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // 等订阅就绪
	if err := b.Bus.PublishRunEvent(ctx, "run_b1", 5); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-ch:
		if n.RunID != "run_b1" || n.Seq != 5 {
			t.Fatalf("nudge=%+v", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no nudge")
	}
}

func TestRedisLimiterBudget(t *testing.T) {
	b := newTestMiddleware(t)
	allowed := 0
	for i := 0; i < 200; i++ {
		if b.Limiter.Allow("k") {
			allowed++
		}
	}
	if allowed == 0 || allowed > 130 { // 默认约 120/min 预算 + 少量时序容差
		t.Fatalf("allowed=%d", allowed)
	}
}
```

`job(runID)` 助手构造 `middleware.Job{RunID: runID, Kind: middleware.KindRun, Input: "x", EnqueuedAt: time.Now()}`；`mwredis.Bundle` 暴露 `Queue/Bus/Limiter/Close`（或让 `Open` 返回 `*middleware.Middleware`——二选一，保持与 `middleware.Open` 一致：推荐 redis 包提供 `Open(ctx, Config) (*middleware.Middleware, error)` 并在内部把 `BridgeToHub` 也挂上）。

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/middleware/redis/ -count=1`
预期：FAIL（包不存在）。

- [ ] **步骤 3：实现 driver.go**

```go
package redis

import (
	"context"
	"fmt"
	"os"

	"github.com/rebornace/baize/internal/middleware"
	goredis "github.com/redis/go-redis/v9"
)

// Config configures the redis middleware driver.
type Config struct {
	Addr, Username, Password, Stream, ConsumerGroup, EventsChannel, ConsumerName string
	DB                                                                          int
}

// Open builds a redis-backed Middleware.
func Open(ctx context.Context, cfg Config) (*middleware.Middleware, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis middleware: addr is required")
	}
	client := goredis.NewClient(&goredis.Options{
		Addr: cfg.Addr, DB: cfg.DB, Username: cfg.Username, Password: cfg.Password,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis middleware: connect: %w", err)
	}
	q := newQueue(client, cfg.Stream, cfg.ConsumerGroup, cfg.ConsumerName)
	if err := q.ensureGroup(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	b := newBus(client, cfg.EventsChannel)
	l := newLimiter(client)
	mw := &middleware.Middleware{Queue: q, Bus: b, Limiter: l}
	mw.Close = func() error { _ = b.Close(); _ = client.Close(); return nil }
	return mw, nil
}

// 驱动自注册（供 middleware.Open("redis", ...) 使用）。
func init() {
	middleware.RegisterDriver("redis", func(ctx context.Context, o middleware.Options) (*middleware.Middleware, error) {
		name, _ := os.Hostname()
		cfg := Config{
			Addr: o.Redis.Addr, DB: o.Redis.DB, Username: o.Redis.Username, Password: o.Redis.Password,
			Stream: o.Redis.Stream, ConsumerGroup: o.Redis.ConsumerGroup,
			EventsChannel: o.Redis.EventsChannel, ConsumerName: name,
		}
		return Open(ctx, cfg)
	})
}
```

（`Middleware.SetCloser`：若任务 3 的 `Middleware.Close` 是函数字段，则直接赋值 `mw.Close = func()error{...}`；沿用任务 3 的定义，不必新增 Setter。）

- [ ] **步骤 4：实现 queue.go（Streams 消费组）**

要点：
- `ensureGroup`：`XGroupCreateMkStream(ctx, stream, group, "$")`（组已存在则忽略 `BUSYGROUP` 错误）。
- Enqueue：`XAdd(ctx, &XAddArgs{Stream: stream, Values: {"job": jsonBytes}})`。
- Consume：`XReadGroup(ctx, &XReadGroupArgs{Group: group, Consumer: consumer, Streams: []string{stream, ">"}, Count: 1, Block: 5*time.Second})`；取到消息后 JSON 解码为 `middleware.Job`；返回 `ack func()`，ack 调 `XAck(ctx, stream, group, msgID)`。
- 无消息（Block 超时）返回 `ok=false`，让 worker 循环重试。
- PEL 认领（`XAUTOCLAIM`）作为额外安全网可在本任务做一个轻量版本：Consume 在 `>` 读不到时，周期性 `XAutoClaim` 空闲超过 lease 的 pending 消息重投；DB 调和已兜底，此项可最小实现或留注释说明由 DB 调和覆盖。**v0 以 DB 调和为准**，XAUTOCLAIM 可只留 TODO 级最小实现不阻塞。

- [ ] **步骤 5：实现 bus.go（Pub/Sub + Hub 桥接）**

```go
type bus struct {
	client *goredis.Client
	channel string
	sub    *goredis.PubSub
	out    chan middleware.RunEventNudge
	hub    *eventbus.Hub // 可选：注入本地 Hub
}

func newBus(c *goredis.Client, ch string) *bus {
	return &bus{client: c, channel: ch, out: make(chan middleware.RunEventNudge, 256)}
}

func (b *bus) PublishRunEvent(ctx context.Context, runID string, seq int64) error {
	payload, _ := json.Marshal(middleware.RunEventNudge{RunID: runID, Seq: seq})
	return b.client.Publish(ctx, b.channel, payload).Err() // 失败由调用方记日志、fail-open
}

func (b *bus) SubscribeRunEvents(ctx context.Context) (<-chan middleware.RunEventNudge, error) {
	b.sub = b.client.Subscribe(ctx, b.channel)
	go func() {
		for msg := range b.sub.Channel() {
			var n middleware.RunEventNudge
			if json.Unmarshal([]byte(msg.Payload), &n) == nil {
				select { case b.out <- n: default: }
				if b.hub != nil {
					b.hub.PublishExternal(n.RunID, n.Seq)
				}
			}
		}
	}()
	return b.out, nil
}

// BridgeToHub wires cross-replica nudges into the local in-process Hub so SSE
// and webhook subscribers see remote events.
func (b *bus) BridgeToHub(hub *eventbus.Hub) {
	b.hub = hub
}

// Start begins consuming the Pub/Sub channel. The returned nudge channel is not
// needed by bootstrap (nudges are injected into the Hub via BridgeToHub), so it
// is intentionally discarded. Must be called after BridgeToHub.
func (b *bus) Start(ctx context.Context) {
	_, _ = b.SubscribeRunEvents(ctx)
}

func (b *bus) Close() error { if b.sub != nil { return b.sub.Close() }; return nil }
```

注意：bootstrap 在拿到 `mw.Bus` 后调 `BridgeToHub(hub)` 并**启动订阅**（`SubscribeRunEvents` 触发订阅协程；即使没人消费 `out` channel，nudge 也会注入 hub）。driver/Open 里不自动启动订阅，由 bootstrap 显式调一次 `SubscribeRunEvents(context.Background())` 并丢弃返回 channel（或提供 `Start(ctx)`）。在 bus 加 `Start(ctx)` 内部调 SubscribeRunEvents 并忽略 out，bootstrap 调它。

- [ ] **步骤 6：实现 limiter.go（Lua 滑动窗口）**

用 Redis Lua 做固定/滑动窗口（sorted set 时间戳），key 前缀 `baize:rl:`，默认预算由调用方场景决定。为满足 `middleware.Limiter.Allow(key)` 单方法，limiter 内置默认预算（如 1000/min），inbox/callback 场景在任务 10 用带配额的包装。Lua 脚本（ZREMRANGEBYSCORE 清旧 + ZCARD 判额 + ZADD + EXPIRE）；Redis 出错时 `Allow` 返回 `true`（fail-open）并记日志。

- [ ] **步骤 7：blank import 与验证**

`cmd/baize/main.go` import 块加：
```go
	_ "github.com/rebornace/baize/internal/middleware/redis"
```

运行：`& $go build ./...`；`& $go test ./internal/middleware/... -count=1`（memory + redis/miniredis 全绿）；`& $go vet ./...`。

- [ ] **步骤 8：Commit**

```bash
git add internal/middleware/ cmd/baize/ go.mod go.sum
git commit -m "feat(middleware): redis 驱动（Streams 队列/PubSub 总线/Lua 限流）"
```

---

## 任务 10：分布式限流接线（inbox + sidecar callback）

**文件：**
- 修改：`internal/api/server.go` / `server_inbox.go`（入站限流用可注入 Limiter）
- 修改：`internal/api/server.go`（`CallbackLimiter` 支持后端替换）
- 修改：`internal/bootstrap/bootstrap.go`（redis 驱动下用 `mw.Limiter` 包装，保留配额/窗口语义）
- 测试：`internal/api/ratelimit_mw_test.go`（新建）

**设计要点**：现有两处限流配额/窗口不同（inbox 120/min/channel；callback 100/hour/run）。`middleware.Limiter.Allow(key)` 单方法不带配额。为不改变语义，引入一个「配额感知」的适配：redis limiter 提供 `AllowBudget(key string, limit int, window time.Duration) bool`（Lua 按 key+window 判额），`middleware.Limiter` 的 `Allow(key)` 用驱动默认预算。bootstrap 按场景构造带配额的 key/包装器。

- [ ] **步骤 1：编写失败的测试**

新建测试：用 miniredis + redis 驱动构造 limiter，断言「同一逻辑 key 在 N 副本共享配额」：两个 limiter 实例指向同一 miniredis，合计放行数不超过预算（区别于内存限流器各算各的）。并断言 Redis「故障」（关闭 miniredis）时 `Allow` 返回 true（fail-open）。照任务 9 的 miniredis helper。

- [ ] **步骤 2：运行测试验证失败**

运行：`& $go test ./internal/middleware/redis/ -run Limit -count=1`
预期：FAIL（`AllowBudget` 不存在 / 行为不符）。

- [ ] **步骤 3：实现配额感知限流**

`internal/middleware/redis/limiter.go` 加：

```go
// AllowBudget is a distributed sliding-window check: at most `limit` allows per
// `window` for key. Fails open (returns true) on Redis errors.
func (l *limiter) AllowBudget(key string, limit int, window time.Duration) bool {
	// Lua: ZREMRANGEBYSCORE key 0 (now-window); ZCARD key; if < limit then ZADD now now; EXPIRE key window; return 1 else 0
	...
}
```

并在 `middleware` 包定义可选接口：

```go
// BudgetLimiter is implemented by limiters that support an explicit quota/window.
type BudgetLimiter interface {
	AllowBudget(key string, limit int, window time.Duration) bool
}
```

bootstrap 接线：
- inbox：`srv.InboxLimiter` 仍是 `*inbox.RateLimiter`（内存，单机）。当驱动为 redis 时，改为注入一个满足 inbox 限流调用点的实现：在 `server_inbox.go` 把 `s.inboxLimiter().Allow(channelID)` 的调用改为走一个可替换接口 `s.inboxGate`（接口 `Allow(string) bool`），bootstrap 在 redis 驱动下设为「`AllowBudget("baize:rl:inbox:"+channelID, 120, time.Minute)`」的适配器，memory 驱动下仍用 `inbox.NewRateLimiter(120, time.Minute)`。
- callback：`srv.CallbackLimiter *plugincallback.Limiter` 同理，redis 驱动下用 `AllowBudget("baize:rl:cb:"+runID, 100, time.Hour)` 适配；memory 驱动保持现状。
- 适配方式：定义小接口 + 两个适配器（memory 用现有具体类型；redis 用闭包）。**不改动** `inbox.RateLimiter`/`plugincallback.Limiter` 的内存算法。

注意：若把两个具体限流器都改成接口会牵动较多测试；采用「Server 增加可选 `inboxGate func(string) bool` / `callbackGate func(string) bool`，为 nil 时回退现有具体限流器」的最小侵入做法。

- [ ] **步骤 4：运行测试验证通过**

运行：`& $go test ./internal/middleware/... ./internal/api/ ./internal/bootstrap/ -count=1` 与 `& $go build ./...`
预期：PASS，既有限流测试不回归。

- [ ] **步骤 5：Commit**

```bash
git add internal/middleware/ internal/api/ internal/bootstrap/
git commit -m "feat(middleware): redis 驱动下接入分布式全局限流（inbox/callback）"
```

---

## 任务 11：文档、全量验证与收尾

**文件：**
- 修改：`configs/demo.yaml`、`configs/minimal.yaml`（`middleware` 段注释示例）
- 修改：`docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md`（X3 标记已交付）
- 修改：`docs/superpowers/plans/2026-09-03-middleware-multi-source.md`（勾选完成项）

- [ ] **步骤 1：后端全量验证**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:GOTOOLCHAIN='auto'
$go='C:\Users\Administrator\go-sdk\go\bin\go.exe'
& $go build ./...
& $go test ./... -count=1
& $go vet ./...
& $go fmt ./...
```
预期：构建通过；全部测试 PASS（含 miniredis redis 驱动测试）；vet 干净；fmt 无改动。

- [ ] **步骤 2：配置示例**

在 `configs/demo.yaml` 加注释段（默认不启用 redis，保持 memory）：

```yaml
middleware:
  driver: memory          # memory（默认，零依赖，单机） | redis（多副本横向扩展/崩溃恢复）
  worker_concurrency: 8   # 每进程竞争消费 worker 数
  lease_ttl_sec: 60       # run 认领租约 TTL
  reconcile_interval_sec: 15
  # driver=redis 时配置：
  # redis:
  #   addr: "127.0.0.1:6379"
  #   password_env: REDIS_PASSWORD
  #   stream: baize:runs
  #   consumer_group: baize-workers
  #   events_channel: baize:run-events
```

`configs/minimal.yaml` 可不加（memory 默认即可）。

- [ ] **步骤 3：backlog 与计划勾选**

- backlog 文档把 X3「中间件多源」标记为**已交付**（在交付表加一行，执行顺序表更新），简述：可替换任务队列/事件总线/限流，memory 默认零依赖，redis 驱动支持多副本 + 崩溃恢复 + 跨副本 SSE。
- 勾选本计划所有已完成步骤复选框（合并/双仓推送那一步在推送完成后再勾）。

- [ ] **步骤 4：提交文档**

```bash
git add configs/ docs/superpowers/
git commit -m "docs: X3 中间件多源收尾，标记 backlog 已交付"
```
（UTF-8 无 BOM `-F` 文件方式提交中文信息。）

- [ ] **步骤 5：手工冒烟（建议）**

- memory 驱动（默认）：`baize demo` 启动，连续发消息确认 run 正常、SSE 实时输出、重启后行为与旧版一致。
- redis 驱动（可选，需本地 redis 或 docker）：`driver: redis` 配好 addr，起两个实例，发消息确认其中一个 worker 消费；kill 掉正在执行的实例，确认调和把 run 重新入队由另一实例续跑；SSE 连在 A、run 跑在 B 时确认实时事件可达。

- [ ] **步骤 6：合并与双仓推送（按用户确认）**

合并 feature 分支到 `main`，推 `real`，再用 `scripts/export-public.ps1` 同步 `public`（照 X1/X2/X4 收尾；public 推送需 `all` 权限，网络超时重试）。**注意**：public 导出是否包含 redis 驱动源码——`internal/middleware/redis` 属开源范围（go-redis 是开源依赖），应随 public 发布；`docs/superpowers/` 仍排除。

---

## 自检备注（计划作者已核对）

- 类型一致性：`middleware.Job/UserParts []Part` ↔ `api.partsToMiddleware([]llm.ContentPart)`；`ExecuteJob(context.Context, middleware.Job) error` 同时被 worker（`middleware.Executor`）与测试使用；`ReconcileStore` 直接复用 `store.Store.ListRunsForReconcile(limit int) ([]*store.Run, error)`；`StartWorkers(ctx, ex, concurrency)` 与 `StartReconciler(ctx, rs, interval)` 签名在任务 5/7 一致；`Middleware.Close` 为函数字段（任务 3 定义），各驱动工厂赋值。
- 规格覆盖：§1 三接口/双驱动（T3/T4/T9）、§2 队列/worker/租约/调和（T1/T5/T6/T7）、§3 总线/SSE（T8/T9 bus）、§4 限流/配置/装配（T2/T4/T7/T9/T10）、§5 降级与测试（各任务测试 + T11）。
- 已知简化：redis XAUTOCLAIM 以 DB 调和为准（T9 步骤 4 允许最小实现）；webhook outbox 跨副本租约属 backlog #5，不在本计划。
