# Run 事件 Webhook v0 设计规格

> 状态：已批准（2026-08-23，用户要求全功能 UI 可操作）  
> 日期：2026-08-23  
> 前置：SSE、eventbus 已落地  
> 依据：架构 §3 Webhook（未实现）

---

## 1. 目标与成功标准

**目标：** 平台通过 HTTP POST 接收 Run 轨迹；**管理员与操作员均通过 UI 配置**，不依赖 curl/YAML 手改（YAML 仍可作为启动默认值）。

**成功标准：**

1. **设置 → Webhook**（admin）：配置全局 `url` + `headers`（`Key=Value` 多行），保存持久化；GET 回显引用形状不展开明文
2. **「发送测试事件」**按钮：向已配置 URL POST `webhook.test`，UI 展示成功/失败
3. **聊天页高级选项**（可选折叠）：本次 Run 可填覆盖 `webhook_url`（空则用全局）
4. `POST /v0/runs` 仍支持 API 字段（与 UI 同源）
5. 每次 event 异步 POST；终态 `run.ended`；不阻塞引擎
6. 集成测试 + UI 测试；`npm run build` 提交 dist

**不做：** 多订阅、重试队列、企业回调 §4.3、侧车 callback_urls

---

## 2. 配置与持久化

**启动：** `configs/*.yaml` 的 `events.webhook` 作为**初始默认**（与 connector 类似）。

**运行时：** SQLite `settings` 表 key `events_webhook` 存 UI 保存值；**优先于** yaml 空值；PUT 后即时生效（Dispatcher 热更新）。

```yaml
events:
  webhook:
    url: ""
    headers: {}
```

---

## 3. API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v0/settings/events-webhook` | admin；`{ url, headers }` |
| PUT | `/v0/settings/events-webhook` | admin；持久化 |
| POST | `/v0/settings/events-webhook/test` | admin；发送测试包 |

**POST /v0/runs** 扩展：`webhook_url`、`webhook_headers` → `runs.webhook_json`

**投递 body（事件）：**

```json
{ "run_id": "...", "index": 2, "event": { "type": "...", "timestamp": "...", "data": {} } }
```

**终态：**

```json
{ "run_id": "...", "ended": true, "status": "succeeded" }
```

头：`Content-Type: application/json`、`X-Baize-Run-Id`、`X-Baize-Protocol: v0`

`GET /v0/runs/{id}` **不回显** webhook URL。

---

## 4. UI

| 位置 | 内容 |
|------|------|
| `/settings/webhooks` | 全局 URL、headers、保存、测试投递、说明（与 SSE 并列） |
| `settingsNav` | admin 导航增加「Webhook」 |
| `ChatPage` | 折叠「高级」：本次 Run webhook URL（可选） |

对称 `McpSettings` / `PluginSettings` 样式与 `AdminOnly`。

---

## 5. 实现组件

| 组件 | 职责 |
|------|------|
| `store` | `settings` KV；`runs.webhook_json` |
| `internal/webhook` | Dispatcher 挂 Hub；异步 POST |
| `internal/api` | settings CRUD + test + runs 字段 |
| `bootstrap` | yaml 默认 + store 覆盖；Dispatcher 注入 |

---

## 6. 测试与文档

- `internal/webhook/*_test.go`、集成测、Settings API 测
- `WebhookSettings.test.ts`
- 架构 §3、README 标 UI 路径

---

*企业执行回调 §4.3 仍为下一 P1。*
