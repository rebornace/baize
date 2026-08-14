# Baize 设计规格：HTTP 侧车插件协议 v0

> 状态：已批准  
> 日期：2026-08-14  
> 前置：Demo C 接入契约、Connector `auth.mode` 已落地  
> 依据：`docs/architecture-and-plugin-protocol.md` §4.1–4.2；Demo C 后续候选第 2 条

---

## 0. 动机

主路径仍是 **OpenAPI → Tool → Runtime 直接 HTTP 调用遗留 API**。没有干净 OpenAPI / Swagger 的系统需要第二条路：在旁边跑一个小侧车，按固定 HTTP JSON 协议暴露工具，Runtime 当客户端去发现和调用。架构草案已写协议，代码里 `type` 仍只认 `openapi`。

本里程碑把协议接到 Registry，并给一份最小参考侧车。不做 MCP，不做企业回调。

---

## 1. 目标与成功标准

**目标：** 平台可以 `PUT type=http` 的 Connector（只需 `base_url`），Runtime 从侧车拉工具清单并在 Run 里 invoke；HITL 与 `auth.mode` 与 OpenAPI Connector 同一套规则。

**成功标准：**

1. 可达的参考侧车上，`PUT /v0/connectors/{id}`（`type: http`）成功；`GET /v0/tools` 含该 Connector 的工具（name、description、`connector_id`）
2. mock LLM 的 Run 能选中并调用侧车工具；invoke 请求带 `X-Baize-Protocol: v0` 与 `X-Baize-Run-Id`
3. `require_approval` 名单中的侧车工具仍走 `waiting_human`
4. 无会话身份时，`auth.mode: static` 解析出的默认头出现在侧车收到的请求上
5. healthz 失败、空工具清单、协议版本不匹配 → `400 invalid_plugin`，既有 Registry 不变
6. 开箱 `baize start` 默认路径仍是 mock-ticket OpenAPI，不自动启动参考侧车

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| 协议客户端 | 新包 `internal/connector/httpplugin`：healthz、list tools、invoke |
| 控制面 | `PUT` / YAML 支持 `type: http`（无需 `spec`）；按 type 分流；失败 `invalid_plugin` |
| Registry | 侧车 ToolDesc 登记为现有 Tool；冲突 / 同 id 替换语义不变 |
| 鉴权 | 复用 `auth.mode` 默认头 + 会话 Identity 优先；本轮无登录捕获 |
| HITL | 仅 `require_approval` 名单；`annotations.dangerous` 不自动开门闸 |
| 样板 | `examples/http-plugin`：healthz、tools、echo + create_ticket |
| 文档 | README 中英「无 OpenAPI：HTTP 插件」；架构 §4.2 与实现对齐 |
| 测试 | 发现/invoke 单测 + 注册失败不污染 + 集成 PUT→GET tools→Run |

### 不做

- MCP 桥
- 企业执行回调、invoke `callback_urls`
- 按 `annotations.dangerous` 自动 HITL
- 侧车工具的登录捕获
- 开箱改默认 Connector / 自动拉起参考侧车
- 重试、可配置超时（本轮固定合理超时，如 30s）

---

## 3. 架构

```
PUT /v0/connectors/{id}  type=http  base_url
          │
          ▼
httpplugin.Register
  GET  {base}/healthz
  GET  {base}/v0/tools
          │  ToolDesc[] → Registry（与 OpenAPI 工具共存）
          ▼
Run / ReAct
  POST {base}/v0/tools/{name}/invoke
  头：X-Baize-Protocol、X-Baize-Run-Id、auth 默认头
```

OpenAPI 注册路径不改。未知 `type` → 现有 `unsupported connector type`。

---

## 4. 注册契约

### 4.1 YAML / PUT

```yaml
connector:
  id: legacy-sidecar
  type: http
  base_url: http://127.0.0.1:19090
  require_approval: [create_ticket]
  auth:
    mode: static
    static:
      headers:
        Authorization: "Bearer ${BAIZE_CONNECTOR_TOKEN}"
```

- `type` 缺省 `openapi`；`http` 不要求 `spec`；缺 `base_url` → `400 invalid_request`
- `openapi` 仍要求 `spec`（现有行为）
- `auth` 与 OpenAPI Connector 同一形状，注册时 `ResolveDefaults`

### 4.2 注册时序

1. `GET {base_url}/healthz`，头带 `X-Baize-Protocol: v0`  
   非 200，或 JSON 的 `status` 不是 `ok` → `invalid_plugin`
2. `GET {base_url}/v0/tools`，同上协议头  
   无法解码、缺 `tools`、长度为 0、某项无名 → `invalid_plugin`
3. `WouldConflict` → `409 tool_conflict`，不卸载本 id 之外的工具
4. `UnregisterConnector(id)` 后按 ToolDesc 登记
5. Store `UpsertConnector`（`Type=http`，`Spec` 空，`BaseURL`、`Auth` 形状、`RequireApproval`）

任一步失败不得留下半套 Tools。

### 4.3 GET connector

回显 `id`、`type`、`base_url`、`require_approval`、`auth`（引用形状）、`tools[]`。http 类型无 `spec` 或为空字符串。

### 4.4 配置面限制

`configs/default.yaml` 保持 `type: openapi` + mock-ticket。参考侧车只通过文档中的第二条 PUT（或本地覆盖配置）接入。YAML 仍是单 Connector；多 Connector 并存仅通过多次 PUT（现网已支持 Registry 多 connector）。

---

## 5. 发现与 Invoke

### 5.1 ToolDesc

```json
{
  "name": "create_ticket",
  "description": "创建工单",
  "input_schema": { "type": "object", "properties": {}, "required": [] },
  "annotations": { "dangerous": false, "idempotent": false }
}
```

- `name` 必填  
- `input_schema` 缺省 `{ "type": "object" }`  
- `annotations` 可忽略；GET `/v0/tools`（Runtime）不强制回显 annotations  
- Registry：`Method`/`Path` 空；`OperationID` 可用 name

### 5.2 Invoke

`POST {base_url}/v0/tools/{tool_name}/invoke`

```json
{
  "arguments": { },
  "context": {
    "run_id": "run_...",
    "agent_id": "agent_...",
    "tenant_id": ""
  }
}
```

本轮不发 `callback_urls`。

请求头：`Content-Type: application/json`、`X-Baize-Protocol: v0`、有则 `X-Baize-Run-Id`、再合并 DefaultHeaders（Identity 优先，否则 `auth.mode`）。

响应 200：`{ "content": {}, "is_error": false }`。`is_error: true` 或 HTTP 4xx/5xx / 传输失败 → 对引擎表现为 tool `is_error`（content 尽量带上错误信息），不直接 `UpdateRun(failed)`。

侧车应用错误体 `{ "error": { "code", "message", "retryable" } }`：注册阶段映射 `invalid_plugin`；invoke 阶段并入 tool content。`protocol_unsupported` 同此。本轮不按 `retryable` 重试。

超时：客户端 30s。

`run_id` / `agent_id`：从 context 读取。若 engine 尚未注入 `run_id`，本里程碑在 `injectAuthCtxFromRun`（或等价处）补上 `identity.WithRunID` / `WithAgentID`，供 httpplugin 闭包使用。

---

## 6. 参考侧车 `examples/http-plugin`

独立 `main`，默认 `:19090`。

| 方法 | 行为 |
|------|------|
| `GET /healthz` | `{"status":"ok"}` |
| `GET /v0/tools` | `echo`、`create_ticket`（后者 description 表明会写） |
| `POST /v0/tools/echo/invoke` | 把 arguments 原样放进 content |
| `POST /v0/tools/create_ticket/invoke` | 内存追加工单，返回 `{id, title}` |
| 协议头不是 v0 | `400` + `error.code=protocol_unsupported` |

可选 `GET /tickets` 便于人工确认 HITL 后有副作用，非协议必选。不嵌入 Runtime、不读 OpenAPI。

---

## 7. 错误处理

| 场景 | 期望 |
|------|------|
| `type: http` 无 base_url | `400 invalid_request` |
| healthz / tools 失败或空 | `400 invalid_plugin` |
| 同名跨 Connector | `409 tool_conflict` |
| 侧车 `protocol_unsupported`（注册） | `400 invalid_plugin` |
| invoke 失败 | tool `is_error` |
| 未知 type | 与现网一致（非 openapi/http → 400） |

---

## 8. 测试计划

| 场景 | 断言 |
|------|------|
| 发现 | 假侧车返回两个 ToolDesc → Registry List 含 name + connector_id |
| invoke | POST body 含 arguments 与 context.run_id；请求带协议头 |
| 注册失败 | healthz 挂 / tools=[] → PUT 400，原 Tools 仍在 |
| HITL | require_approval 含 create_ticket → waiting_human |
| 鉴权 | static 默认 Authorization 出现在侧车 request |
| 集成 | 启动 examples 或 httptest 侧车 → PUT → GET /v0/tools → mock Run |

---

## 9. 文档

- README / README.zh-CN：一节说明无 OpenAPI 时用 HTTP 插件；启动参考侧车；PUT 示例；HITL 仍用 `require_approval`
- `docs/architecture-and-plugin-protocol.md` §4.2：注明 Runtime 已实现客户端；`callback_urls` 与 §4.3 仍未实现

---

## 10. 非目标回顾

之后平台应能回答：「没有 Swagger 时，怎么把工具挂进白泽？」  
不承诺 MCP、企业回调、或开箱改成侧车默认。操作员向样板 / 轨迹可视化仍是下一候选。

---

*本文档经头脑风暴分节批准后落盘；实现前若字段名有微调，以本文语义为准并更新本文，不静默漂移。*
