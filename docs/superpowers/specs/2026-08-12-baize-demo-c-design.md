# Baize Demo C 设计规格：OpenAPI 接入契约

> 状态：已批准（头脑风暴）  
> 日期：2026-08-12  
> 前置：Demo A（OpenAPI → Tool → Run）、Demo B（Chat + HITL）已完成  
> 依据：`docs/architecture-and-plugin-protocol.md`、本轮头脑风暴（先做平台「接入契约」）

---

## 0. 产品叙事（为何要有 Demo C）

**白泽与老系统的交互模型（已锁定）：** Runtime 作为侧车/网关，**不改老系统内核**；主路径是 **OpenAPI → Tool → Runtime 直接 HTTP 调用遗留 API**；危险操作可 HITL；全程 Run 可审计。侧车插件协议 / MCP / 企业回调为扩展，非本里程碑。

Demo A/B 证明了链路机械可行，但对方是玩具 mock、缺少「平台怎么挂系统」的可见契约，故观感不像企业落地。

**受众优先级：** 操作员与平台都要；**下一里程碑先做深平台侧（B）**，且平台侧先做 **接入契约（A）**，鉴权与审计后补。

---

## 1. 目标与成功标准

**目标：** 把「导入遗留 OpenAPI → 注册 Connector → 查询 Tool 清单 → Run 打到该 Connector 的 base_url」做成平台可演示、可文档化的闭环；无需改 Runtime 代码即可换 Spec 重挂。

**一句话成功标准：** 集成方按文档执行：准备 Spec → `PUT /v0/connectors/{id}` → `GET /v0/tools` 看到带 method/path/operation_id 的清单 → `POST /v0/runs`（或 `/ui`）完成一次调用；换 Spec 再 PUT 后 Tool 列表正确替换；坏 Spec 返回 `400` 且不污染已有 Registry。

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| 控制面 | `PUT /v0/connectors/{id}` 硬化（响应含 `tools[]` 摘要；支持 `require_approval`）；**新增** `GET /v0/connectors/{id}`、`GET /v0/tools` |
| 映射元数据 | Tool 暴露 `connector_id`、`operation_id`、`method`、`path`（及既有 name/description/schema） |
| 冲突与替换 | 同 id 再 PUT → **替换**该 Connector 旧 Tools；跨 Connector **同名 Tool → `409 tool_conflict`** |
| 样板 | 加重 `examples/mock-ticket`（至少增加按 id 查询、改状态之一）；OpenAPI 含明确 `operationId` 与错误响应示意 |
| 文档 | 平台「接入三步」写入 README（或 `docs/` 短文）：Spec → PUT Connector → GET Tools → Run |
| UI | `/ui` **只读 Tools 面板**（列出当前已注册 Tools；非画布、非编辑器） |
| 测试 | API 单测 + 集成：列 Tools、替换、坏 Spec、Run 事件可对应 Tool |

### 不做

- 侧车插件协议 v0、MCP 桥、企业执行回调  
- Token 托管 / SSO / 多租户鉴权深做（`auth` 可保留现有透传/none，不扩展为里程碑主题）  
- OTel、合规审计加固、SSE/Webhook  
- Agent 绑定 Tool 子集（本里程碑 Registry **全量** Tools）  
- 工作流 DSL、多 Agent、桌面 App、真实客户生产系统对接  
- 操作员叙事深挖（下一波可再做「更真业务故事」；本里程碑主交付给平台）

---

## 3. 架构与数据流

```
集成方
  │ 1. 遗留 OpenAPI + base_url
  │ 2. PUT /v0/connectors/{id}
  ▼
Baize Runtime
  OpenAPI Loader → Tool[] → Registry
  Connector 元数据 → Store
  │ 3. GET /v0/connectors/{id} · GET /v0/tools
  ▼
平台可见 Tool 清单（operation / method / path）
  │ 4. POST /v0/runs 或 /ui Chat
  ▼
LLM 选 Tool → HTTP → Connector.base_url（仿真或真实遗留）
  │ 5. GET /v0/runs/{id}/events
```

**不变：** 单进程 Runtime + 内建 OpenAPI Connector；不引入新进程形态。

**相对 Demo A/B：** 写路径（PUT + demo YAML 注册）已有；本里程碑补齐**查询面、映射元数据、替换/冲突语义、样板与文档、UI 可见性**。

---

## 4. 控制面契约

### 4.1 `PUT /v0/connectors/{id}`

请求体（字段以实现为准，语义如下）：

```json
{
  "type": "openapi",
  "spec": "<文件系统路径，与现网一致>",
  "base_url": "http://127.0.0.1:18080",
  "require_approval": ["create_ticket"]
}
```

- `type` 缺省 `openapi`；非 openapi → `400`  
- Spec 无法解析 / 无可用 operation → `400 invalid_spec`（**不得**部分注册）  
- 成功：写入 Connector；注册 Tools；**响应**包含该 Connector 的 `tools[]` 摘要（至少 name + method + path + operation_id）  
- 再次 PUT 同 `id`：先移除该 Connector 名下旧 Tools，再注册新 Tools（已有测试语义延续并文档化）

### 4.2 `GET /v0/connectors/{id}`（新增）

返回 Connector 元数据 + tool 名列表（或内嵌摘要）。不存在 → `404`。

### 4.3 `GET /v0/tools`（新增）

返回当前 Registry 全量 Tools，每项至少：

| 字段 | 说明 |
|------|------|
| `name` | Tool 名（优先 operationId） |
| `description` | 描述 |
| `connector_id` | 所属 Connector |
| `operation_id` | OpenAPI operationId（若有） |
| `method` | HTTP 方法 |
| `path` | OpenAPI path |
| `input_schema` | JSON Schema（可与现有一致） |

### 4.4 同名冲突

若新注册 Tool 的 `name` 已属于**另一** Connector → **`409 tool_conflict`**，本次 PUT 整体失败、不改 Registry。  
（不采用静默前缀；保持名称稳定、冲突显式。）

### 4.5 Agent

`PUT /v0/agents/{id}` 保持现状。本里程碑不实现 Agent↔Tool 白名单；引擎继续使用 Registry 全量（或现有等价行为）。

---

## 5. 样板、文档与 UI

### 5.1 mock-ticket

在现有 create/list 基础上至少扩展：

- 按 id 查询工单，和/或  
- 更新工单状态  

OpenAPI 必须带稳定 `operationId`，并对 4xx/5xx 有示意（不必实现全部错误分支，但 Spec 要像遗留系统）。

### 5.2 文档

开源可读的「平台接入」短路径（README 一节或独立短文），步骤固定为：

1. 准备 OpenAPI + 可达的 `base_url`  
2. `PUT /v0/connectors/{id}`  
3. `GET /v0/tools` 核对  
4. 配置/确认 Agent 后 `POST /v0/runs` 或打开 `/ui`

明确说明：主路径是 OpenAPI；无 Swagger 的系统走后续侧车里程碑。

### 5.3 `/ui` Tools 面板

- 只读列表：name、method、path、connector_id  
- 不提供编辑 Spec、不提供画布  
- 数据来源：`GET /v0/tools`（轮询或进入页面时拉取一次即可）

---

## 6. 错误处理与测试

| 场景 | 期望 |
|------|------|
| 坏 Spec | `400 invalid_spec`；既有 Tools 不变 |
| 跨 Connector 同名 | `409 tool_conflict`；既有 Tools 不变 |
| 同 id 替换 | 旧 Tools 消失，新 Tools 可见 |
| 冷启动 demo | `GET /v0/tools` 非空且含 path/method |
| Run | events 中 tool 名可与 `GET /v0/tools` 对应 |

测试：`internal/api` 单测覆盖 GET/冲突/坏 Spec；集成测试覆盖 demo 装配后的列 Tools + 一次 Run（可用 mock LLM）。

---

## 7. 非目标回顾与后续

本里程碑交付后，平台应能回答：「我们怎么把老系统的 HTTP API 挂进白泽？」  
**不**承诺鉴权深做、侧车协议、或操作员侧「真业务感」叙事——这些列为后续候选：

1. 鉴权与凭证（透传/static/vault）  
2. 侧车插件协议 v0（无干净 OpenAPI 时）  
3. 操作员向：更重的业务样板 / 轨迹可视化  

---

## 8. 实现提示（非绑定）

- Store 需能按 `connector_id` 列出/删除 Tools 元数据；若现仅内存 map，扩展结构以支撑 GET 与冲突检测。  
- OpenAPI loader 在 Load 时一并返回 method/path/operationId。  
- Chat UI 增量：一小块 Tools 面板；避免大改对话主路径。

---

*本文档经头脑风暴分节批准后落盘；实现前若字段名有微调，以本文语义为准并更新本文，不静默漂移。*
