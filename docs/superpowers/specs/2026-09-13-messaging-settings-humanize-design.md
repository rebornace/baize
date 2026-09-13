# 消息组设置人话化（消息回调 / 外部来信 + 微信小补）

- 日期：2026-09-13
- 状态：已交付（2026-09-13）
- 归属：WebUI 体验改版后续；承接 P3 原语与 P4 关门后 defer 的「消息组人话化」
- 方法：方案 1（只做人话化回调+来信；微信不掉队小补），已批准

## 1. 背景与目标

消息组三页中：**微信**已有 P2 运营 gate 与部分中文；**消息回调**、**外部来信**仍为旧壳（页内英文主标题 `Webhook`/`Inbox`、裸 `settings-field`、状态串反馈、密钥用 drawer、删除无确认）。与账号/连接器/助手页体验落差大。

**目标：** 把 **消息回调**、**外部来信** 拉到与 P3 同级的人话 + 原语体验；**微信**只做不掉队的小补齐（`PageHeader`、Toast/错误映射等），不重做扫码与进程态机。

**成功标准：**

1. 两页标题与导航一致（「消息回调」「外部来信」），不再用页内英文 `Webhook` / `Inbox` 作主标题。
2. 使用 `PageHeader`、`Field`/`Input`/`Textarea`/`Button`、`Toast`、`ConfirmDialog`（危险操作）、`EmptyState`（空列表）；错误走 `friendlyError` / 集中 `strings`；无裸 `window.confirm`。
3. 字段标签人话化；技术细节（路径模板、HMAC、投递状态码）收进说明或「高级/详情」。
4. 不改任何后端 API / ACL / 路由 id；运营对两页仍为 locked（仅管理员）。
5. 相关 vitest 更新/新增；`tsc` / `build` 绿；重建 `internal/ui/dist`。

## 2. 已确认决策

1. 下一里程碑选 **消息组人话化**（非 F 生产硬化）。
2. 落地方式选 **方案 1**：主攻回调 + 来信；微信上限为不掉队小补。
3. 不改后端契约；不抽 `MessagingListShell`；不合并「消息回调」与「企业统一执行地址」概念。
4. 密钥展示从 drawer 迁到共享 `Modal`；删除通道与轮换密钥用 `ConfirmDialog`。

## 3. 范围

### 3.1 在范围内

| 页 | 做什么 |
|----|--------|
| 消息回调 `/settings/webhooks` | 人话壳 + 原语；URL/请求头；测试投递；投递列表/重试反馈与文案 |
| 外部来信 `/settings/inbox` | 人话壳 + 原语；通道列表增删改；轮换密钥 Modal；测试；复制收件地址；校验人话；出站覆盖进高级 |
| 微信 `/settings/channels/weixin` | `PageHeader` + Toast/错误映射；进程危险操作若缺确认可补 ConfirmDialog；**不**改登录轮询/进程态机/ACL/字段集合 |

### 3.2 不在范围

- 后端契约、多 Channel 广播、Inbox 附件/JSONPath、出站多订阅
- 微信 iLink 协议变更、钉钉/飞书新渠道
- Playwright E2E、F 生产硬化
- 合并消息回调与企业执行回调

## 4. 信息架构与术语

**原则：** 路由与 JSON 字段名不变；界面用导航人话；技术标识收进说明/`code`/高级区。

### 4.1 消息回调

| UI 元素 | 定稿文案 |
|---------|----------|
| 页标题 | 消息回调 |
| 副标题 | 有新消息或运行结束时，主动推送到你指定的地址（与对话实时流并行，不堵引擎） |
| 回调地址 | 原「Webhook URL」；hint：留空表示不推送 |
| 请求头 | 每行 `KEY=VALUE`，可选 |
| 保存 / 测试 | 保存；发送测试 |
| 投递区 | 标题「最近投递」；说明待投递与死信及重试策略（5xx/网络/429 最多 5 次，其余 4xx 死信） |
| 状态徽章 | 死信 / 待投递 / 已投递 |
| 重投 | 重投（无需 ConfirmDialog） |

**概念区分：** 页说明或字段 hint——本页是**运行事件出站通知**，不是连接高级里的「企业统一执行地址」。

布局：上配置、下投递列表；不拆 Modal。

### 4.2 外部来信

| UI 元素 | 定稿文案 |
|---------|----------|
| 页标题 | 外部来信 |
| 副标题 | 生成专属收件地址；外部系统签名 POST 后来信会变成对话 |
| 空态 | 还没有收件通道；添加后即可对接告警/工单等系统 |
| 添加 | 添加收件通道 |
| 通道标题 | 优先 id；无 id 时「新通道 #n」 |
| 通道标识 | 原 `id`；说明：小写开头，字母数字与 `_` `-`，最长 64 |
| 绑定助手 | 原 `agent_id` |
| 启用 / 说明 | 保持语义；说明原 `description` |
| 默认技能 | 原 skills 多选 |
| 出站覆盖地址 / 请求头 | 收进通道内「高级」；说明多数留空、仅覆盖全局消息回调 |
| 复制收件地址 | 按钮人话 |
| 轮换密钥 | ConfirmDialog → Modal 一次性明文 |
| 发送测试 / 删除 / 保存 | 见 §5 |

校验人话：不向用户展示正则原文。页头 ASCII 协议图改为短步骤或「技术说明」折叠。

### 4.3 微信（上限）

- `PageHeader` 标题「微信」+ 导航导航 desc。
- 裸错误 → Toast / 人话映射。
- 不改扫码轮询、进程启停逻辑、运营 login 边界、配置字段集合。

### 4.4 共有约定

- `strings.ts`：`WEBHOOKS` / `INBOX`（及必要时 `WEIXIN` 增量）。
- 成功 Toast；校验 inline；危险 ConfirmDialog。
- admin-only；运营 locked（不新增运营只读模式）。

## 5. 交互细节

### 5.1 消息回调

- 保存 / 测试 / 重投：成功 Toast；失败 Toast + `friendlyError`；表单校验可 inline。
- 投递行：`Badge` + 人话摘要（run id 可次要展示）。

### 5.2 外部来信

| 动作 | 交互 |
|------|------|
| 删除通道（本地行） | ConfirmDialog 后从列表移除；落库靠「保存」 |
| 轮换密钥 | ConfirmDialog（旧密钥立即失效）→ API → Modal 一次性明文 + 复制主 CTA |
| 复制地址/密钥 | Toast |
| 发送测试 | Toast；避免裸 `delivery=`/`run=` 堆砌 |
| 保存 | 整表 PUT；校验人话 inline |

### 5.3 微信

- 见 §4.3；进程危险操作若仍无确认可补 ConfirmDialog，已有则不动。

## 6. 工程

- 修改：`WebhookSettings.tsx`、`InboxSettings.tsx`（及现有测试）、按需 `WeixinChannelSettings.tsx`、`strings.ts`、少量 CSS。
- 保留可测纯函数（`validate*`、`formatDeliveryStatus` 等）；UI 换 P3-A 原语。
- 密钥弹窗：`settings-drawer` → 共享 `Modal`。
- 继续用 `connectorForms/lines` 解析 KEY=VALUE。
- 不改 Go；提交 `real`；过程文档仅 `docs/superpowers/**`。

## 7. 测试与验收

- vitest：标题人话、校验文案、ConfirmDialog（删/轮换）、密钥 Modal、Webhook 保存/测试路径（mock API）。
- `npm test`、`npx tsc --noEmit`、`npm run build` → `internal/ui/dist`。
- 手动：两页空态/保存/测试；Inbox 增删轮换；与企业执行地址文案不混淆；微信扫码区无回归。

## 8. 风险与对策

| 风险 | 对策 |
|------|------|
| Inbox 迁原语漏字段 | 对照现有字段清单；测试锁 PUT body |
| 密钥仅一次展示 | Modal 强调文案 + 复制主按钮 |
| 概念混淆 | §4.1 区分句 |

## 9. 完成定义（DoD）

- [x] 消息回调、外部来信人话 + 原语齐备，主标题与导航一致
- [x] 微信 PageHeader/反馈不掉队；扫码/进程逻辑未改
- [x] 删除/轮换走 ConfirmDialog；密钥走 Modal
- [x] 无未解释的 `window.confirm`；校验无人话正则甩锅
- [x] 测试 / tsc / build 绿；`internal/ui/dist` 已更新
- [x] 非目标未纳入本里程碑

## 10. 之后

本里程碑关门后，开源路线图仍可回到 backlog **F 生产硬化 + 架构文档 P1/P2**（另开头脑风暴）。

## 11. 参考

- P3-A 原语：`docs/superpowers/specs/2026-09-11-webui-refresh-p3-forms-accounts-storage-design.md`
- 导航：`web/chat/src/settingsNav.ts`
- P3-C 曾 defer：Webhook/Inbox 人话化（`2026-09-12-webui-refresh-p3c-mcp-pages-design.md` §11）
