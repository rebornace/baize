# Connector 整删与侧车 callback_urls v0 设计规格

> 状态：已批准（2026-08-26）  
> 日期：2026-08-26  
> 前置：HTTP 插件协议客户端、企业执行回调、OpenAPI/插件/MCP 设置页、工具目录 DELETE tool 行已落地  
> 依据：架构 §4.2「callback_urls 尚未实现」；openapi-settings「不做整 Connector DELETE」；头脑风暴选定 B→A 两阶段  
> **后续：** MCP / 企业 execution_callback 路径注入见 `2026-08-27-callback-urls-extension-v0-design.md`  
> 分支建议：`feat/connector-delete-callback-urls`

---

## 1. 目标与成功标准

**目标：** 管理员可完整删除 Connector（级联工具）；HTTP 侧车 invoke 可收到 Runtime 反向 `callback_urls.event`，并用短期 token 把事件写回对应 Run 流——补齐设置页生命周期与插件协议 v0 异步钩子，且不强迫业务系统嵌 SDK。

**一句话成功标准：** 设置页二次确认后 Connector 与工具目录无残留；侧车用注入的 URL 回投后，该 Run 的 SSE/事件列表出现 `plugin.callback`。

### 做（两阶段，同一规格）

| 阶段 | 内容 |
|------|------|
| **Phase 1（B）** | `DELETE /v0/connectors/{id}`；级联 Store tools + connector；`Registry.UnregisterConnector`；OpenAPI / HTTP 插件 / MCP 设置页删除 + 二次确认 |
| **Phase 2（A）** | HTTP 插件 invoke 注入 `context.callback_urls.event`；`POST /v0/runs/{run_id}/plugin-callbacks`；HMAC 短期 token；事件入 Store；配置 `runtime.public_base_url` |

### 不做

- TS / Python SDK、Webhook 重试/死信、PDF 页图 vision、专用进度条 UI  
- MCP / OpenAPI / 企业 `execution_callback_url` 路径注入 `callback_urls`（可另里程碑）  
- Connector 软删；因「有活跃 Run」返回 409 禁删  
- 回调改变 Run 状态机或自动结束 tool turn  
- 把 Admin API Key 交给侧车做回调鉴权  

### 成功标准（可测）

1. Phase 1：删后 `GET /v0/connectors/{id}` → 404；工具目录无该 `connector_id`；再 invoke 已删工具名失败  
2. Phase 2：invoke body 含签名 `callback_urls.event`；合法 POST → Run 事件含 `plugin.callback`；坏/过期 token → 401，Store 无新事件  
3. 未配置可用 `public_base_url`（生产）时不注入回调 URL（避免把不可达地址发给侧车）

---

## 2. Phase 1：整删 Connector

### 2.1 API

- `DELETE /v0/connectors/{id}`  
- ACL：**Admin**（与 `PUT /v0/connectors/{id}` 同级）  
- 不存在 → `404` `connector_not_found`  
- 成功：与现有 `DELETE .../tools/{name}` 对齐（优先 `204 No Content`；若仓库惯例为 JSON ok 则一致即可）  
- 无 `force` query；无软删  

### 2.2 服务端顺序

1. Store 确认 Connector 存在，否则 404  
2. `Registry.UnregisterConnector(id)`（已有）  
3. 同一事务：删除该 id 下全部 tool 行 + connector 行  
4. **不**取消进行中的 Run；之后 invoke 走「工具不存在」既有路径  

### 2.3 UI

- `OpenApiSettings` / `PluginSettings` / `McpSettings` 列表行增加「删除」  
- 二次确认文案包含 **connector id**，并说明将移除其全部工具  
- 取消不发请求；成功后刷新 Connector 列表与工具相关视图  
- 前端 `api.ts` 增加 `deleteConnector(id)`  

### 2.4 测试

- API：删除级联、404、ACL（Operator 403）  
- UI/单测：确认取消不调用 DELETE；确认后调用正确路径  

---

## 3. Phase 2：侧车 `callback_urls`

### 3.1 注入（Runtime → 侧车）

仅 **HTTP 插件**（`internal/connector/httpplugin`）在 `POST .../invoke` 的 body：

```json
{
  "arguments": {},
  "context": {
    "run_id": "run_...",
    "agent_id": "agent_...",
    "callback_urls": {
      "event": "{public_base}/v0/runs/{run_id}/plugin-callbacks?token=..."
    }
  }
}
```

规则：

- 无 `run_id` → **省略**整个 `callback_urls`  
- 无法得到可用 `public_base` → **省略**并记日志（不注入 `localhost` 给可能远端的侧车）  
- OpenAPI / MCP / 企业执行回调路径本里程碑不注入  

### 3.2 `public_base`

- 配置：`runtime.public_base_url`（字符串，无尾斜杠；例 `https://baize.internal:8080`）  
- 开发：若未配置，可用 Listen 推导本机 base（与现有本地 curl hint 同类），仅便于本机侧车  
- 生产建议：显式配置；未配置则不注入  

### 3.3 鉴权

- `token`：HMAC（或等价）短期凭证，至少绑定 `run_id`，可选绑定 `connector_id` / tool 名  
- TTL 默认 **1 小时**（可配置）  
- 密钥：配置项或进程启动生成的密钥；**禁止**复用并下发 Admin API Key  
- 回调路由：**不**要求 Operator 浏览器会话；只校验 token  
- 失败：`401`（坏签/过期）；run 不存在：`404`；禁止用 A run 的 token 写 B run  

### 3.4 回调契约

```http
POST /v0/runs/{run_id}/plugin-callbacks?token=...
Content-Type: application/json
X-Baize-Protocol: v0

{
  "type": "event",
  "name": "sidecar.note",
  "payload": {}
}
```

- Runtime `AppendEvent`：事件类型 **`plugin.callback`**，内容包含侧车 `name` + `payload`（及必要元数据）  
- 成功：`204`  
- `payload` JSON 大小上限建议 **64 KiB**；超限 `413` 或 `400`  
- 每 `run_id` 简单限流（建议默认 **100 次 / 小时**）→ `429`  
- **不**因此结束 tool、不改 Run 终态；仅进入事件流，现有 SSE/历史折叠可见  
- v0 **无**专用 progress 字段；侧车用 `name` / `payload` 自描述  

### 3.5 测试与文档

- 单测：注入含 token；合法回调入事件；错 token / 错 run / 过期拒绝；超限 payload  
- 更新 `docs/architecture-and-plugin-protocol.md` §4.2：标明 Runtime 已注入并接收回调  
- README / 配置示例：`runtime.public_base_url` 一句说明  
- 可选：示例侧车或集成测演示一次回调（非必须阻塞）  

---

## 4. 实现顺序与风险

### 4.1 顺序

1. Phase 1 全绿（API + Store + UI + 测）并单独可验收  
2. 再开 Phase 2（配置 + token + 注入 + 回调端点 + 测 + 架构文档）  

同一实现计划分任务编号，审查按阶段关卡。

### 4.2 风险

| 风险 | 缓解 |
|------|------|
| 误发不可达 URL | 无可靠 `public_base` 则不注入 |
| token 泄露写事件 | 短 TTL、绑定 run、payload 限大小、限流 |
| 删 Connector 后在途 Run | 文档说明；invoke 失败即可 |
| 三设置页漏删 | 共用 API；三页均加按钮与确认 |

---

## 5. 规格自检

- 无 TBD/占位符实现路径  
- Phase 1/2 边界与「不做」无矛盾  
- 与已实现企业回调（Runtime→企业）方向相反，文中已区分  
- 事件类型名 `plugin.callback`、配置键 `runtime.public_base_url` 已写死，计划勿另起别名除非改规格  

---

*审查本文件后回复批准或修改意见；批准后进入 writing-plans。*
