# HTTP Webhook Inbox v1 设计规格

> 状态：已批准（2026-08-28）  
> 定位：架构 §5「参考 Channel — HTTP Webhook Inbox」的**生产可用首版**。与已落地的出站 Run Webhook 对称，完成「外部 HTTP 进 → Agent Run → 事件 Webhook 出」集成闭环。  
> 前置：SSE、出站 Webhook、HITL、会话持久化、控制面 Gate、`plugincallback` HMAC 模式  
> 命名说明：能力称 **Inbox v1**（生产集成级）；HTTP 路径仍挂在 **`/v0/`** 协议前缀下（与 `X-Baize-Protocol: v0` 一致），不另开 `/v1/` 路由树。

---

## 1. 目标与成功标准

**目标：** 外部系统（监控、ITSM、iPaaS、自研网关）通过 **HTTPS POST + HMAC 验签** 向 Baize 投递消息，Runtime **创建 Run**（可选续聊同一会话），无需打开 `/ui` 或持有 Operator Token。

**生产可用成功标准（宣传可讲）：**

1. 管理员在 **设置 → Inbox** CRUD Channel（绑定 Agent、可选 Skills、轮换 Secret）；YAML 可作启动默认。
2. 集成方持 Channel Secret 签名 POST → **202 Accepted**，返回 `delivery_id` + `run_id` + `conversation_id`。
3. **幂等**：相同 `idempotency_key` 重投返回同一 `run_id`（24h 窗口），不重复建 Run。
4. **续聊**：`external_id` 或显式 `conversation_id` 绑定同一线程，后续消息进入同一对话上下文。
5. **安全默认**：无有效签名 → 401；Channel 禁用 → 404；时间戳偏移 > 5 分钟 → 401；每 Channel 简单速率限制。
6. Run 轨迹含 `inbox.received` 事件；出站 Webhook（若配置）照常推送。
7. 集成测 + Settings UI 测 + `examples/inbox-alert` 端到端故事 + README「生产集成指南」一节。

**明确不做（defer 至 v1.1+）：**

| 项 | 原因 |
|----|------|
| 企业微信 / 钉钉原生协议适配 | 独立 Channel 样板；v1 文档说明「IM 网关转发 JSON 即可」 |
| 入站触发 HITL `resume` | **已实现** — 见 [Inbox HITL resume（I1）设计规格](2026-08-28-inbox-hitl-resume-v0-design.md)（同路径 `action: resume`） |
| 任意 JSONPath / 模板映射 payload | v1 固定 schema；复杂转换由调用方网关完成 |
| 多 Channel 广播、路由规则引擎 | YAGNI |
| IP 白名单、WAF 集成 | 部署侧解决 |
| 出站 Webhook 重试/死信 | **已实现**（2026-08-28，`webhook-outbound-retry-v0`）；见 `docs/superpowers/specs/2026-08-28-webhook-outbound-retry-v0-design.md` |
| 附件 / 多模态入站 | v1 仅 `input` 字符串 |
| `tenant_id` 路由 | 开源默认单租户 |

---

## 2. 概念与数据流

```
外部系统                Baize Runtime                         既有能力
    │                        │                                    │
    │  POST /v0/inbox/{id}   │                                    │
    │  + HMAC 签名            │                                    │
    ├───────────────────────►│ verify → idempotency → thread map  │
    │                        │ → createRun (复用 POST /runs 内核)  │
    │  202 {delivery,run,...}│ → engine Execute                   │
    │◄───────────────────────┤                                    │
    │                        ├───────────────────────────────────►│ SSE / 出站 Webhook
    │                        │                                    │ HITL / Tools / Workflow
```

**Channel** 是配置形态（非第六抽象）：一条入站入口 = 一个 `channel_id` + 绑定 `agent_id` + 独立 Secret。

---

## 3. Channel 配置

### 3.1 字段

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | ✅ | URL 段；`^[a-z][a-z0-9_-]{0,63}$`；创建后不可改 |
| `agent_id` | ✅ | 必须已存在 |
| `enabled` | — | 默认 `true`；`false` 时入站 404 `channel_disabled` |
| `skills` | — | 可选；非空则覆盖该 Agent 默认 `skills`（同 `POST /v0/runs` 语义） |
| `description` | — | 设置页展示用 |
| `webhook_url` | — | 可选；该 Channel 触发的 Run 默认出站 URL（空则用全局 events webhook） |
| `webhook_headers` | — | 与上配对 |
| `secret` | 创建时 | 32 字节随机，Base64URL；**仅创建/轮换时明文展示一次**；持久化供验签 |

### 3.2 持久化

**启动默认：** `configs/*.yaml`：

```yaml
inbox:
  channels: []   # 或预置若干条；secret 空则首次加载时生成并写回 store（日志告警）
```

**运行时权威：** SQLite `settings` KV，key `inbox_channels`，值为 Channel 数组 JSON（与 `events_webhook` 同模式）。  
PUT 后内存 Registry **热更新**；重启从 store 读，store 空则 seed yaml。

**内存 Store（测试）：** 同 KV 语义。

### 3.3 辅助表（SQLite + Memory 实现）

| 表 / 结构 | 用途 |
|-----------|------|
| `inbox_deliveries` | 幂等：`(channel_id, idempotency_key)` UNIQUE → `delivery_id, run_id, body_hash, created_at` |
| `inbox_threads` | 续聊：`(channel_id, external_id)` UNIQUE → `conversation_id` |

幂等窗口：**24h**（`created_at` 超出则同 key 视为新投递）。  
`body_hash` = SHA256(canonical JSON body bytes)；同 key 不同 hash → **409** `idempotency_conflict`。

---

## 4. 入站 API

### 4.1 端点

| 方法 | 路径 | Gate |
|------|------|------|
| `POST` | `/v0/inbox/{channel_id}` | **RoleNone**（不走 Operator Token；仅 HMAC） |

> 控制面 Gate **开启**时：Inbox 端点仍 **匿名可访问**（ACL `RoleNone`），与 `plugin-callbacks` 一致；安全完全依赖 Channel Secret + 签名。

### 4.2 请求头（必填）

| 头 | 说明 |
|----|------|
| `Content-Type` | `application/json` |
| `X-Baize-Inbox-Timestamp` | Unix 秒；与服务端差绝对值 ≤ **300s** |
| `X-Baize-Inbox-Signature` | `v1=<hex>`，见 §4.4 |
| `X-Baize-Protocol` | 可选；若 present 须为 `v0` |

### 4.3 请求体（固定 schema）

```json
{
  "input": "VPN 故障，工号 10086",
  "idempotency_key": "alert-20260828-001",
  "conversation_id": "550e8400-e29b-41d4-a716-446655440000",
  "external_id": "jira-OPS-1234",
  "metadata": { "source": "prometheus", "severity": "high" }
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `input` | ✅ | 非空字符串；trim 后长度 1–8192 |
| `idempotency_key` | 推荐 | 1–128 字符；省略则每次新 Run |
| `conversation_id` | — | 显式续聊；须为合法 UUID 或 store 已有 id |
| `external_id` | — | 外部线程 id（1–256 字符）；与 `conversation_id` 二选一或同时（见 §4.5） |
| `metadata` | — | 仅写入 `inbox.received` 事件，**不**传入 LLM |

**Body 上限：** 64 KiB；超出 **413** `payload_too_large`。

### 4.4 签名算法（v1）

与 Stripe/GitHub 同类，复用 `crypto/hmac`：

```
signed = "<timestamp_unix>.<raw_body_bytes>"
expected = HMAC-SHA256(channel_secret_utf8, signed)
header X-Baize-Inbox-Signature: v1=<lowercase_hex(expected)>
```

- 比较用 `hmac.Equal`  
- Secret 为空或未配置 → Channel 视为不可用（404）  
- 验签失败 → **401** `invalid_signature`  
- 时间戳过期 → **401** `timestamp_skew`

### 4.5 会话解析顺序

1. 若 body 含 `conversation_id` → 使用该值（不存在则 **创建新会话 id 并沿用**，不报错）。  
2. 否则若含 `external_id` → 查 `inbox_threads`；命中用 mapped `conversation_id`；未命中 **生成新 UUID**，写入 mapping。  
3. 否则 → **不绑会话**（`conversation_id` 空，与现有 `POST /v0/runs` 机器路径一致）。

Inbox 触发的 Run **始终**带解析后的 `conversation_id`（当 1/2 路径产生时），以启用会话窗口与身份插件。

### 4.6 响应

**成功 202 Accepted：**

```json
{
  "delivery_id": "dlv_01H...",
  "run_id": "run_01H...",
  "conversation_id": "550e8400-...",
  "status": "accepted"
}
```

**幂等重放：** 同 `(channel_id, idempotency_key)` 且 body hash 相同 → **200 OK**（或 202，实现统一 200）返回**相同 JSON**。

**错误：** 统一 `{ "error": { "code": "...", "message": "..." } }`（与现有 API 一致）。

| HTTP | code | 场景 |
|------|------|------|
| 400 | `invalid_request` | JSON/schema/缺 input |
| 401 | `invalid_signature` / `timestamp_skew` | 验签 |
| 404 | `channel_not_found` / `channel_disabled` | Channel |
| 409 | `idempotency_conflict` | 同 key 不同 body |
| 413 | `payload_too_large` | |
| 429 | `rate_limited` | 见 §4.7 |
| 500 | `internal_error` | |

### 4.7 速率限制（v1 内置）

每 Channel **内存令牌桶**：120 请求 / 分钟（滑动窗口即可，无需 Redis）。  
超限 → **429**，响应头 `Retry-After: 60`。  
单进程部署足够；多副本 defer 到 v1.1（文档注明）。

---

## 5. Run 创建语义（复用内核）

Inbox handler **不得复制** `handlePostRun` 逻辑；抽取共享函数（如 `server.createRunFromInput`）供两处调用。

| 项 | Inbox 行为 |
|----|------------|
| Agent | Channel 配置 `agent_id` |
| Input | body.input（无附件、无 @skill 解析——v1 不支持 mention；skills 仅来自 Channel/body 扩展） |
| Skills | Channel.skills 非空 → 作为 run skills；否则 Agent 默认 |
| Conversation | §4.5 解析结果 |
| Session auth | **不**走 `session_token`；Inbox 为服务间集成 |
| Webhook 出站 | Channel 级 `webhook_url` 优先，否则全局 settings |
| 引擎 | 与 UI/API 相同：`CreateRun` → `Execute` 异步 |

**首事件（Run 创建后、engine 前）：**

```json
{
  "type": "inbox.received",
  "data": {
    "channel_id": "alerts",
    "delivery_id": "dlv_...",
    "external_id": "jira-OPS-1234",
    "idempotency_key": "alert-20260828-001",
    "metadata": { }
  }
}
```

---

## 6. 管理 API（admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v0/settings/inbox-channels` | 列表；secret **不回显**，仅 `secret_hint`（末 4 字符） |
| PUT | `/v0/settings/inbox-channels` | 全量替换数组（与 connectors 批量风格一致） |
| POST | `/v0/settings/inbox-channels/{id}/rotate-secret` | 新 secret 明文返回一次 |
| POST | `/v0/settings/inbox-channels/{id}/test` | 服务端代发一条 `input: "inbox test"` 并返回 `{ delivery_id, run_id }` |

ACL：**RoleAdmin**（与 `events-webhook` 设置同级）。

校验失败（未知 agent、重复 id、非法 slug）→ 400，**整批 PUT 不部分生效**。

---

## 7. UI

| 位置 | 内容 |
|------|------|
| `/settings/inbox` | Channel 列表、新建/编辑/启用开关、Agent 下拉、Skills 多选、出站 Webhook 覆盖、Secret 轮换、**复制入站 URL**、**发送测试** |
| `settingsNav` | admin 导航增加「Inbox」 |
| 样式 | 对称 `WebhookSettings.tsx`；`AdminOnly` |

页面说明块（固定文案）：

- 入站 URL 形如 `https://<host>/v0/inbox/{channel_id}`  
- 签名示例（curl + OpenSSL/Python 片段链接到 README）  
- 与出站 Webhook 配对使用的架构图（文字即可）

---

## 8. 实现组件

| 包/文件 | 职责 |
|---------|------|
| `internal/inbox/model.go` | Channel、Delivery、Verify 输入类型 |
| `internal/inbox/verify.go` | HMAC v1、时间戳、body 读取 |
| `internal/inbox/registry.go` | 内存 Channel 表 + 热更新 |
| `internal/inbox/ratelimit.go` | 每 channel 令牌桶 |
| `internal/inbox/idempotency.go` | 存取 deliveries / threads（接口 + sqlite/memory） |
| `internal/api/server_inbox.go` | 入站 + settings handlers |
| `internal/api/server.go` | 路由；抽取 `createRunFromInput` |
| `internal/store/` | `inbox_deliveries`、`inbox_threads` 表迁移 |
| `internal/config/config.go` | `inbox.channels` yaml |
| `internal/bootstrap/` | seed + Registry 注入 |
| `internal/controlplane/acl.go` | Inbox 路由规则 |
| `web/chat/src/pages/InboxSettings.tsx` + test | 设置 UI |

**不新建** `Channel` 顶级抽象包名冲突——用 `internal/inbox` 即可。

---

## 9. 测试策略

| 层 | 内容 |
|----|------|
| 单测 | 签名校验（好/坏/过期）、幂等（同 key 同/异 body）、thread mapping、rate limit |
| API 测 | settings CRUD、rotate、test、401/404/409/429 |
| 集成测 | `tests/integration/inbox_test.go`：建 Channel → 签名 POST → poll run succeeded；重放幂等；external_id 两次同 conversation |
| 前端 | `InboxSettings.test.ts`：表单校验、保存 payload 形状 |
| 示例 | `examples/inbox-alert/README.md` + 可选 shell 脚本向本地 Inbox POST |

---

## 10. 文档与宣传素材

| 文档 | 内容 |
|------|------|
| README.zh-CN / README.md | 「生产集成：Webhook Inbox」小节；30 分钟故事：Prometheus/脚本 → Inbox → ticket-triage 工作流 → 出站 Webhook |
| `docs/architecture-and-plugin-protocol.md` §5 | Channel 行改为「HTTP Webhook Inbox v1（已实现）」+ 链路到 README |
| `examples/inbox-alert/` | 可运行最小示例 |

**对外一句话：** 「告警和工单系统 POST 到 Baize Inbox，Agent 自动跑；结果 Webhook 回你的平台。」

---

## 11. 安全与部署清单

生产部署检查（写入 README）：

1. 启用 `control_plane.admin_token`；Inbox URL 仅在内网或 API 网关后暴露  
2. 每 Channel 独立 Secret；定期轮换  
3. 调用方必须带 `idempotency_key`  
4. 配置全局或 Channel 级出站 Webhook 做审计  
5. 网关层可选 WAF / mTLS（Baize 不内置）

---

## 12. 文件清单（实现阶段）

| 路径 | 动作 |
|------|------|
| `internal/inbox/*` | 新建 |
| `internal/store/sqlite.go` + `memory.go` | 表 + CRUD |
| `internal/api/server_inbox.go` | 新建 |
| `internal/api/server.go` | 抽取 createRun + 路由 |
| `internal/config/config.go` | inbox yaml |
| `internal/bootstrap/` | seed |
| `internal/controlplane/acl.go` | ACL |
| `web/chat/src/pages/InboxSettings.tsx` + test | UI |
| `tests/integration/inbox_test.go` | 集成 |
| `examples/inbox-alert/` | 示例 |
| README ×2、`docs/architecture-and-plugin-protocol.md` | 文档 |

---

## 13. 规格自检

| 检查项 | 结果 |
|--------|------|
| 占位符 / TODO | 无 |
| 内部一致性 | 路径 `/v0/` 与能力名 v1 已在文首说明；Gate RoleNone + HMAC 与 plugin-callbacks 对齐 |
| 范围 | 单实现计划可覆盖；出站重试明确 defer |
| 模糊性 | 会话解析顺序、幂等窗口、签名算法、错误码已钉死 |
| 与现有代码 | 复用 handlePostRun 内核、settings KV、HMAC 模式、Webhook Dispatcher |

---

*下一步：用户批准本规格后，调用 **writing-plans** 编写 `docs/superpowers/plans/2026-08-28-webhook-inbox-v1.md`。*
