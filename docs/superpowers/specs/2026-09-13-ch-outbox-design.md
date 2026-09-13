# CH-OUTBOX：渠道出站持久化 Outbox

- 日期：2026-09-13
- 状态：已交付（2026-09-13）
- 归属：开源首版史诗 **CH-OUTBOX**；确认清单「渠道 outbox」；接在 **CH-PORT** 之后
- 承接：`2026-09-06-phase2-webhook-channel-design.md` §8「不做持久化 outbox」后续增强；Run 事件 outbox 见 `2026-08-28-webhook-outbound-retry-v0-design.md`
- 产品决策来源：`docs/superpowers/notes/2026-09-13-v1-product-confirmation-checklist.md`
- 实现计划：`docs/superpowers/plans/2026-09-13-ch-outbox.md`

## 1. 目标 / 非目标

### 1.1 目标

1. **跨重启可靠**：渠道 `SendText` / `SendMedia` 先写入持久化 `channel_outbox`；baize 重启后 pending 可续投到适配器。
2. **适配器长时间不可用**：指数退避重试；耗尽或不可重试错误进入死信（`dead`）。
3. **管理面**：Admin API + **设置 → 消息 → 微信** 页可查看 pending/dead 并手动重投。
4. **媒体不撑库**：媒体字节先落 `blob.Store`；outbox 行只存 blob key 与元数据；POST 时再组装 `ContentBase64`。

### 1.2 非目标

- 多副本 outbox 租约（`SKIP LOCKED`）——与 Run webhook outbox 相同，单实例 + `delivery_key` 幂等即可
- 修改适配器出站 JSON 协议（字段形状不变）
- 改动或合并 Run 事件表 `webhook_outbox`
- 飞书 / 钉钉真实适配器、公共 Go SDK、出站长轮询 pull
- blob 自动 GC、Settings 可调重试参数、多订阅 fan-out
- 在「消息回调」（Run 事件 Webhook）页混挂渠道投递表

### 1.3 已锁定产品决策

| 项 | 决策 |
|----|------|
| 成功标准 | 跨重启 + 长宕机死信 + 管理面重投（确认清单 C） |
| 媒体 | blob key 入队（A） |
| 存储 | 新表 `channel_outbox` + 独立 worker（A）；不扩 `webhook_outbox` |
| 调用语义 | 先入队、异步投递；写入成功即 `Send*` 返回 nil（A） |
| 管理面 UI | 设置 → 消息 → 微信页；不进消息回调页（A） |
| 实现路线 | 镜像 Run webhook outbox 状态机与重试策略（方案 1） |

## 2. 数据模型

表 `channel_outbox`（memory / sqlite / postgres 三驱动镜像）：

| 字段 | 说明 |
|------|------|
| `id` | UUID |
| `delivery_key` | UNIQUE 幂等键 |
| `channel` | 渠道实例名（如 `weixin`） |
| `kind` | `text` \| `media` |
| `peer_id` | IM peer |
| `conversation_id` | 会话 ID |
| `account` | IM 账号（出站 Account） |
| `run_id` | 关联 Run（可空；notify 等可能无） |
| `payload_json` | 出站报文快照（媒体项不含大字节，仅 name/mime + blob 引用） |
| `blob_keys_json` | 媒体 blob key 列表（文本可空 `[]`） |
| `target_url` | **入队时**解析的 `outbound_url` 快照 |
| `attempt` | 已完成 POST 次数（0 起，失败后递增） |
| `max_attempts` | 固定 **5** |
| `status` | `pending` \| `delivered` \| `dead` |
| `last_error` | 最近错误摘要（无 URL/secret） |
| `next_retry_at` | 下次投递 UTC |
| `created_at` / `updated_at` | 审计 |

索引：`(status, next_retry_at)`；`(channel, status)`（管理面按渠道过滤）。

### 2.1 幂等键

```
{channel}:{conversation_id}:{kind}:{run_id}:{unique_id}
```

- `run_id` 为空时用字面量 `_`（或与实现一致的占位）。
- `unique_id`：每次出站调用分配（UUID）。`PutChannelOutboxIfAbsent` 仅对**同一** `delivery_key` 的重复入队（同键重试）幂等；**不得**用进程内单调 `seq` 冒充 webhook eventIndex——重启归零后会与已 `delivered` 历史碰撞并静默丢消息。
- `PutChannelOutboxIfAbsent`：已存在且 status 为 `pending|delivered` 则返回 `(created=false, existingID)`，不覆盖。`delivery_key` UNIQUE 仍合理：同键不双写；新调用因 UUID 自然避开 dead 行挡死同键 INSERT 的问题。
- **Store 热切**：`channel_outbox` pending **不**随 Store 热切迁移拷贝；热切前宜排空，或接受旧库 due 丢弃（YAGNI：不做跨库迁移）。

### 2.2 Blob 约定

- key 建议前缀：`channel-outbox/{channel}/{id}/{filename}`（或等价；实现计划锁定一种）。`filename` 须经 `filepath.Base`（或拒绝含路径分隔符），防止路径穿越。
- `SendMedia`：先 `blob.Put`，再入队；Put 失败则 `SendMedia` 返回 error，不写 outbox。
- 死信保留 blob，便于重投；**本版不做自动删除**。

## 3. 投递路径与 Worker

```
Runtime → Channel.SendText | SendMedia
  →（media）blob.Put → key
  → 组装 OutboundMessage 元数据 + payload_json / blob_keys
  → PutChannelOutboxIfAbsent(pending, next_retry_at=now)
  → wake worker
  → return nil（仅 DB/blob 失败返回 err）

Worker（1s tick + wake channel）
  → ListChannelOutboxDue(now, limit)
  → 按 blob_keys 读 blob，拼完整 OutboundMessage（含 ContentBase64）
  → 签名 POST target_url（复用现有 webhooksig / Header* 约定）
  → 2xx → delivered
  → 网络错误 / 5xx / 429 → attempt++；未满 max → pending + 退避；满 → dead
  → 其他 4xx → 直接 dead（不重试）
```

### 3.1 行为细则

| 项 | 约定 |
|----|------|
| 同步重试 | **取消**：`sender.post` 不再在调用路径做 3 次退避；单次 POST 由 worker 执行，状态机负责重试 |
| 超时 | 单次 POST 仍 10s（与今日 `outboundTimeout` 一致） |
| 退避 | 1s → 2s → 4s → 8s → 16s（对齐 Run webhook outbox） |
| `target_url` | 入队快照；autostart 动态端口变更后，**已入队**行仍打旧 URL；新消息用新 URL |
| Worker 生命周期 | webhook Channel Bootstrap/Start 注入 `Store` + `blob.Store` 并启动；`Stop` 取消 ctx |
| 多副本 | 不做租约；靠唯一 `delivery_key`；部署文档可注明「渠道出站 worker 宜单副本承担」 |
| 引擎语义 | 入队失败记日志、不中断 run（与今日最终失败一致） |

### 3.2 Store 接口（计划级）

在 `internal/store` 增加与 `WebhookOutbox*` 平行的一组方法，例如：

- `PutChannelOutboxIfAbsent`
- `ListChannelOutboxDue`
- `ListChannelOutbox`（按 channel + statuses + limit）
- `GetChannelOutbox`
- `UpdateChannelOutbox`
- `ResetChannelOutboxRetry`

常量：`ChannelOutboxMaxAttempts = 5`；status/kind 类型与 Run outbox 同构命名风格。

## 4. Admin API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v0/settings/channels/{name}/outbound-deliveries?status=&limit=` | 默认 `status=dead,pending`；按 `{name}` 过滤；不含 headers/secret；媒体不回 base64，可回文件名/blob key |
| POST | `/v0/settings/channels/{name}/outbound-deliveries/{id}/retry` | `attempt=0`、`status=pending`、`next_retry_at=now`；wake worker；跨渠道 id 或缺失 → 404 |

鉴权：与现有渠道设置面一致（admin；operator 策略跟同页其它写操作对齐，实现时对照 `WeixinChannelSettings` / gate）。

列表项建议字段：`id`、`status`、`kind`、`peer_id`、`conversation_id`、`run_id`、`attempt`、`max_attempts`、`last_error`、`updated_at`。

## 5. UI

- 页面：`WeixinChannelSettings`（路由 `/settings/channels/weixin`）。
- 区块：页底「最近出站」表格 + 重投按钮；空态提示。
- 文案：集中在 `strings.ts`；避免「5xx / 死信 / outbox」等术语；人话对齐消息组既有风格（「未发出 / 多次失败 / 重试」）。
- API 客户端：`web/chat/src/api.ts` 增加 list/retry。
- **不**修改 `WebhookSettings`（消息回调 / Run 事件投递表）。
- 其它 webhook 渠道实例：同一 Admin API 可用；无专用设置页则本版不强制挂 UI。

## 6. 测试 DoD

1. **Store**：`channel_outbox` round-trip、幂等、due 查询（memory + sqlite；postgres 随驱动镜像）。
2. **Worker**：mock 适配器 503×2 → 200 成功；永久 4xx → `dead`；进程重启后 pending 续投。
3. **Media**：`blob.Put` → 行内仅 key → worker 组装 base64 后 POST 体正确。
4. **API**：list 过滤与默认 status；retry 重置字段；鉴权。
5. **UI**：表格渲染 + 重投冒烟（vitest）。
6. **回归**：原 `SendText` 签名/契约测试改为「入队 + worker 投递」路径断言；入站/登录/supervisor 不回归。

## 7. 文档与账本

实现合并后：

- 更新 `docs/architecture-and-plugin-protocol.md`（或渠道相关公开文档）：渠道出站为 durable outbox。
- `2026-09-06-phase2-webhook-channel-design.md`：将「不做持久化 outbox」改为指向本规格已交付。
- `2026-09-13-spec-ledger.md` / 确认清单：CH-OUTBOX → **已交付**。

## 8. 实现触点（计划级索引）

| 区域 | 预期 |
|------|------|
| `internal/store` | 模型 + 三驱动 CRUD |
| `internal/channel/webhook` | Enqueue 替换同步 `post` 重试；worker；注入 store/blob |
| `internal/bootstrap` / `wireChannels` | 依赖注入 |
| `internal/api` | outbound-deliveries 路由 |
| `web/chat` | api + WeixinChannelSettings + strings + 测试 |
| `internal/ui/dist` | 前端 build 嵌入（若本刀改 UI） |
| 文档 / 账本 | §7 |

## 9. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 动态端口后旧 `target_url` 失效 | 入队快照是刻意选择；适配器重启后 pending 可能短暂失败并重试/死信；运营可重投（新 URL 需否在 retry 时刷新：本版 **retry 不改 URL**，与 Run outbox 一致；若 PORT 抖动频繁导致大量 dead，后续可另开「retry 刷新 URL」小刀） |
| 双写与 Run outbox 行为漂移 | 状态机/退避/API 形状刻意镜像；代码可平行但不全量抽象（YAGNI，避免方案 3） |
| 媒体重复占 blob | 本版无 GC；可接受；后续与 BLOB-CS 一并评估 |
| Store 热切丢 pending | 渠道 outbox pending **不**随热切迁移；热切宜先排空或接受旧库 due 丢弃（本版不做拷贝） |

---

*本文件仅存在于 baize_real（`docs/superpowers`）；不进入 public 开源导出。*
