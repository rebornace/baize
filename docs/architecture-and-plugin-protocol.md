# Baize（白泽）架构与插件协议草案（v0）

> 面向企业遗留系统的轻量可插拔 Agent Runtime。  
> 状态：设计草案 · 插件协议 **v0（实验）** · 许可证 MIT（插件协议 / 商标另规）

---

## 1. 一句话与边界

**Baize** 是独立进程 Runtime（侧车 / 网关），把 LLM、Tool、遗留 HTTP API、人机卡点收成可审计的 `Run`。  
平台团队用 REST / SDK 嵌入；集成商按同一产物交付。

| 做 | 不做（v1） |
|----|------------|
| OpenAPI → Tool、执行回调、HITL、可选工作流 DSL | 桌面 App、个人 IM 全家桶、内置自我进化、人生记忆、重画布 |
| 无必选 Redis/PG/K8s | 绑死单一云或单一 Agent 框架 |

**五抽象：** `Runtime` · `Agent` · `Tool` · `Connector` · `Run`  
（Skill / Memory / Channel / Workflow 均为配置或插件形态，不升格。）

---

## 2. 逻辑架构

```
┌─────────────────────────────────────────────────────────────┐
│                     企业平台 / 集成方                          │
│         REST 控制面 · SSE/Webhook 事件 · TS/Python SDK        │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────┐
│                    Baize Runtime (Go)                         │
│  Agent 定义 │ LLM 适配(OpenAI-compatible / 仅网关开关)         │
│  Tool 注册  │ Run 引擎(ReAct 默认 · 可选状态机) · HITL        │
│  Connector  │ 轨迹/Checkpoint · 结构化日志 · 可选 OTel        │
└───────┬─────────────┬─────────────┬─────────────┬───────────┘
        │             │             │             │
   OpenAPI 通配    HTTP 插件v0    MCP 桥(v0)     企业回调
        │             │             │             │
        ▼             ▼             ▼             ▼
   遗留 REST API   侧车 Connector   MCP Server   webhook/gRPC-HTTP
```

**MCP 桥（v0 已实现）**：`PUT /v0/connectors/{id}` 注册 `type: mcp` Connector（stdio 子进程或 Streamable HTTP）；`tools/list` 发现工具写入目录 `source=mcp`，Run 内 `tools/call` 执行；与 OpenAPI / HTTP 插件并列，不替代插件协议 v0。

**默认存储：** SQLite（Run / HITL / checkpoint）；可换 PG。短 Run 可开无状态模式。  
**租户：** Schema 含软 `tenant_id`；开源默认单租户用法。  
**凭证：** 默认透传调用方 Token；可选本地加密托管，接口可换 KMS。

---

## 3. 控制面（平台主集成）

| 能力 | 方法（示意） |
|------|----------------|
| 注册/更新 Agent、Connector | `PUT /v0/agents/{id}` · `PUT /v0/connectors/{id}` |
| 查询 Connector | `GET /v0/connectors/{id}` |
| 查询工具目录（含停用行） | `GET /v0/tools` |
| 启停 / 改 `require_login` / `title` / `description` | `PATCH /v0/tools/{name}` |
| 手加 REST 工具（仅 OpenAPI Connector） | `POST /v0/connectors/{id}/tools` |
| 删除手加行（仅 `source=extra`） | `DELETE /v0/connectors/{id}/tools/{name}` |
| 启动 Run | `POST /v0/runs` → `{ run_id }` |
| 查询轨迹 | `GET /v0/runs/{id}` · `GET /v0/runs/{id}/events` |
| 恢复 HITL | `POST /v0/runs/{id}/resume` |
| 事件推送 | SSE `GET /v0/runs/{id}/stream`（已实现）· Webhook 出站（已实现；UI：**设置 → Webhook**） |

`Run` 状态机（最小）：`queued` → `running` → (`waiting_human` ↔ `running`) → `succeeded` | `failed` | `cancelled`。

可选控制面口令（`control_plane.operator_token` / `admin_token`）：挡住 Runtime 的 `/v0`。操作员可跑 Run / HITL / 会话身份；管理员可改 Agent、Connector、Tools。不是下游业务 IAM，也不是多租户 SSO。

#### 工具目录与 Registry 的关系

- **`store.Tool` 是 Connector 目录行**（`connector_id` / `name` / `source` / `enabled` / `title` / `description` / `description_custom` / `method` / `path` / `input_schema` / `require_login` / `require_approval` / `operation_id`）。`title` 仅供设置页展示，不进模型 tool list。Memory 与 SQLite 同一接口；SQLite 下 `connectors` / `tools` 两表落盘，重启按 §5 合并恢复。
- **`source` 四种**：`spec`（OpenAPI operation）、`plugin`（侧车 `GET /v0/tools` 发现）、`mcp`（MCP `tools/list` 发现）、`extra`（管理员手加 REST）。`spec` / `plugin` / `mcp` 可启停不可删；`extra` 可启停也可删。
- **Registry 仅注册 `enabled = true` 的行**。引擎、HITL、登录门闸继续读 Registry，不直接扫目录；`GET /v0/tools` 改为读目录，因此能看见停用行。
- **Agent 不绑定 Connector 子集**：每次 Run 把 Registry 中全部启用工具交给模型（跨 Connector 工具名全局唯一，冲突拒绝）。
- **合并规则（PUT 与开机相同）**：发现列表里新出现的 `spec`/`plugin` 名插入并默认启用；仍在的同名保留 `enabled`、行上门闸与 `title`；`description_custom=true` 时说明保留，否则用发现侧说明覆盖；若本次 PUT/YAML 显式带了 `require_login` / `require_approval` 数组则按名单重写；消失的 `spec`/`plugin` 行删除；`extra` 行除非 DELETE 否则保留；省略目录字段则保留行上已有值。Apply 失败时该次拟写入回滚，Registry 保持失败前状态。

---

## 4. 插件协议 v0（HTTP + JSON）

侧车 Connector 与 Runtime **同网 HTTP 互通**；OpenAPI 通配 Connector 由 Runtime 内建，不走本协议。  
MCP 桥（v0 已实现）：MCP Tool 映射为内部 `Tool`（`source=mcp`），不替代本协议。

### 4.1 约定

- Base URL 由 Connector 注册时声明；超时 / 重试由 Runtime 配置。  
- 请求头：`Authorization`（透传或 Runtime 注入）、`X-Baize-Run-Id`、`X-Baize-Tenant-Id`（可空）、`X-Baize-Protocol: v0`。  
- 错误体：`{ "error": { "code": "string", "message": "string", "retryable": bool } }`。

### 4.2 侧车必须实现

```http
GET  /healthz                          → 200 { "status": "ok" }
GET  /v0/tools                         → { "tools": [ ToolDesc, ... ] }
POST /v0/tools/{tool_name}/invoke      → ToolResult
```

**ToolDesc（摘要）：**

```json
{
  "name": "create_ticket",
  "description": "创建工单",
  "input_schema": { "type": "object", "properties": {}, "required": [] },
  "annotations": { "dangerous": false, "idempotent": false }
}
```

**Invoke 请求 / 响应：**

```json
// POST body
{
  "arguments": { },
  "context": {
    "run_id": "run_...",
    "agent_id": "agent_...",
    "tenant_id": "",
    "callback_urls": { "event": "https://runtime/.../v0/runs/{run_id}/plugin-callbacks?token=..." }
  }
}

// 200
{ "content": { }, "is_error": false }
```

Runtime 已实现该协议的客户端。当配置了 `runtime.public_base_url`（及 HMAC 密钥）且 invoke 带 `run_id` 时，会注入短期签名的 `callback_urls.event`；侧车可 `POST` 该 URL 写入 Run 事件流（类型 `plugin.callback`）。未配置 `public_base_url` 则不注入。MCP `tools/call` 通过 `_meta.io.baize/callback_urls` 注入同一 URL 与 token；企业 execution_callback POST body 的 `callback_urls` 亦使用同一签发逻辑。详见规格 `docs/superpowers/specs/2026-08-26-connector-delete-and-callback-urls-v0-design.md` 与 `docs/superpowers/specs/2026-08-27-callback-urls-extension-v0-design.md`。

### 4.3 企业执行回调（补充路径，v0 已实现）

Runtime 作为客户端调用企业提供的 endpoint（注册在 Connector 配置 `execution_callback_url` 中）：

```http
POST {enterprise_callback_url}
X-Baize-Protocol: v0
{
  "tool": "approve_payment",
  "arguments": { },
  "run_id": "run_...",
  "idempotency_key": "...",
  "callback_urls": { "event": "https://runtime/.../v0/runs/{run_id}/plugin-callbacks?token=..." }
}
```

与侧车 `/invoke` 语义对齐；企业无需实现 `/v0/tools` 发现（工具清单由配置或 OpenAPI 提供）。`callback_urls.event` 为可选字段：仅当 invoke 带 `run_id` 且 Runtime 配置了 `public_base_url` 时注入。

### 4.4 内建 OpenAPI Connector

1. 导入 OpenAPI 3.x → 每 operation → 一个 `Tool`（名优先 `operationId`）。
2. 每次工具 invoke 的凭证优先级：
   1. 强制 `identity_id`（若有且存在且未过期）
   2. 会话内未过期、scheme 匹配的 Identity（无 `security` 时与现逻辑相同：活跃身份里按默认 / 最近使用挑选）
   3. **仅当 Run 没有 `conversation_id`：** Connector 默认 Headers（`static` / `passthrough` / `vault_ref`）
   4. 空头

   带 `conversation_id` 时**不用** Connector 默认头。「需要登录」是操作员开关（`require_login`），不是从 OpenAPI `security` 推断；默认公开。有会话且工具需要登录、第 1–2 步又无可用凭证时，不发下游 HTTP。完整凭证不出现在 events 与 GET run。
3. 覆盖不了的遗留逻辑 → 侧车插件或执行回调。

HTTP 插件侧车在 `POST /v0/tools/{name}/invoke` 成功返回后，亦可按 Connector 的 `auth.capture` 将工具结果写入会话身份（配置与 OpenAPI Connector 相同）。

### 4.5 版本策略

- 头与路径中的 `v0` 表示实验协议；破坏性变更增至 `v1` 并保留迁移说明。  
- 外部插件应以 `X-Baize-Protocol` 协商；未知主版本 → `400 protocol_unsupported`。

---

## 5. Agent 运行与扩展形态

| 模式 | 行为 |
|------|------|
| 默认 | 单 Agent ReAct：模型选 Tool → 执行 → 写轨迹 → 直至结束 |
| 可选 | YAML / 等价 JSON 状态机：`step` / `tool` / `branch` / `wait_human` |
| 多 Agent | 多个 Agent 配置 + Run 间消息（非默认） |
| Skill 包 | **配置形态**（不升格为与 Runtime / Agent / Tool / Connector / Run 并列的第六抽象）：`SKILL.md` 流程正文 + `tools` 清单；无在线自闭环、无市场 |
| Memory | Run 工作记忆内置；企业 Memory 插件默认关 |
| Channel 参考 | **HTTP Webhook Inbox v1（已实现）** — `POST /v0/inbox/{channel_id}` + HMAC 验签，见 [README 生产集成](../README.zh-CN.md#生产集成webhook-inbox) 与 [`examples/inbox-alert/`](../examples/inbox-alert/)；企业微信（样板，非内核概念） |

### Skill：发现与激活（语义）

- **发现（目录段）**：安装集 = 扫描 `skills.builtin_dir` ∪ `skills.user_dir`（同 id：user 覆盖 builtin）。Run 组装 system 时，若安装集非空，追加固定「Available skills」目录段（已安装 id + description），供模型发现可激活包。
- **默认激活**：`agent.skills` 为初始激活集；对应正文以 `## Skill: {id}` 注入 system。空数组 = 无默认正文，且可见工具 = 全部目录已启用（与未挂 Skill 时一致）。
- **激活（Run overlay）**：内置工具 `activate_skill` 仅扩大**本 Run** 的激活集与可见工具并集；新 Run 从 Agent 默认 `skills` 重开。可见工具始终与工具目录 `enabled` **求交**，不能启用已停用行。
- **配置边界**：Skill 不改变 Connector / 工具目录启停、HITL、会话身份语义；上传删除走控制面 API，正文不在线编辑。

### 线性流水线（workflow.yaml）

Skill 包目录内可选 `workflow.yaml`（与 `SKILL.md` 并列）。激活该 Skill 且包内存在合法 workflow 时，Run 进入**线性流水线模式**：按 `steps[]` 顺序调用已注册工具，**不经过 LLM 逐步决策**；全部步骤成功则 Run 直接 `succeeded`（无总结性 `llm.message`）。

| 字段 | 含义 |
|------|------|
| `name` | 工作流名（通常与 Skill id 一致） |
| `steps[].id` | 步骤 id（全局唯一，供模板引用） |
| `steps[].tool` | Registry 中工具名 |
| `steps[].args` | 可选；值支持 `{{路径}}` 模板 |
| `steps[].approve` | 可选 `true` → 该步走既有 HITL 门 |

**模板（v0）：** 仅 `{{dot.path}}`。Run 输入在树中为 `input.text`（用户字符串）；已完成步骤输出为 `<step_id>.result`（工具返回的 content map）。例：`{{input.text}}`、`{{list.result.items}}`。

**与 ReAct 的分工：** 需要条件分支、动态选工具或 LLM 编排的场景**不要**写 workflow，保留 `SKILL.md` 正文 + ReAct；workflow 只覆盖「步骤与参数已确定、仅需顺序执行 + 可选审批」的路径。事件：`workflow.started` / `workflow.step_started` / `workflow.step_completed`（SSE 与 `/events` 同通道）。

---

## 6. 开源 / 商业切分（提醒）

**开源：** Runtime 内核、OpenAPI Connector、HITL、工作流 DSL、插件协议 v0、参考 Channel、TS/Python SDK。  
**商业：** 多租户/SSO/审计加固、厂商连接器、可视化 UI、托管记忆/进化（若做）。

---

## 7. 首个验收故事

1. 导入模拟工单 OpenAPI → Agent 查询/创建工单（README 30 分钟）。  
2. **文档第二页（HITL）：** 拟稿 → `waiting_human` → `POST .../resume` → 回调写回业务系统；运营入口见 `/ui`（Chat），规格见 [Demo B 设计](superpowers/specs/2026-08-12-baize-demo-b-design.md)。

---

*本文是 grilling 共享理解的工程落盘；实现前若协议字段有增删，只改本文 v0 并记变更，不静默漂移。*
