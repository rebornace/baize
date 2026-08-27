# 对话回滚与 Fork v0 设计规格

> 状态：已批准（2026-08-23）  
> 日期：2026-08-23  
> 前置：`2026-08-12-conversation-persistence-design.md`（线性 Message Store + Chat UI）  
> 产品对齐：Cursor 式 — **同会话截断续聊** + **前缀复制到新会话**

---

## 1. 目标与成功标准

**目标：** 操作员在 `/ui` 对历史气泡执行 **回滚**（截断后续并续聊）与 **Fork**（复制前缀到新对话），行为与主流 Agent 产品一致；不改变 Run/Events 审计模型。

**成功标准：**

1. **回滚到用户消息**：从该条用户消息起删除后续（含该条）→ 可改文再发 → 新 Run 仅看到截断后历史  
2. **重新生成**：从某条助手回复起删除后续（含该助手）→ 自动用上一条用户内容发起新 Run  
3. **Fork**：复制「到某条消息为止」的前缀到新 `conversation_id` → UI 切换新会话；原会话不变  
4. 有 **非终态 Run**（`queued` / `running` / `waiting_human`）时，回滚/Fork 返回 `409 conversation_busy`  
5. Run / Events **不删除**；仅 `messages` 表变更  
6. Fork **不复制** identities（新会话需重新登录）  
7. SQLite 与 memory 驱动行为一致；集成测试覆盖 Store + API + 纯函数

---

## 2. 范围

### 做（v0）

| 层 | 内容 |
|----|------|
| Store | `TruncateFrom`、`Fork`（Memory + SQLite） |
| Store（runs） | `HasActiveRun(conversationID)` 供 API 互斥 |
| API | `POST .../rollback`、`POST .../fork` |
| UI | 气泡菜单：编辑并回滚（user）、重新生成（assistant）、Fork 到此 |
| UI | 用户消息回滚后 Composer 预填被删内容，可编辑后发送 |
| 测试 | Store 单测、API 集成、前端纯函数（若有） |

### 不做（v0）

- 分支树可视化、`parent_message_id` 图模型  
- 删除单条消息而不截断后续  
- Fork 时复制 identities / 凭证  
- 回滚时删除或修改 Run/Events  
- 机器路径（无 `conversation_id`）  
- 对话标题自动生成（仍用首条 user 截断）  

---

## 3. 概念与语义

### 3.1 线性历史不变

Message 仍为 `(conversation_id, created_at)` 有序列表。回滚 = **物理删除** 一段后缀（非软删）。Fork = **复制前缀** 到新 `conversation_id`（新 `msg_` id）。

### 3.2 回滚（Rollback）

**操作对象：**

| 气泡 role | 菜单项 | 截断范围 | 后续 |
|-----------|--------|----------|------|
| `user` | **编辑并回滚** | 删除 **该条及之后** 全部消息 | Composer 预填该条 `content`；用户改文后 `POST /v0/runs` |
| `assistant` | **重新生成** | 删除 **该条及之后** 全部消息 | 取截断后列表最后一条 `user` 的 `content` 自动 `POST /v0/runs` |
| `system_note` | **回滚到此** | 删除 **该条及之后** | 不自动发 Run；用户手动输入 |

**Store：** `TruncateFrom(conversationID, messageID)` — 删除 `created_at >= anchor.created_at` 的所有消息（含 anchor）。若 anchor 不存在 → `ErrNotFound`。

### 3.3 Fork

**操作：** 对任意消息（user / assistant / system_note）执行 **Fork 到此**。

**语义：** 复制源会话中 `created_at <= anchor.created_at` 的所有消息到新会话（保持顺序），每条生成新 `msg_` id、`run_id` 原样复制（仅作关联展示，不保证 Run 仍存在）。

**新会话 id：** 服务端生成 `conv_` + UUID（与 UI `newConversationId` 一致）。

**Identities：** 不复制；新会话 identities 为空。

**Store：** `Fork(srcConversationID, throughMessageID) (newConversationID string, copied int, error)`

---

## 4. 数据模型

无新表、无新列。`messages` 表结构不变。

**Fork 元数据（v0 可选）：** 不在 DB 记录 `forked_from`；UI 侧新会话标题仍由 `ListSummaries` 首条 user 推导。若需「从 xxx 分叉」展示，v1 可加 `conversations` 表。

---

## 5. Store API

扩展 `conversation.Store`：

```go
// TruncateFrom deletes anchor and all messages after it in the same conversation.
TruncateFrom(conversationID, messageID string) (deleted int, err error)

// Fork copies messages up through throughMessageID into a new conversation.
Fork(srcConversationID, throughMessageID string) (newConversationID string, copied int, err error)
```

**错误：**

| 错误 | 场景 |
|------|------|
| `ErrNotFound` | conversation 无消息 / messageID 不属于该 conversation |
| `ErrInvalidRequest` | 空 id |

**SQLite：** `DELETE FROM messages WHERE conversation_id = ? AND created_at >= ?`（TruncateFrom 先 SELECT anchor `created_at`）。Fork：`INSERT` 批量复制，`created_at` 保留原值以保持顺序。

**ListSummaries：** 截断/Fork 后自然反映；空会话不出现在列表（与现行为一致）。

扩展 `store.Store`（或 Server 内 SQL）：

```go
HasActiveRun(conversationID string) (bool, error)
```

查询 `runs` 中 `conversation_id = ? AND status IN ('queued','running','waiting_human')` 是否存在行。

---

## 6. HTTP API

基路径：`/v0/conversations/{id}/...`；需与现有 gate 一致（operator/admin）。

### 6.1 回滚

```
POST /v0/conversations/{id}/messages/{message_id}/rollback
```

**Body（可选）：**

```json
{ "regenerate": false }
```

| 字段 | 默认 | 说明 |
|------|------|------|
| `regenerate` | `false` | `true` 且 anchor 为 `assistant` 时，截断后自动创建 Run（见 §6.3） |

**成功 `200`：**

```json
{
  "conversation_id": "conv_...",
  "deleted_count": 3,
  "messages": [ /* 截断后全量，时间正序 */ ],
  "regenerated_run": null
}
```

当 `regenerate: true` 且成功发起 Run：

```json
{
  "regenerated_run": {
    "run_id": "run_...",
    "status": "running"
  }
}
```

**错误：**

| HTTP | code | 说明 |
|------|------|------|
| 400 | `invalid_request` | 缺 message_id、消息不属于该 conversation |
| 404 | `not_found` | conversation / message 不存在 |
| 409 | `conversation_busy` | 存在非终态 Run |
| 409 | `regenerate_unavailable` | `regenerate: true` 但截断后无 user 消息可重跑 |

### 6.2 Fork

```
POST /v0/conversations/{id}/fork
```

**Body：**

```json
{
  "through_message_id": "msg_..."
}
```

**成功 `200`：**

```json
{
  "source_conversation_id": "conv_old",
  "conversation_id": "conv_new",
  "copied_count": 5,
  "messages": [ /* 新会话全量 */ ]
}
```

**错误：** 同回滚（404 / 409 busy / 400）。

### 6.3 服务端 Regenerate 流程

仅当 `POST rollback` 且 `regenerate: true`：

1. `TruncateFrom`  
2. `List` 取最后一条 `role=user`；无则 `409 regenerate_unavailable`  
3. 与 `POST /v0/runs` 相同路径创建 Run（同 `agent_id` 取自配置默认 agent 或请求体 — **v0 用 Server 默认 agent：`cfg.Agent.ID`**，body 可选 `agent_id`）  
4. 返回 `regenerated_run`；**不在 rollback 内写 user message**（Truncate 已删掉该 assistant，历史中应仍有 user 行；若 regenerate 删 assistant 保留 user，则 `POST runs` 会再 Append 一条 user — **需与 Engine 去重逻辑一致**）

**去重：** `buildMessages` 已有「末尾 user == input 则跳过」；regenerate 应用 **同一 input** 创建 Run，依赖该去重，避免双 user。

**agent_id：** Body 可选 `{ "agent_id": "...", "regenerate": true }`；缺省 `cfg.Agent.ID`。

---

## 7. UI（`/ui` ChatPage）

### 7.1 气泡操作

每条 **已持久化** 消息（非 `local_*` 乐观行）悬停或 `⋯` 菜单：

| role | 项 | 行为 |
|------|-----|------|
| user | 编辑并回滚 | `POST rollback` → 刷新 messages → Composer 填入该条 content |
| assistant | 重新生成 | `POST rollback` + `regenerate: true` → 刷新 → 接 SSE 同 `onSend` |
| any | Fork 到此 | `POST fork` → `setConversationId(new)` → 刷新列表与 messages |

`system_note`：仅「回滚到此」（无 regenerate）。

### 7.2 互斥

- `busy` 或 `liveRunId != null` 时禁用菜单  
- API 返回 `conversation_busy` 时 toast / status 提示  

### 7.3 样式

沿用 `chat-shell` / `bubble`；菜单用轻量 `bubble-menu`（与 settings 按钮风格一致），不引入新主题。

### 7.4 `api.ts`

```ts
rollbackMessages(convId, messageId, opts?: { regenerate?: boolean; agentId?: string })
forkConversation(convId, throughMessageId)
```

---

## 8. 与 Run / Events 的关系

| 实体 | 回滚/Fork 后 |
|------|----------------|
| `messages` | 截断或复制 |
| `runs` | **保留**；旧 run_id 在消息上仍可展示，但历史窗口不再包含对应轮次 |
| `events` | **保留** |
| `identities` | 回滚不影响；Fork 不复制 |

审计员仍可通过 Run ID 查完整工具链；模型侧仅见新窗口。

---

## 9. 安全

- 与现有 conversation API 相同 gate（operator 起）  
- 不回滚/Fork 其他用户的会话（单机部署无多租户；`conversation_id` 即边界）  
- 消息内容不含凭证（与 persistence 规格一致）  

---

## 10. 测试计划

| 用例 | 期望 |
|------|------|
| TruncateFrom 中间 user | 后续全删；之前保留 |
| TruncateFrom 最后 assistant | 仅删 assistant |
| Fork 5 条前缀 | 新 conv 5 条；新 msg id；源 conv 不变 |
| Fork 不复制 identity | 新 conv List identities 空 |
| rollback API + active run | 409 |
| regenerate assistant | 新 run；messages 含 user；assistant 由终态写入 |
| UI 纯函数（可选） | 菜单可见性按 role |

---

## 11. 已锁定决策

| 决策 | 选择 |
|------|------|
| 方案 | A：截断 + 一次性 Fork 复制 |
| 回滚语义 | 含 anchor 及之后全删 |
| Regenerate | 服务端 `rollback` + `regenerate: true` |
| Fork 凭证 | 不复制 identities |
| Run/Events | 不删 |
| 活跃 Run | 禁止回滚/Fork |
| 分支元数据 | v0 不存 parent |

---

## 12. 实现顺序建议

1. `TruncateFrom` / `Fork` + Store 测试  
2. `HasActiveRun` + rollback/fork handlers + API 测试  
3. `api.ts` + ChatPage 菜单 + 样式  
4. `npm run build` → `internal/ui/dist`  
5. README 简短说明（操作员文档）  
6. 集成测试：截断后第二轮 LLM 不含被删内容  

正式任务拆解在规格批准后由 `writing-plans` 产出。
