# X3 中间件多源（可替换任务队列 / 事件总线 / 限流）设计规格

> 状态：已批准（2026-09-03，分节逐节确认）
> 里程碑：X3 v0
> 范围：把进程内的「任务调度 / 事件广播 / 限流」三类中间件抽象为可替换驱动；默认 `memory` 驱动保持单机零依赖行为，新增 `redis` 驱动支持多副本横向扩展、崩溃恢复、跨副本 SSE 与全局限流。

## 1. 目标、总体架构与组件边界

### 1.1 目标

- 企业可能已有成熟中间件（Redis / Kafka / RabbitMQ 等），希望直接接入自有配套系统，而非被绑死在项目内置实现。
- 参照既有两个多源样板：
  - **数据库**：`store.Store` 接口 + `store.RegisterDriver` 驱动注册表 + `store.driver` 配置选择（memory/sqlite/postgres），见 `internal/store/open.go`、`internal/store/registry.go`。
  - **大模型**：`llm.Provider` 接口 + `llm.Switch` 运行时按 profile 热切换。
- v0 交付：统一「中间件驱动」打包，内含三个小接口；`memory` 默认零依赖，`redis` 为首个外部后端。

### 1.2 非目标（YAGNI / 留独立 spec）

- Kafka / RabbitMQ / NATS / SQS 等其它 MQ 驱动（接口留好，后续按 registry 增加）。
- 对象存储 S3/OSS/MinIO（`artifact.Store` 接口已就绪，独立里程碑）。
- webhook outbox 的 `SKIP LOCKED` 多副本租约（backlog #5）；本切片多副本下 outbox 投递依赖唯一键幂等去重。
- 独立 worker 进程角色（`role=api/worker`）；本切片采用**同进程竞争消费**，不引入进程角色拆分。
- 限流配额管理 UI / 动态调参；可观测性看板（metrics/tracing），仅结构化日志。

### 1.3 三接口

新增 `internal/middleware/` 包。对外一个驱动、一份配置、一条 Redis 连接；对内三个边界清晰的小接口：

```go
// JobQueue 投递一个待执行 run，worker 竞争消费。
type JobQueue interface {
    Enqueue(ctx context.Context, job Job) error
    // Consume 取一个任务；处理完调用 ack。ok=false 表示暂无任务（超时返回）。
    Consume(ctx context.Context) (job Job, ack func(), ok bool)
    Close() error
}

// EventBus 通知本/他副本「某 run 有新事件」；只传轻量唤醒信号，不传事件内容。
type EventBus interface {
    PublishRunEvent(ctx context.Context, runID string, seq int64) error
    SubscribeRunEvents(ctx context.Context) (<-chan RunEventNudge, error)
    Close() error
}

// Limiter 与现有 inbox / plugincallback 限流器同形。
type Limiter interface {
    Allow(key string) bool
}
```

```go
type RunEventNudge struct {
    RunID string
    Seq   int64 // 生产副本已看到的最大事件序号，供订阅方判断是否需要回放
}
```

### 1.4 双驱动

- **memory（默认）**：
  - `JobQueue` = 带缓冲 channel + 一组 worker 协程（替代现在的裸 `go func`，但有界）。
  - `EventBus` = 包体现有 `eventbus.Hub`（进程内 fan-out），行为与现在一致。
  - `Limiter` = 现有内存滑动窗口算法。
  - 单机行为与今天等价，零外部依赖。
- **redis**：
  - `JobQueue` = Redis Streams 消费组（`XADD` / `XREADGROUP` / `XACK` + `XPENDING` / `XAUTOCLAIM`）。
  - `EventBus` = Pub/Sub 频道唤醒。
  - `Limiter` = Lua 原子滑动窗口。
  - 一条 `*redis.Client` 注入三者。

### 1.5 权威层定位

- run 的权威记录、事件、状态全部在 **DB（现有 store）**；Redis 仅作实时通道 / 触发。
- Redis 故障不丢任务：入队失败回退本地执行，DB 调和兜底。
- worker 执行 run **复用现有 `Server.runExecute` / `Engine.ExecuteWithOpts`**，不重写执行逻辑。
- 队列消息只放「runID + 重放所需轻量参数」。

---

## 2. 任务队列、worker 竞争消费与崩溃恢复

### 2.1 Job 负载

```go
type Job struct {
    RunID  string  `json:"run_id"`
    Kind   JobKind `json:"kind"` // 普通对话 run / regenerate / resume 等
    // 实时投递的重放参数（memory 直传；redis 序列化为 JSON）
    AgentID    string      `json:"agent_id"`
    Input      string      `json:"input"`
    Skills     []string    `json:"skills,omitempty"`
    UserParts  []llm.Part  `json:"user_parts,omitempty"` // 多模态负载，随消息携带
    EnqueuedAt time.Time   `json:"enqueued_at"`
}
```

**入队点（3 处 `go func` 改为 `Enqueue`）**：
- `internal/api/run_start.go:67`（发消息）
- `internal/api/server.go:2440`（regenerate / resume）
- `internal/bootstrap/bootstrap.go:424`（微信 AfterCreateRun）

Handler 现在只做 `CreateRun → Append 输入消息 → Enqueue`，不再裸起协程。

**多模态负载说明**：`UserParts`（图片）不落库。实时路径随 Job 消息携带。崩溃调和路径无法重建图片：这类 run 若租约过期，调和时**标记失败并记一条 system_note「多模态任务因进程中断无法重放，请重新发送」**，不静默用空输入重跑。纯文本 run 可从 DB（`runs.input` / agent_id）完整重建。

### 2.2 Worker（每进程一组，竞争消费）

- bootstrap 启动 N 个 worker 协程（配置 `middleware.worker_concurrency`，默认 8），循环 `Consume → 执行 → Ack`。
- memory 队列直接用 Job 参数调 `runExecute`；redis worker 从 Job 取 runID，**优先用消息内参数**，缺失则从 DB 重建文本 run。
- 成功 / 失败收尾沿用现在 `runExecute` 外层逻辑（失败置 `StatusFailed` + 记 system_note）。
- **幂等门禁**：执行前检查 run 状态，只有 `queued` / `running(租约过期)` 才执行；已 `succeeded/failed/cancelled/waiting_human` 直接 Ack 跳过。复用引擎 `ExecuteWithOpts` 已有的状态判断（`internal/run/engine.go:162`）。

### 2.3 租约与崩溃恢复（DB 调和兜底）

- `runs` 表新增可空列 `lease_until`（sqlite/postgres 迁移，方式同 X4 加列）。
- 新增 store 方法：
  - `LeaseRun(id string, ttl time.Duration) (acquired bool, err error)` —— 原子条件更新：`UPDATE runs SET lease_until=? WHERE id=? AND (lease_until IS NULL OR lease_until < now)`。
  - `HeartbeatRun(id string, ttl time.Duration) error` —— 执行中周期续租。
  - `ListRunsForReconcile(limit int) ([]Run, error)` —— 查 `status IN (queued,running) AND (lease_until IS NULL OR lease_until < now)`。
- worker 认领 run 时 `LeaseRun(ttl)`（默认 60s，长于单次工具超时上限）；执行中心跳续租；完成清除。
- **调和循环（reconciler）**：每进程一个协程，启动即跑一次 + 每 `reconcile_interval_sec`（默认 15s）跑一次：
  - `ListRunsForReconcile` 捞出孤儿 run，重新 `Enqueue`。
  - `queued` 从未认领 → 正常重放。
  - `running` 租约过期 → 判定 worker 崩溃 → 重新入队（新 worker 续跑；幂等门禁防重入）。
  - 多副本下 `LeaseRun` 原子更新保证只有一个副本抢到，避免重复入队。
- memory 驱动：租约用进程内 map；调和主要兜底「协程 panic」。
- redis 驱动：DB 租约为权威（所有副本共享同一 DB）；Redis Streams 的 PEL（pending entries list）+ `XAUTOCLAIM` 处理「消息被取走但 worker 崩溃未 Ack」，与 DB 调和双保险、DB 门禁去重。

### 2.4 Redis Streams 拓扑

- 一个 stream（默认 `baize:runs`）+ 一个消费组 `baize-workers`；每副本用唯一 consumer 名（`hostname-pid`）。
- `XADD` 入队；`XREADGROUP` 消费；处理完 `XACK`。
- 闲置 pending 消息（`XPENDING` + `XAUTOCLAIM`，超过 lease_ttl）由各副本定时任务认领重投，与 DB 调和一致，DB 门禁去重。

---

## 3. 跨副本事件总线与 SSE 实时推送

### 3.1 现状

`eventbus.Hub` 是进程内 fan-out：store 装饰器 `eventbus.Notify(st, hub)` 在每次 `AppendEvent` / 终态 `UpdateRun` 后调 `hub.Publish`；SSE handler（`internal/api/server.go:1965`）先 `ListEvents` 回放历史，再 `Subscribe(runID)` 收实时。问题：run 在 B 副本执行、事件写进共享 DB，但 A 副本 Hub 收不到通知，连在 A 的 SSE 客户端只能干等。

### 3.2 nudge 只传信号

事件总线只传轻量唤醒信号（runID + 最新 seq），**不传事件内容**——内容永远从 DB `ListEvents` 读取（权威、不丢）。

### 3.3 memory 驱动

`EventBus` 直接包体现有 `eventbus.Hub`，行为与现在完全一致（进程内 fan-out，SSE 订阅 channel），零改动路径。

### 3.4 redis 驱动

- **生产端**：store 装饰器在 `AppendEvent` 成功后，除本地 `hub.Publish`（通知本副本 SSE），再 `PUBLISH baize:run-events <json{runID,seq}>`。
- **消费端**：每副本启动**一个**订阅协程，`SUBSCRIBE baize:run-events`；收到 nudge 后调新增的 `hub.PublishExternal(runID, seq)` 注入本副本 Hub。
  - 这样 SSE handler、webhook dispatcher 等**现有 Hub 订阅者完全不用改**——它们只管从 Hub 收信号然后照旧 `ListEvents` 回放。跨副本事件被「注入」本地 Hub，与本地事件走同一条路。
- **去重 / 补缺**：SSE 本就是「回放 + 实时」模型，收到 nudge 重查 `ListEvents(afterSeq)`；重复 nudge 无害（序号没涨则无新内容）。

### 3.5 兜底（Pub/Sub 丢消息 / 订阅断连）

- Pub/Sub 发后即忘，断线重连期间可能漏 nudge。SSE handler 增加**低频兜底轮询**：每 2–3s `ListEvents(runID, afterSeq)` 一次，仅对**当前有活跃 SSE 连接的 run**（数量少、开销小）。
- memory 驱动行为不变；redis 驱动下作为漏通知安全网，并覆盖 SSE 连接建立时刻之外的时序缝隙。
- 订阅协程由 go-redis 自动重连，重连后靠兜底轮询补 gap。

### 3.6 webhook 等观察者

`webhook.Dispatcher.Attach(hub)` 的 `OnEvent/OnEnd` 观察者同理——跨副本 nudge 注入本地 Hub 后可能在多副本被唤醒，但出站投递由 **DB outbox 单写者 + 幂等 `PutWebhookOutboxIfAbsent`**（deliveryKey 唯一）去重，不会重复投递。outbox 跨副本 `SKIP LOCKED` 租约属 backlog #5，不在本切片；多副本下建议部署文档注明「出站 webhook 轮询可单副本承担」或接受唯一键幂等去重。

### 3.7 接口落点

`EventBus` 接口定义在 `internal/middleware`；`eventbus.Hub` 保留为 memory 实现与本地 fan-out 核心，新增 `PublishExternal`；redis 实现在 `internal/middleware/redis`。bootstrap 按驱动把 redis bus 的 nudge 接到 Hub。

---

## 4. 分布式限流、配置与驱动装配

### 4.1 限流抽象

现有两个限流器（`inbox.RateLimiter` 入站 webhook 滑动窗口、`plugincallback.Limiter` sidecar 回调固定窗口）都是 `Allow(key) bool` 形态内存 map。统一为 `middleware.Limiter`：

- **memory 驱动**：复用现有算法（进程内 map），行为不变；让现有限流器满足 `middleware.Limiter` 或由 memory 驱动提供等价实例。
- **redis 驱动**：Lua 脚本做原子滑动窗口（`INCR` + `EXPIRE` 或 sorted-set 时间戳），配额 / 窗口沿用现有配置（inbox 默认 120 req/min；callback 100/hour）。不同用途用不同 key 前缀（`baize:rl:inbox:<channelID>`、`baize:rl:cb:<runID>`）。
- **语义**：分布式下 N 副本共享同一配额（修复「N 副本 = N 倍配额」）。Redis 故障时 `Allow` **fail-open**（降级放行，记日志）——限流是保护措施，不应因 Redis 抖动拒掉正常请求。

### 4.2 配置

`internal/config/config.go` 新增 `middleware` 段：

```yaml
middleware:
  driver: memory              # memory（默认，零依赖） | redis
  worker_concurrency: 8       # 每进程竞争消费 worker 数，默认 8
  lease_ttl_sec: 60           # run 认领租约 TTL，默认 60
  reconcile_interval_sec: 15  # DB 调和周期，默认 15
  redis:
    addr: ""                  # host:port；空且 driver=redis 则启动报错
    db: 0
    username: ""
    password_env: ""          # 从环境变量读密码，不写明文
    stream: baize:runs
    consumer_group: baize-workers
    events_channel: baize:run-events
```

默认值在 `applyDefaults` 归一（driver 空→memory；concurrency≤0→8；ttl≤0→60；reconcile≤0→15）。`driver=redis` 但 `addr` 为空 → 启动即报错（fail fast，不静默退回 memory）。

### 4.3 驱动注册与装配（贴 store 样板）

- `internal/middleware/middleware.go`：三接口 + `Job` / `RunEventNudge` + `Middleware` 组合体（`Queue JobQueue`、`Bus EventBus`、`Limiter Limiter`、worker/reconciler 启停）。
- `internal/middleware/registry.go`：`DriverFactory func(opts Options) (*Middleware, error)`、`RegisterDriver(name, factory)`、`Open(driver, opts)`，memory 内建默认。
- `internal/middleware/memory/`：channel 队列 + 包 `eventbus.Hub` 的总线 + 内存限流，`init()` 自注册。
- `internal/middleware/redis/`：`go-redis` 客户端 + Streams/PubSub/Lua，`init()` 自注册。go-redis 依赖仅被 redis 子包引用（go.mod 新增 `github.com/redis/go-redis/v9`，显式可选驱动，与 pgx 同性质）。
- `cmd/baize/main.go`：blank-import `_ ".../middleware/redis"`（照 weixin channel 的 blank import 做法）；memory 始终内建。
- **bootstrap 接线**（`newAPIServer`）：`mw, err := middleware.Open(cfg.Middleware.Driver, opts)`；
  - 三个入队点从 `go func` 改为 `mw.Queue.Enqueue`；
  - 启动 worker 池（消费 → `runExecute`）与 reconciler（启动即跑 + 定时）；
  - redis bus 订阅协程把跨副本 nudge 注入 `eventbus.Hub`；
  - 两个限流器改用 `mw.Limiter`（按用途传不同 key 前缀 / 配额）。
  - memory 驱动：worker 池替代裸协程（有界、同进程），Hub / 限流原样——单机行为等价。

### 4.4 优雅退出

进程收到关闭信号：停止取新任务、等待在执行 run 到安全点（或租约自然过期被他副本接管）；redis 下 `XACK` / 关订阅；`Close()` 统一收口。

---

## 5. 错误处理、测试与范围边界

### 5.1 错误处理原则

中间件是增强项，绝不能让单机 / 零依赖路径变脆弱。

- **memory 驱动**：不依赖外部服务，行为等价今天（裸协程 → 有界 worker channel），无新增失败模式。
- **redis 驱动故障降级**：
  - `Enqueue` 失败（Redis 不可用）→ **回退本地执行**：记 error 事件后直接在本进程 `go runExecute`（等价旧行为），保证消息照发不阻塞；run 行已落库，DB 调和仍兜底。
  - 事件 `PUBLISH` 失败 → 仅影响跨副本实时性，本地 Hub 照常通知，DB 事件不丢；记日志，不报错。
  - 限流 Redis 失败 → **fail-open** 放行。
  - 订阅协程断线 → go-redis 自动重连；重连期间靠 SSE 2–3s 兜底轮询补 gap。
- **调和安全**：`LeaseRun` 原子条件更新保证多副本只有一个抢到；重复入队由 run 状态门禁去重（非 queued / 过期 running 直接 Ack 跳过）。
- **多模态重放**：租约过期且 Job 负载丢失（仅崩溃调和路径）→ 标记失败 + system_note 提示重发，不用空输入误跑。

### 5.2 测试策略（TDD，每步先红后绿）

1. **接口与 memory 驱动**：`JobQueue` 入队 / 消费 / Ack 顺序与并发竞争；worker 池执行 run 成功 / 失败收尾；`EventBus` 本地 nudge 到达 Hub；`Limiter` 窗口配额。
2. **租约与调和**：`LeaseRun` 原子抢领（两副本只有一成）；`ListRunsForReconcile` 只捞 queued / 过期 running、排除未过期与终态；reconciler 启动 + 定时把孤儿 run 重新入队；终态 run 不重复执行。
3. **入队点改造**：三个 `go func` 点改为 Enqueue 后，发消息 / regenerate / 微信三条路径端到端仍产出正确 run（用现有 api/channel 测试套）。
4. **redis 驱动**：用 `miniredis`（纯 Go 内存 Redis，`github.com/alicebob/miniredis/v2`，仅测试依赖）验证 Streams 消费组 `XADD/XREADGROUP/XACK`、Pub/Sub nudge、Lua 限流、`XAUTOCLAIM` 认领 pending；不依赖真实 Redis 进程。
5. **降级**：redis client 指向不可达地址时，Enqueue 回退本地执行、PUBLISH 失败不影响、限流 fail-open。
6. **SSE 跨副本**：模拟「事件写 DB 但本地 Hub 无 nudge」，注入外部 nudge（走 redis bus 注入路径），断言 SSE 回放出新事件；兜底轮询在无 nudge 时也能补上。
7. 全量 `go build ./... && go test ./... && go vet ./... && gofmt`；前端无改动（本切片纯后端 + 配置）。

### 5.3 范围边界

**做**：
- `middleware` 包（三接口 + registry + memory/redis 驱动）。
- 三个入队点改队列；worker 池 + DB 租约 / 调和。
- 跨副本事件 nudge + SSE 兜底轮询；分布式限流。
- `middleware` 配置段；`runs.lease_until` 迁移；miniredis 测试。

**不做**：见 §1.2（其它 MQ 驱动、对象存储、outbox SKIP LOCKED、独立 worker 角色、限流 UI、可观测性看板）。

### 5.4 风险与缓解

| 风险 | 缓解 |
|------|------|
| worker 重复执行 | 状态门禁 + 租约原子认领 + 调和去重，三重保障 |
| Redis 故障面扩大 | 全路径 fail-open / 本地回退，DB 权威兜底，memory 驱动零依赖 |
| 长 run 租约过期被他副本重复接 | 心跳续租（lease_ttl 60s > 单次工具超时，执行中周期续）；过期才判定崩溃 |
| 多副本 webhook 重复投递 | outbox 唯一 deliveryKey 幂等；完整租约留 backlog #5 |
