# Inbox HITL resume（I1）设计规格

> 状态：已批准（2026-08-28）  
> 日期：2026-08-28  
> 前置：Webhook Inbox v1、HITL `POST /v0/runs/{id}/resume`  
> 依据：开源首版边界清单（I1 本版必做）；头脑风暴选定「同路径扩 action」（方案 1）  
> 路线：本里程碑 → PostgreSQL（P4）→ 文档边界 + 生产硬化（F）

---

## 1. 目标与成功标准

**目标：** 外部系统使用**同一** Inbox 入口与 Channel HMAC，对处于 `waiting_human` 的 Run 执行 approve / reject，无需控制面 Operator Token。

**成功标准：**

1. `action` 缺省或为 `create_run` 时，行为与 Inbox v1 **完全兼容**（既有测试不改语义）。
2. `action: "resume"` 时，必填 `run_id` + `decision`（`approve`|`reject`），内部走与 `POST /v0/runs/{id}/resume` **同一** `ContinueFromHITL` 路径。
3. Chat UI 人工审批与 Inbox resume **可并存**：先成功者生效；后到者得到明确冲突（非静默双批）。
4. 验签、时间戳、限速、幂等表与 Inbox v1 共用；集成测 + README「机器审批」示例。
5. **归属校验：** `run.agent_id` 必须等于该 Channel 的 `agent_id`，否则拒绝。

**明确不做：**

| 项 | 原因 |
|----|------|
| 新 URL（如 `/inbox/.../resume`） | 方案 1：同路径扩 action |
| 省略 `run_id`、按会话自动找 waiting Run | 歧义大；须显式指定 |
| 从 Inbox 请求刷新 passthrough Authorization | v0 保持简单；机器审批不拷控制面头 |
| 附件 / JSONPath / 多订阅 / SDK | 已划出本版或后续 |

---

## 2. API

### 2.1 端点（不变）

| 方法 | 路径 | Gate |
|------|------|------|
| `POST` | `/v0/inbox/{channel_id}` | **RoleNone**（仅 HMAC） |

请求头与签名算法与 Inbox v1 **完全相同**（`X-Baize-Inbox-Timestamp` / `X-Baize-Inbox-Signature: v1=...`）。

### 2.2 请求体

**创建 Run（兼容，默认）：**

```json
{
  "action": "create_run",
  "input": "...",
  "idempotency_key": "...",
  "conversation_id": "...",
  "external_id": "...",
  "metadata": {}
}
```

- `action` 可省略，等价 `create_run`。
- 其余字段语义同 Inbox v1。

**Resume：**

```json
{
  "action": "resume",
  "run_id": "run_...",
  "decision": "approve",
  "comment": "可选说明",
  "idempotency_key": "alert-001-approve"
}
```

| 字段 | create_run | resume |
|------|------------|--------|
| `action` | 可省略 | 必填 `"resume"` |
| `input` | 必填 | 忽略（可不传） |
| `run_id` | 忽略 | **必填** |
| `decision` | 忽略 | **必填** `approve` \| `reject` |
| `comment` | 忽略 | 可选字符串 |
| `idempotency_key` | 推荐 | **强烈推荐** |
| `conversation_id` / `external_id` / `metadata` | 同 v1 | **忽略** |

未知 `action` → **400** `invalid_request`。

### 2.3 响应

**resume 成功：200 OK**

```json
{
  "delivery_id": "dlv_...",
  "run_id": "run_...",
  "status": "running",
  "action": "resume"
}
```

`status` 为 resume 调用后 Store 中的 Run 状态（与 `handlePostResume` 一致）。

**幂等重放：** 同 `(channel_id, idempotency_key)` 且 body hash 相同 → **200**，返回**首次成功**时保存的响应 JSON（含当时 `run_id` / `status`）。

**create_run：** 保持 Inbox v1（202 Accepted / 幂等 200）。

### 2.4 错误码（resume 增量）

| HTTP | code | 条件 |
|------|------|------|
| 400 | `invalid_request` | 缺 `run_id`/`decision`、decision 非法、action 未知 |
| 401 | `invalid_signature` / `timestamp_skew` | 同 v1 |
| 403 | `run_forbidden` | Run 存在但 `agent_id` ≠ Channel.`agent_id` |
| 404 | `not_found` | Channel 不存在/禁用；或 `run_not_found` |
| 409 | `not_waiting` | Run 状态不是 `waiting_human`（含已被 UI 批过） |
| 409 | `idempotency_conflict` | 同 key 不同 body hash（同 v1） |
| 429 | 限速 | 同 v1 |

`run_not_found` 与 Channel `not_found` 对外均可映射为既有错误体风格；实现上 resume 路径对缺失 Run 使用 **404** `run_not_found`，与控制面 resume 对齐。

---

## 3. 处理流程

```
验签 → 限速 → 解析 JSON → 按 action 分支
  create_run → 现有 Inbox 建 Run 流程
  resume →
      校验 run_id / decision
      幂等：若命中且 hash 同 → 返回缓存响应
      GetRun；缺失 → 404
      agent_id 与 Channel 不一致 → 403 run_forbidden
      status ≠ waiting_human → 409 not_waiting
      ContinueFromHITL(approve|reject, comment)
      AppendEvent inbox.resumed（decision/comment 摘要，无 secret）
      写入幂等行（若有 key）→ 200 JSON
```

**与 UI 并发：** 不引入分布式锁；依赖 Store/引擎既有状态机。若 UI 已 resume 成功，Inbox 再来 → `not_waiting`。若 Inbox 先成功，UI 再点 → UI 侧既有 `not_waiting`。

**Passthrough：** Inbox resume **不**调用 `SetPassthroughHeaders`（与设计拍板一致）。

---

## 4. 事件

| type | 何时 | data（示例字段） |
|------|------|------------------|
| `inbox.resumed` | resume 成功调用 ContinueFromHITL 之后 | `channel_id`, `delivery_id`, `decision`, `comment`（可空） |

不在此事件中写入 URL、Secret 或完整请求体。

---

## 5. 测试与文档

- **单测 / API 测：** create 兼容；resume approve/reject；403 归属；409 not_waiting；幂等重放；非法 action。
- **集成测：** mock LLM 触发 `waiting_human` → Inbox resume → Run 终态符合预期。
- **文档：** README「生产集成」增加机器审批 curl；可选 `examples/inbox-alert` resume 片段。
- **规格交叉引用：** Inbox v1 defer「入站触发 HITL resume」标为已由本规格实现；架构 Channel 行可补一句「支持 resume action」。

---

## 6. 实现落点（提示，非计划）

| 区域 | 变更 |
|------|------|
| `internal/inbox` | Payload 增加 `Action`/`RunID`/`Decision`/`Comment`；校验辅助 |
| `internal/api/server_inbox.go` | action 分支；resume 处理器复用 Runner |
| 幂等 | 复用 `PutInboxDelivery`；resume 响应 JSON 形状写入既有字段策略（与 create 一致：存 delivery 映射 run_id；若需缓存 status，用既有或扩展最小字段——实现计划选定，须满足幂等重放语义） |
| 测试 / README / examples | 见 §5 |

幂等缓存细节：若当前 `InboxDelivery` 仅存 `run_id`，resume 重放至少返回相同 `run_id`，`status` 可现查 Store；若 Run 已终态仍返回当前 status + `action: resume`。实现计划不得削弱「同 key 同 body → 不重复 ContinueFromHITL」。

---

## 7. 规格自检

| 检查 | 结果 |
|------|------|
| 占位符 / TODO | 无（幂等 status 重放策略交给实现计划，语义已钉死） |
| 与 Inbox v1 / resume API 一致性 | 签名、Gate、ContinueFromHITL 对齐 |
| 范围 | 单里程碑可覆盖；不含 PG / F |
| 模糊性 | action 默认、归属 403、不做 passthrough 已写死 |

---

*下一步：用户审查本文件后，调用 **writing-plans** 编写 `docs/superpowers/plans/2026-08-28-inbox-hitl-resume-v0.md`。*
