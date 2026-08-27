# 企业执行回调 v0 设计规格

> 状态：已批准（2026-08-23）  
> 日期：2026-08-23  
> 前置：OpenAPI Connector、HTTP 侧车插件、Run Webhook 已落地  
> 依据：`docs/architecture-and-plugin-protocol.md` §4.3

---

## 1. 目标与成功标准

**目标：** 遗留系统不暴露 per-operation HTTP、也不跑侧车 invoke 时，企业提供一个**统一执行回调 URL**；Runtime 仍从 OpenAPI（或侧车发现）获得工具清单，invoke 时 POST 到该 URL。

**成功标准：**

1. Connector 可选字段 `execution_callback_url`；`openapi` 与 `http` 类型均支持
2. 非空时：**跳过** OpenAPI 直连 `base_url` 或侧车 `POST /v0/tools/{name}/invoke`，改为 POST 回调 URL
3. 请求体与架构 §4.3 对齐；响应体与侧车 invoke 对齐 `{ content, is_error }`
4. `auth.mode` 默认头与会话 Identity  overlay 仍出现在回调请求上；HITL / `require_login` 语义不变
5. `idempotency_key` = 本次 LLM `tool_call_id`（引擎注入 context）
6. **设置 → Tools** 每个 Connector 组头可编辑并保存回调 URL（GET+PUT 全量 connector）
7. 参考样例 `examples/enterprise-callback` + 集成测试；README / 架构 §4.3 标已实现

**不做：**

- MCP Connector 走企业回调（v0）
- 侧车 invoke 上下文里的 `callback_urls`（Runtime 反向 URL，另里程碑）
- 注册时 health 探测回调端点
- 重试 / 死信 / 异步回调
- `auth.capture` 完整设置表单（仍下一项）

---

## 2. 协议

### 2.1 Runtime → 企业

```http
POST {execution_callback_url}
Content-Type: application/json
X-Baize-Protocol: v0
X-Baize-Run-Id: run_...
Authorization: ...   # auth.mode / Identity overlay
```

```json
{
  "tool": "create_ticket",
  "arguments": { "title": "..." },
  "run_id": "run_...",
  "agent_id": "agent_...",
  "idempotency_key": "call_abc123"
}
```

- `agent_id`：从 context 读取，可空
- `idempotency_key`：引擎在 `Tools.Invoke` 前写入 context（`tool_call_id`）

### 2.2 企业 → Runtime（响应）

与侧车 invoke 相同：

```json
{ "content": { }, "is_error": false }
```

HTTP 非 2xx 或 `is_error: true` → 引擎侧 tool 错误，不直接把 Run 标 `failed`。

超时：客户端 30s（与 httpplugin 一致）。

### 2.3 工具发现（不变）

| Connector type | 发现来源 | invoke 路径（有 callback URL 时） |
|----------------|----------|-----------------------------------|
| `openapi` | OpenAPI spec | 企业回调 |
| `http` | 侧车 `GET /v0/tools` | 企业回调（仍须可达 sidecar 做发现） |
| `mcp` | MCP list | 不变（直连 MCP） |

`base_url`：OpenAPI 在仅回调模式下可为空（不直连遗留 API）；`http` 仍要求 `base_url`（发现用）。

登录捕获：回调响应 `content` 仍走现有 `capture` 解析（仅 openapi connector）。

---

## 3. 配置与存储

### 3.1 YAML / PUT

```yaml
connector:
  id: legacy
  type: openapi
  spec: examples/mock-ticket/openapi.yaml
  base_url: ""                      # 可空当仅回调
  execution_callback_url: https://enterprise.example/baize/execute
  auth:
    mode: static
    static:
      headers:
        Authorization: "Bearer ${BAIZE_CONNECTOR_TOKEN}"
```

`PUT /v0/connectors/{id}` 与 GET 回显 `execution_callback_url`（URL 本身可回显，非秘密）。

### 3.2 Store

- `store.Connector.ExecutionCallbackURL string`
- SQLite `connectors` 表新列 `execution_callback_url TEXT`（`migrateConnectorsColumns`）
- Memory store 同字段

---

## 4. 实现组件

| 组件 | 职责 |
|------|------|
| `internal/connector/executecallback` | `Client.Invoke(ctx, url, tool, args, meta)`；复用 httpplugin 响应解析 |
| `internal/connector/register_one.go` | `registerOneContext.callbackURL`；openapi/plugin 闭包分支 |
| `internal/connector/apply.go` | `ApplyInput.ExecutionCallbackURL`；持久化 |
| `internal/run/engine.go` | `identity.WithToolCallID(ctx, tc.ID)` 供 idempotency |
| `internal/identity/context.go` | `ToolCallIDFrom` / `WithToolCallID` |
| `internal/api/server.go` | PUT/GET connector 字段 |
| `internal/config` | YAML 字段 |
| `examples/enterprise-callback` | 最小 HTTP 服务：按 tool 分支返回 content |
| `web/chat` | Tools 组头编辑回调 URL + `api.ts` 类型 |

---

## 5. UI（Tools 设置页）

在每个 Connector 组头旁增加「执行回调 URL」：

- 展示当前值（空 = 直连模式）
- 编辑后 `getConnector(id)` → 合并 `execution_callback_url` → `putConnector` 全量字段
- 简短说明：与 OpenAPI 直连互斥路径；企业须实现 §4.3 POST

不新增独立设置导航项（范围小于 Webhook 全局订阅）。

---

## 6. 测试

| 场景 | 断言 |
|------|------|
| OpenAPI + callback | mock 回调收到 tool/arguments/run_id/idempotency_key |
| 直连回退 | URL 空时仍打 base_url |
| HTTP plugin + callback | 发现走 sidecar，invoke 走回调 |
| 鉴权头 | static 默认头出现在回调请求 |
| HITL | `require_approval` 仍 waiting_human |
| 集成 | examples 或 httptest → PUT → Run echo tool |

---

## 7. 文档

- `architecture-and-plugin-protocol.md` §4.3 标已实现；§4.2 `callback_urls` 仍标未实现
- README 中英：企业回调 URL、Tools 页配置路径

---

*批准后进入 `writing-plans` 与 `feat/enterprise-callback` 实现。*
