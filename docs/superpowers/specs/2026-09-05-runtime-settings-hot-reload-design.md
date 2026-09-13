# 运行时设置热更新（Runtime Settings Hot-Reload）v0 设计规格

> 日期：2026-09-05
> 状态：已交付（2026-09-13 账本对齐；API/UI 热更新已在 main；运行参数页人话化另计体验债）
> 依赖：DB 设置 KV（`store.GetSetting/UpsertSetting`，已交付）；原子热切换模式（同 `llm.Switch`）
> 归属里程碑：Agent 能力增强之后、F 生产硬化之前的体验增强

---

## 1. 目标、范围与整体架构

### 1.1 背景与问题

Baize 的「业务配置面」（模型 profile、工具目录、连接器、技能、webhook、渠道凭证、
会话身份）已全面 DB 化 + API 热改。但仍有一批设置**只能改 YAML/环境后重启进程**：

- 引擎调参项（历史窗口、最大工具步数、工具超时、压缩阈值等）——调参/排障时频繁要改，每次重启打断；
- 控制面操作员/管理员口令——轮换口令要重启，安全运维体验差；
- 微信渠道启停——`Channel.Start/Stop` 运行时方法已现成，但设置页改 `enabled` 不即时生效。

### 1.2 目标

让这批设置可经 **UI/API 即时修改、不重启生效**，并**跨重启保留、跨副本一致**。
复用已有 DB 设置 KV 与原子快照热切换模式（与 `llm.Switch` 同构）。

### 1.3 V0 范围（三项）

1. **引擎参数**：历史窗口 `max_messages`、最大工具步数 `max_steps`、工具超时
   `tool_timeout_seconds`、压缩开关 `compaction_enabled`、压缩阈值 `compact_threshold`、
   预留 token `compact_reserve_tokens`、保留近期消息 `compact_keep_recent`。
2. **控制面凭据**：轮换 `operator_token` / `admin_token`，以及具名 `operators`
   列表（id + token）的增/删；支持清空回退 YAML。
3. **微信渠道启停**：`enabled` 热应用——开启即时 `Start`、关闭即时 `Stop`。

### 1.4 非目标（明确不做）

- 存储/中间件/数据库**驱动**的热切换、S3/Redis 连接参数热重连（走现有 overlay+重启）；
- 监听端口/TLS、`data_dir` 等目录路径、demo/start 运行模式热改；
- 微信**白名单入站强制**（现状白名单只存不生效；作为独立安全特性另开）；
- 配置文件 SIGHUP/watch 整体热重载；
- 凭据 KV 加密（V0 明文落库，与模型 profile api_key / webhook secret 同级）。

### 1.5 整体架构

新增包 `internal/runtimecfg`，核心是一个**原子快照持有者**：

```
config.Config (YAML 默认)
      │ 启动
      ▼
runtimecfg.Holder ── atomic.Pointer[Snapshot]（读路径无锁）
   ▲    │  ▲
   │    │  └─ TTL（20s）后台从 DB KV 重读 → 跨副本一致
   │    └─ Update(patch)：校验 → 合并 → 写 store.UpsertSetting → 原子换快照
   │
   └─ 各消费点注入窄读取接口：
        ├─ run.Engine     读 Knobs（每次 run 开始读一次，run 内一致）
        ├─ run.Compactor  读 Knobs（MaybeCompact 内读，避免并发改共享字段）
        ├─ api.Server     gateTokens() 读 Credentials
        └─ （微信 Enabled 沿用 settings.json + 补 Start/Stop，不入快照）
```

- **`Snapshot`**（不可变）：

  ```go
  type Snapshot struct {
      Knobs Knobs
      Creds Credentials
  }
  type Knobs struct {
      MaxMessages          int
      MaxSteps             int
      ToolTimeout          time.Duration
      CompactionEnabled    bool
      CompactThreshold     float64
      CompactReserveTokens int
      CompactKeepRecent    int
  }
  type Credentials struct {
      OperatorToken string
      AdminToken    string
      Operators     []controlplane.Operator // 有效集 = config 基线 + 运行时新增
  }
  ```

- **持久化**：单个 KV 键 `runtime_settings`（JSON）。只存**显式覆盖**；缺省/空字段
  表示「回退 YAML 默认」。凭据 token 明文落库（本地 SQLite/PG）；GET 出口**一律脱敏**。
- **启动**：bootstrap 用 YAML 值构造基线 Snapshot，再读 KV 覆盖；KV JSON 损坏 →
  记警告、回退 YAML，不阻断启动。
- **多副本**：Holder 起 TTL 刷新 goroutine（仅 SQL 后端；memory 单副本可不刷），
  重读 KV 换快照；PATCH 本副本立即换（无 TTL 延迟）；刷新失败保留上一份好快照。
- **nil 安全**：消费点持有 `*runtimecfg.Holder`（或窄接口），为 nil（旧测试/未装配）
  时全部回退到原字段/YAML 默认，零行为变化、旧测试零改动。

---

## 2. 引擎参数热更新

### 2.1 读取点改造（`internal/run`）

`Engine` 现有字段 `MaxSteps`、`ToolTimeout`、`MaxMessages` 与 `Compactor.*` 保留为
**启动默认/YAML 值**；新增可选读取器：

```go
// internal/run
type KnobReader interface { Knobs() runtimecfg.Knobs }

// Engine 新增字段
Settings KnobReader // 可选；nil = 用现有字段（YAML 默认）
```

`currentKnobs()` 合并「快照覆盖 > engine 字段 > 代码默认」，逐字段取值：

| 读取点 | 现状 | 改后 |
|---|---|---|
| 工具超时 `toolTimeout()`（engine.go ~218） | 读 `e.ToolTimeout` | 快照 `ToolTimeout>0` → 快照；否则字段；再否则 `DefaultToolTimeout`(60s) |
| 最大步数 `runLoop`（engine.go ~533） | `e.MaxSteps` | 快照 `MaxSteps>0` → 快照；否则字段；再否则 16 |
| 历史窗口 `buildMessages`（engine.go ~243） | `ListWindow(conv, e.MaxMessages)` | 快照 `MaxMessages>0` → 快照；否则字段；再否则 40 |
| 压缩开关/参数 | `Compactor` 字段直读 | 见下 |

**run 内一致性**：每次 `ExecuteWithOpts` 开始读一次快照（`knobs := e.currentKnobs()`），
该 run 内工具超时/步数/窗口都用这份；避免 run 中途参数跳动。

**压缩**：`Compactor` 也持有同一 `KnobReader`（字段 `Settings KnobReader`）。
`MaybeCompact` 开头求值一次有效参数：
- `knobs.CompactionEnabled == false` → 直接返回 `changed=false`（该 run 跳过压缩，等价 compactor=nil）；
- 否则用快照的 `Threshold/ReserveTokens/KeepRecent`（>0/在合理范围才覆盖），仍走
  `normalize()` 的下限保护；快照未覆盖则用 Compactor 结构体字段（YAML 值）。
- 在 `MaybeCompact` 内读快照而非改共享字段，避免多 run 并发的数据竞争。

### 2.2 校验规则（PATCH 时，字段级）

| 字段 | 规则 |
|---|---|
| `max_messages` | 可选；0/缺省=用默认；若给则 1–500 |
| `max_steps` | 可选；若给则 1–100 |
| `tool_timeout_seconds` | 可选；若给则 1–600 |
| `compaction_enabled` | 布尔；缺省=不改变 |
| `compact_threshold` | 可选；若给则 0.1–0.95 |
| `compact_reserve_tokens` | 可选；若给则 256–100000 |
| `compact_keep_recent` | 可选；若给则 0–100 |

非法 → 400 + 字段级错误。**部分更新**语义：只传要改的字段，其余保持。

### 2.3 默认值与来源链

启动 Knobs 由 `config.Config` 填充：`Run.MaxSteps`、`Run.ToolTimeoutSec`、
`Conversation.MaxMessages`、`Conversation.CompactEnabled`（`*bool`，nil=配了 compactor 即 true）、
`Conversation.CompactThreshold`、`Conversation.CompactReserveOutput`、
`Conversation.CompactRecentMessages`。

取值优先级：**KV 覆盖 > YAML/config > 代码默认**。GET 响应回 `effective` 生效值与
`overridden` 布尔（该字段是否被 KV 覆盖），便于 UI 显示「当前生效 / 是否自定义」。

---

## 3. 控制面凭据热更新

### 3.1 读取点改造（`internal/api`）

`gateTokens()`（server.go ~223）从「读 `s.OperatorToken/AdminToken/Operators` 字段」
改为「读快照凭据」：

```go
type CredReader interface { Credentials() runtimecfg.Credentials }

// Server 新增字段
Settings CredReader // 可选；nil = 用现有字段（旧测试零改动）

func (s *Server) gateTokens() controlplane.Tokens {
    if s.Settings != nil {
        c := s.Settings.Credentials()
        return controlplane.Tokens{
            Operator:  c.OperatorToken,
            Admin:     c.AdminToken,
            Operators: c.Operators,
        }
    }
    return controlplane.Tokens{
        Operator:  s.OperatorToken,
        Admin:     s.AdminToken,
        Operators: s.Operators,
    }
}
```

换快照是原子的，下一个请求即刻用新口令，无需重启。

### 3.2 凭据快照如何构成（break-glass 合并语义）

Holder 内部区分两层：

- **config 基线**（启动时由 `controlplane.ResolveSecret` 解析 `cfg.ControlPlane.*`）：
  `baseOperator`、`baseAdmin`、`baseOperators []controlplane.Operator`。这层永不在运行时改，
  是永久 break-glass。
- **运行时覆盖**（来自 KV）：`overrideOperator`、`overrideAdmin`（空串=不覆盖该槽位）、
  `runtimeOperators []controlplane.Operator`（运行时新增）。

`Credentials()` 求值：
- `OperatorToken = overrideOperator != "" ? overrideOperator : baseOperator`
- `AdminToken    = overrideAdmin    != "" ? overrideAdmin    : baseAdmin`
- `Operators     = baseOperators + runtimeOperators`（追加；运行时删除只能移除 `runtimeOperators` 中的）

**重置**：清空覆盖（override 置空、runtimeOperators 清空）→ 立即回到纯 config 口令。

### 3.3 PATCH 接口（`PATCH /v0/settings/credentials`，仅 admin）

部分更新、字段均可省略：

| 字段 | 含义 |
|---|---|
| `operator_token` | 非空则轮换 operator 主口令（覆盖槽位） |
| `admin_token` | 非空则轮换 admin 主口令 |
| `add_operators` | `[{id, token}]`；id 在有效集（base+runtime）已存在 → 409 |
| `remove_operators` | `[id]`；仅能移除 runtime 中的 id；id 属于 config 基线 → 400（不可删） |
| `reset` | true = 清空全部覆盖回 config（与其它字段互斥，同现 → 400） |

校验：id/token 非空、id 唯一。**防锁死**：拒绝「config 有口令、覆盖后三个有效槽位全空」
的改法（此类请求 400），避免误操作把鉴权关掉。

### 3.4 安全、脱敏与锁死恢复

- **GET 绝不回传 token**。`GET /v0/settings/credentials`（admin）只返回：

  ```json
  {
    "source": "override",
    "operator_set": true,
    "admin_set": true,
    "operators": [{"id": "alice", "source": "config"}, {"id": "bob", "source": "runtime"}]
  }
  ```

  `source`：有任何覆盖为 `"override"`，否则 `"config"`。
- 凭据 KV 明文落库（与模型 profile api_key、webhook secret 同级，本地 SQLite/PG）。
- **锁死恢复**：轮换后若丢失新 admin 口令，清除该 KV 覆盖即恢复 YAML break-glass——
  提供本地 CLI 子命令 `baize settings reset-credentials`（直接删除 KV 中凭据覆盖段），
  并在 README/规格写明。CLI 不走 HTTP 门禁，本地执行。
- 写凭据的 PATCH 本身要求当前请求已是 admin（现有门禁已区分 operator/admin 角色）。

---

## 4. 微信启停热应用、启动装配、API 与测试

### 4.1 微信 Enabled 热应用（`internal/api/server_channel_weixin.go`）

`handlePutWeixinSettings` 已存 `settings.json`；把 `applyWeixinSettings` 从「只同步
Assignee/AgentID」扩展为**按 `Enabled` 调和实际运行态**（仍在 `weixinMu` 锁内）：

- `Enabled=true`：`WeixinChannel != nil` 且**未启动**且**已有凭证** → `Start(s.weixinRunCtx())`；
  无凭证 → 保持停止（响应 `running:false, reason:"login_required"`；登录成功后现有
  `applyWeixinLoginSuccess` 在 `settings.Enabled` 为真时自动 Start）。
- `Enabled=false`：`IsStarted()` → `Stop(ctx)`（**只停轮询、不清凭证**，区别于 logout；
  重新启用可直接 Start）。
- Assignee/AgentID 维持现状。
- 响应回传 `{saved:true, running:<IsStarted()>}`，UI 可区分「已启用但未登录」。
- bootstrap 启动时已有 `if credErr==nil && settings.Enabled { Start }`（bootstrap.go ~614），
  语义不变；热改补齐运行时这条路径。

### 4.2 启动装配（`internal/bootstrap`）

- store 就绪后构造 `runtimecfg.Holder`：
  1. 用 `config.Config` 建基线 Snapshot——Knobs 取自 `cfg.Run.*` / `cfg.Conversation.*`；
     Creds 由 `controlplane.ResolveSecret(cfg.ControlPlane.OperatorToken/AdminToken)` 与
     `cfg.ControlPlane.Operators`（逐条 ResolveSecret）解析。
  2. 读 KV `runtime_settings` 覆盖；**KV JSON 损坏 → log 警告、回退 YAML，不阻断启动**。
- 注入：`engine.Settings = holder`、`compactor.Settings = holder`、`srv.Settings = holder`。
- `holder.StartRefresh(ctx, store, 20*time.Second)`：SQL 后端起 TTL 重读 goroutine（跨副本）；
  PATCH 本副本原子换快照（无延迟）；刷新失败保留上一份好快照。进程关停随 ctx 结束。

### 4.3 API 与权限（复用现有角色门禁）

| 方法/路径 | 权限 | 说明 |
|---|---|---|
| `GET /v0/settings/runtime` | operator+ | 引擎旋钮 `effective` 值 + `overridden` 标记 |
| `PATCH /v0/settings/runtime` | **admin** | 部分更新 knobs，字段级校验（§2.2） |
| `GET /v0/settings/credentials` | **admin** | 仅脱敏：source / 各槽位是否设置 / operators 的 id+source（§3.4） |
| `PATCH /v0/settings/credentials` | **admin** | 轮换/增删/reset（§3.3） |

微信设置沿用现有 `GET/PUT /v0/settings/channels/weixin`（已注册于 `server.go:390-391`，不并入新端点）。

### 4.4 错误处理

- 校验失败 → 400 + 字段级消息；operator id 冲突 → 409；非 admin 写 → 403；
  KV 写失败 → 500 且**不换快照**（先写库成功才换内存，保证内存≤库）。
- holder 为 nil（旧测试/未装配）→ 全部回退原字段/YAML，零行为变化。
- TTL 刷新遇到损坏/读错 → 保留当前快照，记日志。

### 4.5 测试

- **runtimecfg**：快照合并与默认回退；KV 往返（覆盖/缺省）；Update 字段级校验；
  TTL 刷新换快照；损坏 JSON 回退基线；nil 安全；凭据 break-glass 合并（config 基线 +
  runtime 追加、override 空串回退、reset、config operator 不可删）。
- **run**：假 `KnobReader` 下 max_steps / tool_timeout / max_messages 热改即时生效；
  `compaction_enabled=false` 跳过压缩；compactor 阈值/保留数被快照覆盖；run 内一致。
- **api**：凭据覆盖后 `gateTokens()` 即刻用新口令、reset 回退；GET credentials 响应
  **不含任何 token 字段**；非 admin PATCH=403；knob 非法值=400、部分更新保留其余字段；
  operator id 冲突=409、删 config operator=400。
- **微信**：PUT `enabled:false` → Stop（不清凭证）；`enabled:true` 有凭证 → Start；
  无凭证 → `running:false` 且不报错。
- **bootstrap（冒烟）**：holder 被装配到 engine/compactor/server；KV 覆盖在启动时加载。

### 4.6 范围边界

不做：驱动热切换 / S3·Redis 热重连、端口/TLS/目录路径、微信白名单入站强制、
SIGHUP 整体热重载、凭据 KV 加密。这些进入 backlog 作为后续候选（白名单强制为安全特性优先项）。

