# Webhook 出站重试 v0 设计规格

> 状态：已批准（2026-08-28）  
> 日期：2026-08-28  
> 前置：Run Webhook v0、Webhook Inbox v1  
> 依据：头脑风暴（进程内退避 + SQLite 死信 + 设置页重投；固定重试策略）

---

## 1. 目标与成功标准

**目标：** 出站 Run 事件 Webhook 在下游短暂不可用时自动重试；最终失败进入死信并可从 **设置 → Webhook** 查看与手动重投。

**成功标准：**

1. **5xx / 网络错误 / 429** → 自动重试；其他 **4xx** → 不重试，直接死信
2. 固定策略：**最多 5 次 POST（含首次）**；退避 **1s → 2s → 4s → 8s → 16s**，单次间隔上限 **60s**
3. SQLite `webhook_outbox` 持久化；进程重启后 pending 可续投递
4. Admin API + UI：列出最近 `dead`/`pending`（默认各 50 条内），手动 **重投**
5. 集成测：mock 503×2 → 200 成功；永久失败 → `dead` 行
6. 仍 **不做**：多订阅、外部队列、Settings 可调 retry 参数

**不变：** SSE payload 形状、per-run webhook 覆盖、Inbox 入站路径。

---

## 2. 数据模型

表 `webhook_outbox`：

| 字段 | 说明 |
|------|------|
| `id` | UUID |
| `delivery_key` | UNIQUE：`{run_id}:{kind}:{event_index}`（`ended` 用 index=-1） |
| `run_id` | 关联 Run（test 为 `test`） |
| `kind` | `event` \| `ended` |
| `event_index` | event 序号；ended 为 -1 |
| `payload_json` | POST body |
| `target_url` | 解析后 URL |
| `headers_json` | 已 resolve 的请求头 |
| `attempt` | 已完成 POST 次数（0 起，失败后递增） |
| `max_attempts` | 固定 5 |
| `status` | `pending` \| `delivered` \| `dead` |
| `last_error` | 最近错误摘要 |
| `next_retry_at` | 下次投递 UTC |
| `created_at` / `updated_at` | 审计 |

索引：`(status, next_retry_at)`。

---

## 3. 投递与 Worker

```
Hub → Dispatcher.Enqueue → INSERT outbox (pending, next_retry_at=now) → wake worker
Worker（1s tick + wake）→ ListDue pending → POST → 更新状态/退避
```

- 首次入队后 worker 立即尝试投递
- **手动重投：** `attempt=0`、`status=pending`、`next_retry_at=now`
- 终态 **dead** 且 `run_id != test` 时 append Run event `webhook.delivery_dead`（摘要，无 URL/secret）

---

## 4. API（admin）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v0/settings/events-webhook/deliveries?status=&limit=` | 默认 `status=dead,pending`；不含 headers 明文 |
| POST | `/v0/settings/events-webhook/deliveries/{id}/retry` | 手动重投 |

---

## 5. UI

`WebhookSettings` 增加 **最近投递** 表格 + **重投** 按钮；样式对齐现有 Webhook 页。

---

## 6. 测试与文档

- `internal/webhook` 单测：退避、可重试判定、worker
- `internal/store` outbox round-trip
- `tests/integration/webhook_retry_test.go`
- `WebhookSettings.test.ts`
- 更新 architecture §3、README；Inbox v1 defer「出站重试」标注已实现

---

*取代 `run-webhook-v0` 中「不做重试队列」的 defer 项（v0 仍无多订阅）。*
