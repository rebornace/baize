# 消息回调 / 外部来信结果导向文案 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 按规格把消息回调、外部来信主界面改成结果导向白话；协议/格式/重试细则收入「给技术人员」折叠；UI 不出现产品名。

**架构：** 纯前端。定稿文案集中在 `strings.ts` 的 `WEBHOOKS` / `INBOX`；两页把硬编码协议段改成读 strings；状态徽章与校验错误同步换人话键。不改 Go、路由、ACL。

**技术栈：** React 19、TypeScript、Vite 6、Vitest + jsdom、`web/chat`。命令：`cd web/chat` 下 `npm test`、`npx tsc --noEmit`、`npm run build`（产物进 `internal/ui/dist`）。提交私有仓，中文 Conventional Commits。

规格：`docs/superpowers/specs/2026-09-13-messaging-copy-outcome-design.md`

对照：`InboxSettings.tsx` 已有 `<details className="settings-developer">`；Webhook 页需对称增加。

---

## 文件结构

修改：

- `web/chat/src/strings.ts` — 重写 `WEBHOOKS` / `INBOX` 键文案；新增折叠正文键
- `web/chat/src/pages/WebhookSettings.tsx` — 使用新键；增加「给技术人员」折叠；列表区用新标题/说明
- `web/chat/src/pages/WebhookSettings.test.ts` — `formatDeliveryStatus('dead')` 等期望值
- `web/chat/src/pages/WebhookSettings.test.tsx` — 主路径无协议词；折叠内可见技术细节；标题/hint
- `web/chat/src/pages/InboxSettings.tsx` — 技术说明改 strings；密钥尾号文案；高级区 hint
- `web/chat/src/pages/InboxSettings.test.ts` — 校验键（文案变了仍用 `INBOX.err*`）
- `web/chat/src/pages/InboxSettings.test.tsx` — 副标题/空态/无 HMAC；折叠 summary；密钥尾号
- `internal/ui/dist/**` — 末任务重建
- `docs/superpowers/specs/2026-09-13-messaging-copy-outcome-design.md` — 状态已交付、勾 DoD

不改：Go、微信页、`settingsNav` 路由 id、运营 locked。

**文案锁定（实现时与规格一致；按钮用「重试」）：** 见下方任务内完整对象字面量。禁止字符串含「白泽」「Baize」。

---

## 任务 1：`WEBHOOKS` 文案 + 投递状态 + 校验键

**文件：**
- 修改：`web/chat/src/strings.ts`
- 修改：`web/chat/src/pages/WebhookSettings.test.ts`
- （本任务可不改 UI 结构；`formatDeliveryStatus` 已读 `WEBHOOKS.status*`，改 strings 即生效）

- [ ] **步骤 1：先改测试期望（失败驱动）**

在 `WebhookSettings.test.ts`：

```ts
expect(formatDeliveryStatus('dead')).toBe(WEBHOOKS.statusDead)
expect(formatDeliveryStatus('pending')).toBe(WEBHOOKS.statusPending)
expect(formatDeliveryStatus('delivered')).toBe(WEBHOOKS.statusDelivered)
// 另增：
expect(WEBHOOKS.statusDead).toBe('已停止（多次失败）')
expect(WEBHOOKS.description).not.toMatch(/引擎|HMAC|5xx|KEY=VALUE|白泽|Baize/i)
expect(WEBHOOKS.deliveriesHint).not.toMatch(/5xx|429|4xx|死信/)
expect(WEBHOOKS.headersHint).not.toMatch(/KEY=VALUE/)
expect(WEBHOOKS.errBadHeaderLine).not.toMatch(/KEY=VALUE/)
```

`errBadHeaderLine` 用例仍 `toBe(WEBHOOKS.errBadHeaderLine)`。

- [ ] **步骤 2：运行确认失败**

```powershell
cd web/chat
npm test -- src/pages/WebhookSettings.test.ts
```

预期：FAIL（旧文案仍是「死信」等）。

- [ ] **步骤 3：替换 `WEBHOOKS` 对象**

```ts
export const WEBHOOKS = {
  title: '消息回调',
  description:
    '有消息或任务结束时，自动通知你填的网址。这不是连接里的「企业统一执行地址」。',
  urlLabel: '通知地址',
  urlHint: '留空表示不发送通知',
  headersLabel: '请求头',
  headersHint: '一般不用填；有对接文档时按文档填写',
  save: '保存',
  saving: '保存中…',
  test: '发送测试',
  testing: '测试中…',
  deliveriesTitle: '最近通知',
  deliveriesHint: '可查看未发出或多次失败的通知，并可重试。',
  deliveriesEmpty: '暂无待发送或已停止的记录。',
  retry: '重试',
  retrying: '重试中…',
  statusDead: '已停止（多次失败）',
  statusPending: '等待发送',
  statusDelivered: '已发送',
  techDetails: '给技术人员',
  techDetailsBody:
    '请求头每行格式为 KEY=VALUE。网络错误、HTTP 5xx 或 429 会自动重试（最多 5 次）；其余 4xx 记为已停止。本页是运行结束后的出站通知，不是连接高级里的「企业统一执行地址」。',
  toastSaved: '已保存消息回调配置',
  toastTestOk: '测试发送成功',
  toastTestFail: '测试发送失败',
  toastRetryQueued: '已加入重试队列',
  errBadHeaderLine: '请求头填写不符合对接文档要求，请核对后重试',
  loadFailed: '无法加载消息回调配置',
} as const
```

- [ ] **步骤 4：再跑测试确认通过**

```powershell
npm test -- src/pages/WebhookSettings.test.ts
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/strings.ts web/chat/src/pages/WebhookSettings.test.ts
git commit -m "feat(web): 消息回调文案改为结果导向"
```

---

## 任务 2：消息回调页折叠 + 主路径无协议词测试

**文件：**
- 修改：`web/chat/src/pages/WebhookSettings.tsx`
- 修改：`web/chat/src/pages/WebhookSettings.test.tsx`

- [ ] **步骤 1：写失败测试**

在 `WebhookSettings.test.tsx` 增加（渲染后、**不**点开 details）：

```ts
it('keeps protocol jargon out of the main path', async () => {
  // mock getEventsWebhook / getEventsWebhookDeliveries 同现有用例
  // render WebhookSettings
  const main = host.textContent ?? ''
  expect(main).toContain(WEBHOOKS.urlLabel)
  expect(main).toContain(WEBHOOKS.deliveriesTitle)
  expect(main).not.toMatch(/\bHMAC\b|\bPOST\b|5xx|KEY=VALUE/)
  const details = host.querySelector('details.settings-developer')
  expect(details).toBeTruthy()
  expect(details!.querySelector('summary')?.textContent).toBe(WEBHOOKS.techDetails)
  // 展开后可见技术正文
  ;(details as HTMLDetailsElement).open = true
  expect(host.textContent).toContain('KEY=VALUE')
})
```

（若 jsdom 对 `.open` 不触发子树可见性，改为 `expect(details!.textContent).toContain('KEY=VALUE')`——details 内文本仍在 DOM。）

- [ ] **步骤 2：运行确认失败**

```powershell
npm test -- src/pages/WebhookSettings.test.tsx
```

预期：FAIL（尚无 `settings-developer` 折叠）。

- [ ] **步骤 3：在 PageHeader 下增加折叠**

与 Inbox 同结构：

```tsx
<details className="settings-developer">
  <summary>{WEBHOOKS.techDetails}</summary>
  <p className="settings-meta">{WEBHOOKS.techDetailsBody}</p>
</details>
```

字段已绑 `WEBHOOKS.*`，任务 1 改完后主路径自动新人话；本步只加折叠。

- [ ] **步骤 4：测试通过后 Commit**

```powershell
npm test -- src/pages/WebhookSettings.test.tsx
git add web/chat/src/pages/WebhookSettings.tsx web/chat/src/pages/WebhookSettings.test.tsx
git commit -m "feat(web): 消息回调技术细节收入折叠"
```

---

## 任务 3：`INBOX` 文案 + 校验人话键

**文件：**
- 修改：`web/chat/src/strings.ts`
- 修改：`web/chat/src/pages/InboxSettings.test.ts`（若仅用 `INBOX.err*` 引用则可能无需改断言；增无产品名/无 KEY=VALUE 于主路径键的断言）

- [ ] **步骤 1：扩展 `InboxSettings.test.ts` 断言**

```ts
expect(INBOX.description).not.toMatch(/HMAC|POST|签名|白泽|Baize/i)
expect(INBOX.idHint).not.toMatch(/小写|64|正则|_/)
expect(INBOX.headersHint ?? INBOX.overrideHeadersHint ?? '').not.toMatch(/KEY=VALUE/)
// 若尚无 overrideHeadersHint，在步骤 3 增加该键后再断言
expect(INBOX.errIdRequired).toMatch(/通道名称/)
expect(INBOX.errBadHeaderLine).not.toMatch(/KEY=VALUE/)
```

- [ ] **步骤 2：运行确认失败**

```powershell
npm test -- src/pages/InboxSettings.test.ts
```

- [ ] **步骤 3：替换 / 扩展 `INBOX`**

```ts
export const INBOX = {
  title: '外部来信',
  description:
    '给外部系统一个专用收件地址；对方按约定发来后，会在这里变成一场对话。',
  emptyTitle: '还没有收件通道',
  emptyDesc: '添加后，告警、工单等系统就能把消息送进来。',
  add: '添加收件通道',
  save: '保存',
  saving: '保存中…',
  channelNew: '新通道',
  idLabel: '通道名称（英文）',
  idHint: '保存后不要轻易改；用来区分不同来源',
  agentLabel: '用哪个助手处理',
  enabledLabel: '启用',
  descriptionLabel: '备注',
  skillsLabel: '默认带上的技能',
  skillsEmpty: '暂无可用技能',
  advanced: '高级',
  overrideUrlLabel: '本通道专用通知地址',
  overrideUrlHint: '多数情况留空，会用「消息回调」里的全局设置',
  overrideHeadersLabel: '本通道请求头',
  overrideHeadersHint: '一般不用填；有对接文档时按文档填写',
  copyUrl: '复制收件地址',
  rotateSecret: '更换密钥',
  rotating: '更换中…',
  test: '发送测试',
  testing: '测试中…',
  remove: '删除',
  confirmRemoveTitle: '删除该收件通道？',
  confirmRemoveBody:
    '从列表去掉这个通道。若已保存过，还需再点一次「保存」才会真正删除。',
  confirmRemoveOk: '删除',
  confirmRotateTitle: '更换该通道密钥？',
  confirmRotateBody:
    '旧密钥将立即失效，对方系统需换成新密钥。新密钥只显示一次。',
  confirmRotateOk: '更换',
  secretModalTitle: '新密钥',
  secretModalBody: '仅展示一次，请立即复制到对方系统。',
  secretCopy: '复制密钥',
  secretClose: '关闭',
  secretHint: (tail: string) => `已设置密钥（尾号 ···${tail}）`,
  techDetails: '给技术人员',
  techDetailsBody:
    '外部系统按约定签名后，向 {origin}/v0/inbox/{channel_id} 发送请求，即可在这里创建对话。请求头格式为每行 KEY=VALUE。签名与示例见仓库 README「生产集成」相关章节。通道高级里的专用通知地址可覆盖全局「消息回调」。',
  toastSaved: '已保存外部来信通道',
  toastCopiedUrl: '已复制收件地址',
  toastCopiedSecret: '已复制新密钥',
  toastTestOk: '测试已发出',
  toastRotateOk: '密钥已更换',
  errIdRequired: '请填写通道名称',
  errAgentRequired: '请选择要用的助手',
  errIdFormat: '通道名称格式不正确',
  errIdDuplicate: '通道名称重复',
  errBadHeaderLine: '请求头填写不符合对接文档要求，请核对后重试',
  loadFailed: '无法加载外部来信配置',
} as const
```

注意：`secretHint` 若用函数，`as const` 对象里可改为普通导出旁路函数：

```ts
export function inboxSecretHint(tail: string): string {
  return `已设置密钥（尾号 ···${tail}）`
}
```

则 `INBOX` 内不放函数，测试与 UI 调 `inboxSecretHint`。

- [ ] **步骤 4：测试通过 + Commit**

```powershell
npm test -- src/pages/InboxSettings.test.ts
git add web/chat/src/strings.ts web/chat/src/pages/InboxSettings.test.ts
git commit -m "feat(web): 外部来信文案改为结果导向"
```

---

## 任务 4：外部来信页结构对齐（折叠正文、密钥尾号、高级 hint）

**文件：**
- 修改：`web/chat/src/pages/InboxSettings.tsx`
- 修改：`web/chat/src/pages/InboxSettings.test.tsx`

- [ ] **步骤 1：写失败测试**

```ts
it('hides protocol jargon until tech details opened', async () => {
  // mock 空 channels + agents/skills 同现有
  expect(host.textContent).toContain(INBOX.description)
  expect(host.textContent).not.toMatch(/\bHMAC\b/)
  // 未展开时主文案不应出现「白泽」
  expect(host.textContent).not.toMatch(/白泽|Baize/)
  const details = host.querySelector('details.settings-developer')
  expect(details?.querySelector('summary')?.textContent).toBe(INBOX.techDetails)
  expect(details?.textContent).toContain('KEY=VALUE')
})

it('shows human secret hint instead of raw secret label', async () => {
  // mock 一行 channel 带 secret_hint: 'ab12'
  expect(host.textContent).toContain(inboxSecretHint('ab12'))
  expect(host.textContent).not.toMatch(/secret\s*\u2026/i)
})
```

高级区：有行时展开 `summary` 为 `INBOX.advanced`，断言 `overrideUrlLabel` / `overrideHeadersHint` 可见且无 `KEY=VALUE` 于 Field hint（`KEY=VALUE` 仅在 tech details）。

- [ ] **步骤 2：运行确认失败**

```powershell
npm test -- src/pages/InboxSettings.test.tsx
```

- [ ] **步骤 3：改 InboxSettings.tsx**

1. `<details>` 内改为 `{INBOX.techDetailsBody}`（可对 `{origin}` / `{channel_id}` 保持纯文本，或拆成带 `<code>` 的小段落——若用纯字符串含花括号即可，与规格「短步骤 + 路径示意」一致）。
2. 密钥展示：`{row.secret_hint ? <span className="settings-muted"> · {inboxSecretHint(row.secret_hint)}</span> : null}`
3. 高级里 `overrideHeaders` 的 `Field` 增加 `hint={INBOX.overrideHeadersHint}`。
4. 所有标签已走 `INBOX.*`，任务 3 改完即生效。

- [ ] **步骤 4：通过 + Commit**

```powershell
npm test -- src/pages/InboxSettings.test.tsx
git add web/chat/src/pages/InboxSettings.tsx web/chat/src/pages/InboxSettings.test.tsx web/chat/src/strings.ts
git commit -m "feat(web): 外部来信技术细节与密钥展示人话化"
```

---

## 任务 5：全量验证、dist、规格关门

**文件：**
- 修改：`internal/ui/dist/**`
- 修改：`docs/superpowers/specs/2026-09-13-messaging-copy-outcome-design.md`（状态 → 已交付；DoD 勾选）

- [ ] **步骤 1：全量测试与类型检查**

```powershell
cd web/chat
npm test
npx tsc --noEmit
```

预期：全绿。

- [ ] **步骤 2：构建嵌入产物**

```powershell
npm run build
```

确认 `internal/ui/dist` 更新。

- [ ] **步骤 3：产品名扫描**

```powershell
Select-String -Path src/strings.ts,src/pages/WebhookSettings.tsx,src/pages/InboxSettings.tsx -Pattern '白泽|Baize'
```

预期：无命中（注释除外；UI 字符串不得有）。

- [ ] **步骤 4：更新规格状态与 DoD，Commit**

```powershell
git add internal/ui/dist docs/superpowers/specs/2026-09-13-messaging-copy-outcome-design.md
git commit -m "chore(ui): 重建嵌入产物并关闭结果导向文案里程碑"
```

---

## 规格覆盖自检

| 规格章节 | 任务 |
|----------|------|
| §2 成功标准 1–5 | 2、4、5 |
| §4 原则 | 1–4 |
| §5 消息回调文案/折叠 | 1、2 |
| §6 外部来信文案/折叠/密钥 | 3、4 |
| §7 工程 / §8 测试 / §10 非目标 | 5；未纳入微信/后端 |
| 无产品名 | 1、3、5 扫描 |

无占位符；`retry` 锁定为「重试」；`inboxSecretHint` 与 `INBOX` 键名在任务 3/4 一致。
