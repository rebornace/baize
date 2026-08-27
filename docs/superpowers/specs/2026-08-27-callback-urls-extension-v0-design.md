# callback_urls 扩展 v0 设计规格

> 状态：待用户审查  
> 日期：2026-08-27  
> 前置：`2026-08-26-connector-delete-and-callback-urls-v0`（HTTP 侧车注入 + `plugin-callbacks` 接收端已落地）  
> 依据：头脑风暴选定方案 1 — MCP + 企业 execution_callback 注入，复用现有接收端与 token  
> 分支建议：`feat/callback-urls-extension-v0`

---

## 1. 目标与成功标准

**目标：** 在 HTTP 侧车已支持的 `callback_urls.event` 基础上，把**同一签名 URL** 扩到 **MCP `tools/call`** 与 **企业 execution_callback** 路径，使 MCP Server / 企业统一回调也能异步回投 Run 事件，无需 TS/Python SDK。

**一句话成功标准：** MCP invoke 或企业回调 invoke 在配置齐全时收到 `callback_urls.event`；目标方 POST 后该 Run 的 SSE/事件列表出现 `plugin.callback`。

### 做

| 项 | 内容 |
|----|------|
| 共享 | 将 URL 签发逻辑集中到可复用函数（`plugincallback.FormatTokenURL` + Issue 或等价）；HTTP / MCP / 企业路径共用 |
| MCP | `tools/call` 的 `_meta` 写入 Baize 命名空间键（见 §3.1） |
| 企业回调 | Runtime→企业 POST body 增加 `callback_urls.event`（见 §3.2） |
| 接收端 | **复用** `POST /v0/runs/{run_id}/plugin-callbacks`；事件类型仍为 `plugin.callback` |
| 测试 | MCP `_meta` 单测；enterprise callback body 单测；至少一条端到端回投（MCP mock 或 `examples/enterprise-callback`） |
| 文档 | 架构 §4.2/§4.3；README 配置说明一句；更新 v0 规格交叉引用 |

### 不做

- 新接收 API、新事件类型、新 token 格式
- **OpenAPI 直连遗留 API** 注入（下游无 Baize 语义）
- MCP 走企业 execution_callback（仍不支持，见 enterprise-callback 规格）
- Webhook 重试/死信、SDK、专用 progress UI
- 要求所有 MCP Server 必须理解 callback（可选能力；不识别则忽略 `_meta`）
- 改变 Run 状态机或自动结束 tool turn

### 可测成功标准

1. 配置 `runtime.public_base_url` + secret、invoke 带 `run_id` 时，MCP `CallToolParams._meta` 含 `io.baize/callback_urls.event`
2. 同上条件下，企业 execution_callback POST body 含 `callback_urls.event`
3. 两路径合法 POST → Store 追加 `plugin.callback`；坏 token → 401
4. 无 `run_id` / 无 `public_base` → 两路径均**省略** `callback_urls`（与 HTTP 侧车一致）
5. HTTP 侧车注入回归仍绿

---

## 2. 注入规则（全局，与 HTTP v0 一致）

| 条件 | 行为 |
|------|------|
| `run_id` 空 | 省略整个 `callback_urls` |
| `public_base` 不可用（未配置且无法安全推导） | 省略并记 debug/warn 日志 |
| `CallbackSigner` / secret / TTL 任一缺失 | 省略 |
| token | 沿用 `plugincallback.Issue`：HMAC 绑定 `run_id`，TTL 默认 1h（可配置） |
| URL 格式 | `{public_base}/v0/runs/{run_id}/plugin-callbacks?token={token}` |

**不**把 Admin API Key 交给 MCP Server 或企业侧车。

---

## 3. 各路径注入形状

### 3.1 MCP `tools/call`

在 `internal/connector/mcp.CallTool`（或调用处）构造 `mcp.CallToolParams` 时写入 `_meta`：

```json
{
  "io.baize/run_id": "run_...",
  "io.baize/agent_id": "agent_...",
  "io.baize/callback_urls": {
    "event": "https://runtime.example/v0/runs/run_.../plugin-callbacks?token=..."
  }
}
```

规则：

- 键前缀 `io.baize/` 遵循 MCP 扩展元数据命名；实现使用 go-sdk `CallToolParams.Meta`（`json:"_meta"`）
- 无 callback 可注入时：**不**写 `io.baize/callback_urls`；`run_id`/`agent_id` 可选写入（便于 Server 日志；若省略可减少噪音，实现二选一并在测试中固定）
- stdio 与 Streamable HTTP MCP 均走同一 `CallTool` 包装
- MCP Server **不必**回投；识则 POST `event` URL，不识则忽略

### 3.2 企业 execution_callback

扩展 `executecallback.Client.Invoke` body（`internal/connector/executecallback`）：

```json
{
  "tool": "create_ticket",
  "arguments": {},
  "run_id": "run_...",
  "agent_id": "agent_...",
  "idempotency_key": "call_...",
  "callback_urls": {
    "event": "https://runtime.example/v0/runs/run_.../plugin-callbacks?token=..."
  }
}
```

规则：

- 仅当 `execution_callback_url` 非空且走企业回调路径时附加
- 注入条件与 §2 相同；省略时不带 `callback_urls` 键（非空对象）
- OpenAPI / HTTP 插件在配置了 `execution_callback_url` 时均受益
- `examples/enterprise-callback` 增加演示：收到 body 后可选 POST `callback_urls.event` 写一条进度/备注

### 3.3 明确不注入

| 路径 | 原因 |
|------|------|
| OpenAPI 直连 `base_url` | 遗留 REST 无 Baize 回调契约 |
| HTTP 侧车直连 | **已实现**（本里程碑回归即可） |
| MCP + 企业回调组合 | MCP 仍直连 MCP Server；企业回调规格不变 |

---

## 4. 实现要点（非绑定）

### 4.1 代码结构

- 新增小函数，例如 `plugincallback.EventURL(signer, secret, publicBase, ttl, runID) string`，内部 Issue + FormatTokenURL；`httpplugin.SignCallbackEventURL` 改为薄包装或迁到 `plugincallback`
- `registerOneContext` 已有 `callbackSigner` / `callbackSecret` / `callbackPublicBase` / `callbackTTL`；**MCP 与 enterprise 闭包**读取同一组字段
- `mcpInvokerClosure`：CallTool 前计算 event URL，写入 `_meta`
- `invokeEnterpriseCallback`：传入 event URL 给 `executecallback.InvokeMeta`

### 4.2 回调契约（不变）

与 `2026-08-26-connector-delete-and-callback-urls-v0` §3.4 相同：

```http
POST /v0/runs/{run_id}/plugin-callbacks?token=...
Content-Type: application/json
X-Baize-Protocol: v0

{ "type": "event", "name": "sidecar.note", "payload": {} }
```

- ACL：`RoleNone`（仅 token）
- payload 64 KiB 上限；每 run 限流 100/h（沿用现有实现）

### 4.3 测试计划

| # | 用例 |
|---|------|
| 1 | `EventURL` / 共享签发：有 run+base → 非空 URL；缺 run → 空 |
| 2 | MCP `CallTool`：mock session 断言 `_meta["io.baize/callback_urls"]` |
| 3 | MCP 无 run_id：`_meta` 无 callback_urls |
| 4 | `executecallback.Client`：body JSON 含/不含 `callback_urls` |
| 5 | API：`plugin-callbacks` 接收 MCP/企业路径签发的 token（可复用现有测） |
| 6 | 回归：`httpplugin` 注入测、`TestPutHTTPPluginPreservesCaptureAndUsesIdentity` 等无关但需全绿 |

---

## 5. 文档

- `docs/architecture-and-plugin-protocol.md` §4.2：注明 MCP `_meta` 与企业回调 body 亦注入 `callback_urls`
- `docs/architecture-and-plugin-protocol.md` §4.3：企业回调请求体增加 `callback_urls` 字段说明
- `2026-08-26-connector-delete-and-callback-urls-v0-design.md` 文首交叉引用本规格
- README 中英：一句「MCP / 企业执行回调亦支持异步回投 Run 事件」

---

## 6. 非目标回顾

本里程碑补齐 **callback_urls 在 MCP 与企业回调路径的对称注入**，服务「插拔内核 / 异步集成不绑 SDK」。不扩展 OpenAPI 直连、不改变接收语义。

---

*头脑风暴方案 1 已批准；实现前若 `_meta` 键名微调，以本文语义为准并更新本文。*
