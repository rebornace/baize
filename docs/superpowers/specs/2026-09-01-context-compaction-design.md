# 对话上下文压缩 / 滚动摘要（X4）设计规格

> 日期：2026-09-01
> 依据：头脑风暴——长对话不爆模型上下文、显著省 token；原始历史全保留，滚动摘要持久化、可重算
> 路线：本里程碑（X4）→（排队）X3 中间件多源 / F 生产硬化
> 前置依赖：X2 多模型 profile（`llm.Switch`、`llm.ProfileSource`、`store.ModelProfile`）

---

## 1. 目标与成功标准

长对话随消息增长会超出模型上下文窗口或浪费 token。现状是硬滑动窗口：`Engine.buildMessages` 经 `conversation.Store.ListWindow(convID, MaxMessages)`（默认 40 条）直接丢弃更早消息，丢失关键信息。

X4 引入**滚动摘要压缩**：当组装 prompt 的估算 token 超过「该模型上下文长度 × 阈值」时，把较旧历史增量摘要成一段滚动摘要，prompt 变为 `system + 滚动摘要 + 近期原文窗口 + 当前输入`。原始消息一条不删。

成功标准：

1. 长对话触发压缩后，发给 LLM 的 token 显著下降，旧信息以摘要形式保留。
2. 压缩由**模型上下文长度 + token 估算**触发（非纯条数），不同模型 profile 自适应。
3. 摘要**持久化到会话**，重开、后续消息复用；可丢弃重算。
4. 模型 profile 可在设置页配置「上下文长度（tokens）」。
5. 摘要使用**默认模型 profile** 生成。
6. 压缩失败/未配置时**静默回退**现有硬窗口行为，绝不阻断发消息。
7. 原始消息、UI 聊天记录、Fork / 回滚 / 重生成能力不受影响。
8. 微信 / Inbox / MCP 等无人值守入口自动获得压缩。

### v0 边界（不做，记 backlog）

- 单次 run 内多步工具调用中途不压缩（run 内工具链是临时内存增长；跨 run 的持久化历史才是主因）。
- 不做精确 tokenizer（不引入 tiktoken 等重依赖）；用轻量字符估算。
- 不做独立「摘要模型」配置（统一用默认 profile）。
- 不做向量 / 语义检索式记忆。
- 全局压缩阈值等只走 YAML 配置，不进设置页；聊天气泡不显示「已压缩」提示。

---

## 2. 数据模型

在会话存储中新增一张派生表 `conversation_summaries`（与 `messages` / `conversation_meta` 同库，三驱动：Memory + SQLite + Postgres）。原始 `messages` 表不动一行。

```go
// internal/conversation/summary_rolling.go
type RollingSummary struct {
    ConversationID         string    `json:"conversation_id"`
    Summary                string    `json:"summary"`                   // 当前累积的滚动摘要文本
    CoversThroughMessageID string    `json:"covers_through_message_id"` // 游标：已摘要到哪条消息
    CoversThroughOrder     int       `json:"covers_through_order"`      // 该消息在会话中的序号（0-based）
    UpdatedAt              time.Time `json:"updated_at"`
}
```

- 每个会话至多一条滚动摘要（以 `conversation_id` 为主键）。
- `CoversThroughOrder` 用于稳定地取「游标之后」的消息（按 `List` 返回的时间序索引）。
- 摘要为纯派生数据，可随时删除重建。

`conversation.Store` 接口新增：

```go
GetRollingSummary(conversationID string) (RollingSummary, bool)
UpsertRollingSummary(s RollingSummary) error
ClearRollingSummary(conversationID string)
```

（Memory + SQLite 都实现；`bool` 表示是否存在，避免用错误表达 not-found。）

**联动**：

- `Clear(conversationID)`（清空会话）：一并 `ClearRollingSummary`。
- `TruncateFrom(conversationID, messageID)`（回滚 / 重生成截断）：截断成功后 `ClearRollingSummary`，下次从干净状态重算（游标可能指向已删消息）。
- `Fork(src, throughMsgID)`：产生新会话 ID，新会话没有摘要记录 → 天然从无摘要重新累积，不复制摘要。

**建表**：

- SQLite：加入 `sqliteMessagesSchema`：
  ```sql
  CREATE TABLE IF NOT EXISTS conversation_summaries (
    conversation_id TEXT PRIMARY KEY,
    summary TEXT NOT NULL,
    covers_through_message_id TEXT NOT NULL,
    covers_through_order INTEGER NOT NULL,
    updated_at TEXT NOT NULL
  );
  ```
- Postgres：跟随现有 messages / conversation_meta 的归属方式建等价表（BOOLEAN 不涉及；`covers_through_order` 为 INTEGER/BIGINT）。

---

## 3. Token 估算

新增轻量估算（偏保守，宁可稍早压缩也不溢出），纯函数、永不返回 error：

```go
// internal/run/tokens.go（或 internal/llm/tokens.go）
func EstimateTokens(text string) int
func EstimateMessagesTokens(msgs []llm.Message) int
```

口径：

- CJK 字符（Unicode 中日韩区间）按约 **1 token/字**。
- ASCII / 其他按约 **1 token / 4 字符**（`len/4`，按字节近似即可）。
- 每条消息加 **4 token** 结构开销（role 等）。
- 工具规格：按 `tool spec 的 name + description` 字符 / 4 粗估。

总候选 token：

```
total = EstimateMessagesTokens(system + 摘要注记 + 窗口历史 + 当前输入)
      + EstimateToolsTokens(toolSpecs)
      + ReserveOutput
```

---

## 4. Compactor 组件

新建 `internal/run/compact.go`：

```go
type Compactor struct {
    Messages conversation.Store
    LLM      llm.Provider      // 用「不带 profile id」的 ctx 调用 → llm.Switch 解析到默认 profile
    Profiles llm.ProfileSource // 取 ContextTokens（run 有 profile id 用该 profile，否则默认）
    Cfg      CompactConfig
}

type CompactConfig struct {
    Enabled        bool
    Threshold      float64 // 0.8
    ReserveOutput  int     // 8000
    RecentMessages int     // 8
    DefaultContextTokens int // 128000
}
```

`MaybeCompact` 签名（示意）：

```go
func (c *Compactor) MaybeCompact(ctx context.Context, convID string, msgs []llm.Message, toolSpecs []llm.ToolSpec, profileID string) []llm.Message
```

- 输入 `msgs` 为 `buildMessages` 的产物（system + 窗口历史 + 当前输入）。
- 返回应发给 LLM 的消息（可能被压缩重组）。
- 任何失败都返回原始 `msgs`（回退）。
- 游标与顺序以 `Messages.List(convID)` 的**全量持久化历史**（时间序）为准计算 `CoversThroughOrder`；`msgs` 仅提供 system、窗口内容与当前输入。折叠对象是「全量历史中、游标之后、且不在近期保留窗口内」的消息（窗口外更早的消息已被硬窗口丢弃，不在本次考虑）。

**算法（增量滚动）**：

1. 若 `!Enabled` 或 `convID == ""` 或无 Messages store → 原样返回。
2. 解析上下文长度：`profileID != ""` 时 `Profiles.ModelProfileByID(profileID)` 取 `ContextTokens`，失败/空则 `Profiles.DefaultModelProfile()`；`ContextTokens <= 0` → `DefaultContextTokens`(128000)。
3. 估算当前候选 token；未超 `ContextTokens × Threshold` → 原样返回（不调用 LLM）。
4. 超限 → 读 `GetRollingSummary(convID)`：
   - 近期原文保留：最近 `RecentMessages` 条**持久化历史**（不含 system 与当前输入）。
   - 待折叠：游标之后、近期窗口之前的旧消息（首轮即较早的历史）。
   - 调默认模型（ctx **不**带 profile id）：把「既有滚动摘要（如有）+ 待折叠旧消息」发给模型，指令为合并为一份简洁滚动摘要，保留关键事实、决策、ID、工具结果要点。
   - 摘要为空/异常短 → 视为失败，回退、不更新游标。
   - 持久化 `UpsertRollingSummary`（推进 `CoversThroughMessageID/Order` 到最后一条已折叠消息）。
5. 重组：`system + [摘要注记（摘要非空时）] + 游标之后近期原文 + 当前输入`。
6. 再估算；仍超阈值则再折叠一轮，**上限 3 轮**；近期窗口为底线（不再折叠，交给模型处理，与现状一致）。

**摘要注记形态**（system 之后的一条 system 消息）：

```
以下是较早对话的滚动摘要（供参考，不要向用户提及这是摘要）：
<summary text>
```

近期原文的 role 映射与 `buildMessages` 一致（user→user；assistant/system_note→assistant）。

---

## 5. prompt 组装与数据流

一次发消息：

1. `handlePostRun` 创建 run（可能带 `model_profile_id`，X2）→ 引擎 `Execute`。
2. `Execute` 照旧 `buildMessages` 得到 `system + 窗口历史 + 当前输入`。
3. **新增**：`Compactor.MaybeCompact(ctx, convID, msgs, specs, run.ModelProfileID)`，得到（可能压缩过的）messages。
4. 进入 `runLoop`，后续不变；run 内多步累积逻辑不变。
5. assistant 回复照常 `Append` 进 messages 表（下一次发消息才纳入压缩考虑）。

集成点：`Execute` 在 `buildMessages` 之后、`runLoop` 之前**同步内联**调用一次（只给该条消息增加一次性延迟；实现确定、摘要立即可用）。`Engine` 持有一个 `Compactor`（可为 nil → 不压缩，保持测试与 demo 简单）。

无人值守入口（微信 / Inbox / MCP export）同样经 `Execute`，自动获得压缩；摘要用默认模型。

UI 聊天记录读完整 `messages` 表，摘要不混入消息流 → 界面无断层。

---

## 6. 模型 profile 字段与配置

### 6.1 profile 新增 `context_tokens`

- `store.ModelProfile` 加 `ContextTokens int \`json:"context_tokens"\``。
- `model_profiles` 表加列 `context_tokens`（sqlite/postgres 迁移，方式同 X2 加列；缺省归一为 128000）。
- `llm.ModelProfileView` 加 `ContextTokens int`；`StoreProfileSource.toView` 映射。
- 管理 API payload 加 `context_tokens`（PATCH 指针字段 `*int`，部分更新；0/缺省由后端归一为默认，不允许存 0）。
- 种子 profile（bootstrap）`ContextTokens = 128000`。
- 模型设置页表单加「上下文长度（tokens）」数字输入，默认 128000；列表可展示。

### 6.2 全局压缩配置（YAML `conversation` 段）

```yaml
conversation:
  max_messages: 40              # 既有硬窗口（兜底）
  compact_enabled: true         # 新增，默认 true
  compact_threshold: 0.8        # 新增，占上下文长度比例；<=0 或 >=1 归一 0.8
  compact_reserve_output: 8000  # 新增，预留回复 token；<=0 归一 8000
  compact_recent_messages: 8    # 新增，压缩时保留近期原文条数；<=0 归一 8
```

`compact_enabled=false` 完全退回现有硬窗口行为。这些走 config（与 `max_messages` 一致），v0 不进设置页。

---

## 7. 错误处理

- 摘要 LLM 调用失败 / 超时 / 无 profile / 上下文长度未知 → **不阻断**：回退原始硬窗口 prompt 照常发消息；记录一条可观测事件或日志。
- 摘要结果为空或异常短 → 视为失败，回退、不更新游标（避免空洞摘要）。
- token 估算纯函数，永不返回 error（空输入 → 0）。
- 折叠 3 轮仍超阈值 → 停止折叠、用当前结果发送（近期窗口为底线），由模型侧处理超长。
- 并发：Compactor 对摘要 Upsert 为「读-改-写」；SQLite 经 MaxOpenConns(1) 串行化，Memory 加锁；v0 接受最后写入胜出（摘要可重算、幂等）。
- `ContextTokens <= 0` → 归一 128000；`compact_threshold` 越界 → 归一 0.8。

### 可观测事件

新增 run 事件类型 `EventContextCompacted`，压缩发生时追加（含折叠条数、估算 token、轮次；失败时含 error 字段）。不改变消息流，仅供时间线 / 日志日后使用。

---

## 8. 测试（TDD）

1. **Token 估算**：CJK/ASCII 混合、空输入、工具规格；断言量级与保守方向（CJK 比 ASCII 每字符 token 多）。
2. **滚动摘要存储**（Memory + SQLite）：Upsert/Get/不存在、游标字段往返；`Clear` / `TruncateFrom` 联动清摘要；`Fork` 新会话无摘要。
3. **Compactor**：
   - 未超阈值 → 不调用 LLM、原样返回（fake LLM 断言未被调用）。
   - 超限 → 调默认模型（ctx 无 profile id）生成摘要、推进游标、重组 prompt = system + 摘要注记 + 近期原文 + 输入；旧消息不以原文出现、近期消息保留。
   - 已有摘要 → 增量折叠（fake LLM 断言输入含旧摘要）。
   - 摘要 LLM 报错 / 返回空 → 回退原窗口、不更新摘要、run 不失败。
   - 3 轮上限 / 近期窗口底线。
   - run 指定 profile id → 用该 profile 的 ContextTokens 判阈值，但摘要调用走默认模型（ctx 无 id）。
4. **引擎集成**：`Execute` 在 buildMessages 后调用 Compactor（stub Compactor 或 fake LLM 验证压缩后 messages 传入 Chat）；Compactor 为 nil 时行为与现状一致。
5. **profile 字段**：store 往返 `context_tokens`、API PATCH 部分更新、种子默认 128000。
6. **前端**：模型设置表单 context_tokens 渲染与提交（沿用 X2 测试模式）。
7. **全量**：`go build ./...`、`go vet ./...`、`go test ./... -count=1`、`gofmt -l internal cmd`；前端 `npx vitest run` + `npm run build`。

---

## 9. 文件结构（预期）

- 新建 `internal/conversation/summary_rolling.go` — `RollingSummary` 类型 + Memory 实现。
- 修改 `internal/conversation/store.go` — 接口 3 方法 + Memory；Clear 联动。
- 修改 `internal/conversation/sqlite.go` / `sql_dialect.go` — 建表 + SQL CRUD + TruncateFrom 联动。
- 新建 `internal/run/tokens.go` + 测试 — token 估算。
- 新建 `internal/run/compact.go` + 测试 — Compactor。
- 修改 `internal/run/engine.go` — `Engine.Compactor` 字段 + `Execute` 调用。
- 修改 `internal/store/store.go` / `model_profiles.go` / `model_profiles_sql.go` / `sqlite.go` / `postgres.go` — `ContextTokens` 字段、列、迁移。
- 修改 `internal/llm/profile_source.go` / `switch.go`（ModelProfileView）— `ContextTokens` 透传。
- 修改 `internal/api/server_models.go` — payload `context_tokens`（PATCH 指针）。
- 修改 `internal/bootstrap/bootstrap.go` — 种子 ContextTokens；构造 Compactor 注入 Engine；config 映射。
- 修改 `internal/config/config.go` — `conversation.compact_*` 配置与默认归一。
- 前端：`web/chat/src/api.ts`（ModelProfile 类型 + payload）、`pages/ModelSettings.tsx`（上下文长度字段）、对应测试。
