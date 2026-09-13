# 消息组设置人话化实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将「消息回调」「外部来信」迁到 P3 原语 + 导航人话；微信仅补 `PageHeader`/Toast（及缺省危险确认），不改扫码与进程态机。

**架构：** 以前端为主。文案进 `strings.ts`（`WEBHOOKS` / `INBOX` / 可选 `WEIXIN`）。两页换 `PageHeader`/`Field`/`Input`/`Textarea`/`Button`/`Badge`/`EmptyState`/`Toast`/`ConfirmDialog`/`Modal`。保留现有纯函数与 PUT/GET API；Inbox 校验文案人话化；密钥弹窗从 drawer 迁 `Modal`。不改 Go。

**技术栈：** React 19、TypeScript、Vite 6、Vitest + jsdom、`web/chat`。命令：`cd web/chat && npm test`、`npx tsc --noEmit`、`npm run build`（→ `internal/ui/dist`）。提交 `real/main`，中文 Conventional Commits。

规格：`docs/superpowers/specs/2026-09-13-messaging-settings-humanize-design.md`

对照实现：`IdentitiesSettings.tsx`（PageHeader + useToast + ConfirmDialog）、`StorageSettings.tsx`（Field/Input）、`ModelSettings.tsx`（ConfirmDialog 测试模式）。

---

## 文件结构

修改：

- `web/chat/src/strings.ts` — 新增 `WEBHOOKS`、`INBOX`、按需 `WEIXIN`
- `web/chat/src/pages/WebhookSettings.tsx` — 人话壳 + 原语 + Toast
- `web/chat/src/pages/WebhookSettings.test.ts` — 纯函数仍测；可增 header 错误文案键断言
- `web/chat/src/pages/WebhookSettings.test.tsx` — **新建** jsdom：标题、保存 Toast、测试按钮
- `web/chat/src/pages/InboxSettings.tsx` — 人话壳 + 原语 + ConfirmDialog + Modal；`validateChannelsForm` 人话
- `web/chat/src/pages/InboxSettings.test.ts` — 校验文案改断言 `INBOX.err*`
- `web/chat/src/pages/InboxSettings.test.tsx` — **新建** jsdom：空态、删除确认、轮换 Modal
- `web/chat/src/pages/WeixinChannelSettings.tsx` — PageHeader + Toast；登出/停进程若无确认则补 ConfirmDialog
- `web/chat/src/pages/WeixinChannelSettings.test.ts` — 按需补标题断言（若已有渲染测则扩）
- `internal/ui/dist/**` — 末任务重建
- `docs/superpowers/specs/2026-09-13-messaging-settings-humanize-design.md` — 状态改为已交付、勾 DoD

不改：Go API、路由、ACL、微信登录轮询逻辑、抽 MessagingListShell。

---

## 任务 1：`WEBHOOKS` 文案 + 校验错误人话键

**文件：**
- 修改：`web/chat/src/strings.ts`
- 修改：`web/chat/src/pages/WebhookSettings.tsx`（仅 `validateWebhookForm` 使用 strings；UI 壳可留任务 2）
- 修改：`web/chat/src/pages/WebhookSettings.test.ts`

- [ ] **步骤 1：在 strings 增加 WEBHOOKS（先写测试期望的键）**

在 `WebhookSettings.test.ts` 的 invalid header 用例改为：

```ts
import { WEBHOOKS } from '../strings'
// ...
expect(result.message).toBe(WEBHOOKS.errBadHeaderLine)
// 或 toContain 人话片段，勿再依赖「无效键值行」唯一性若你改为更长句
```

- [ ] **步骤 2：运行确认失败**

```powershell
cd web/chat
npm test -- src/pages/WebhookSettings.test.ts
```

预期：FAIL（无 `WEBHOOKS` 或文案未改）。

- [ ] **步骤 3：实现 WEBHOOKS 并改 validateWebhookForm**

```ts
// strings.ts
export const WEBHOOKS = {
  title: '消息回调',
  description:
    '有新消息或运行结束时，主动推送到你指定的地址（与对话实时流并行，不堵引擎）。这是运行事件通知，不是连接里的「企业统一执行地址」。',
  urlLabel: '回调地址',
  urlHint: '留空表示不推送',
  headersLabel: '请求头',
  headersHint: '每行 KEY=VALUE，可选',
  save: '保存',
  saving: '保存中…',
  test: '发送测试',
  testing: '测试中…',
  deliveriesTitle: '最近投递',
  deliveriesHint:
    '展示待投递与死信；网络错误、5xx 或 429 会自动重试（最多 5 次），其余 4xx 进死信。',
  deliveriesEmpty: '暂无待投递或死信记录。',
  retry: '重投',
  retrying: '重投中…',
  statusDead: '死信',
  statusPending: '待投递',
  statusDelivered: '已投递',
  toastSaved: '已保存消息回调配置',
  toastTestOk: '测试投递成功',
  toastTestFail: '测试投递失败',
  toastRetryQueued: '已加入重投队列',
  errBadHeaderLine: '请求头格式不正确：请使用每行 KEY=VALUE',
  loadFailed: '无法加载消息回调配置',
} as const
```

`validateWebhookForm`：headers 解析失败时 `message: WEBHOOKS.errBadHeaderLine`（可附带行号细节到 message 后缀，测试用 `toContain` 或精确相等——选一并锁死）。

`formatDeliveryStatus` 可改为读 `WEBHOOKS.status*`（同步改纯函数测试期望值，文案应仍为 死信/待投递/已投递）。

- [ ] **步骤 4：测试通过并提交**

```powershell
npm test -- src/pages/WebhookSettings.test.ts
git add web/chat/src/strings.ts web/chat/src/pages/WebhookSettings.tsx web/chat/src/pages/WebhookSettings.test.ts
git commit -m "feat(web): 消息回调文案常量与校验人话"
```

---

## 任务 2：消息回调页 UI 原语化

**文件：**
- 修改：`web/chat/src/pages/WebhookSettings.tsx`
- 创建：`web/chat/src/pages/WebhookSettings.test.tsx`

- [ ] **步骤 1：编写失败的 jsdom 测试**

```tsx
// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { WEBHOOKS } from '../strings'
import { WebhookSettings } from './WebhookSettings'

async function renderPage() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  vi.spyOn(api, 'getEventsWebhook').mockResolvedValue({ url: '', headers: {} })
  vi.spyOn(api, 'getEventsWebhookDeliveries').mockResolvedValue([])
  await act(async () => {
    root.render(createElement(WebhookSettings))
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

describe('WebhookSettings UI', () => {
  afterEach(() => vi.restoreAllMocks())

  it('shows humanized title and empty-url hint, not English Webhook heading', async () => {
    const { host, root } = await renderPage()
    expect(host.textContent).toContain(WEBHOOKS.title)
    expect(host.textContent).toContain(WEBHOOKS.urlHint)
    expect(host.querySelector('h1')?.textContent).not.toBe('Webhook')
    root.unmount()
    host.remove()
  })

  it('save success uses toast path (putEventsWebhook called)', async () => {
    const put = vi.spyOn(api, 'putEventsWebhook').mockResolvedValue({
      url: 'https://example.com/h',
      headers: {},
    })
    const { host, root } = await renderPage()
    const url = host.querySelector('input') as HTMLInputElement
    await act(async () => {
      // 触发受控输入：按页面实际 input 选择器调整
      const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        'value',
      )!.set!
      nativeInputValueSetter.call(url, 'https://example.com/h')
      url.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const save = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(WEBHOOKS.save),
    )
    await act(async () => {
      save!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(put).toHaveBeenCalled()
    expect(host.textContent).toContain(WEBHOOKS.toastSaved)
    root.unmount()
    host.remove()
  })
})
```

实现时：以真实 DOM（`Field`/`Input` 的结构）调整选择器；Toast 可能在 `ToastRegion` 内，断言 `host.textContent` 含 toast 文案即可。

- [ ] **步骤 2：运行确认失败**

```powershell
npm test -- src/pages/WebhookSettings.test.tsx
```

- [ ] **步骤 3：改写 WebhookSettings UI**

对齐 `IdentitiesSettings`：

```tsx
import {
  Badge, Button, Field, Input, PageHeader, Textarea, ToastRegion, useToast,
} from '../components/ui'
import { WEBHOOKS, friendlyError } from '../strings'
```

- 去掉 `<h1>Webhook</h1>` 与裸 `settings-error` 成功串；`push({ tone:'success', title: WEBHOOKS.toastSaved })` 等。
- `formatDeliveryStatus` 已人话；行上用 `<Badge>`。
- 加载失败：`push` danger + `friendlyError`。

- [ ] **步骤 4：测试通过 + commit**

```powershell
npm test -- src/pages/WebhookSettings.test.ts src/pages/WebhookSettings.test.tsx
git add web/chat/src/pages/WebhookSettings.tsx web/chat/src/pages/WebhookSettings.test.tsx
git commit -m "feat(web): 消息回调页人话化与原语迁移"
```

---

## 任务 3：`INBOX` 文案 + `validateChannelsForm` 人话

**文件：**
- 修改：`web/chat/src/strings.ts`
- 修改：`web/chat/src/pages/InboxSettings.tsx`（校验函数）
- 修改：`web/chat/src/pages/InboxSettings.test.ts`

- [ ] **步骤 1：改测试断言为人话键**

```ts
import { INBOX } from '../strings'

it('requires id', () => {
  const r = validateChannelsForm([{ id: '', agent_id: 'a', enabled: true }])
  expect(r.ok).toBe(false)
  if (!r.ok) expect(r.message).toContain(INBOX.errIdRequired) // 或整句匹配
})

it('requires agent', () => {
  const r = validateChannelsForm([{ ...baseRow(), agent_id: '' }])
  expect(r.ok).toBe(false)
  if (!r.ok) {
    expect(r.message).not.toContain('agent_id')
    expect(r.message).toMatch(/助手|绑定/)
  }
})

it('rejects invalid channel id slug without regex dump', () => {
  const r = validateChannelsForm([{ ...baseRow(), id: 'Bad ID' }])
  expect(r.ok).toBe(false)
  if (!r.ok) {
    expect(r.message).not.toMatch(/\^\[/)
    expect(r.message).toContain(INBOX.errIdFormat)
  }
})
```

- [ ] **步骤 2：运行确认失败**

```powershell
npm test -- src/pages/InboxSettings.test.ts
```

- [ ] **步骤 3：实现 INBOX 常量并改 validateChannelsForm**

```ts
export const INBOX = {
  title: '外部来信',
  description: '生成专属收件地址；外部系统签名 POST 后来信会变成对话。',
  emptyTitle: '还没有收件通道',
  emptyDesc: '添加后即可对接告警、工单等外部系统。',
  add: '添加收件通道',
  save: '保存',
  saving: '保存中…',
  channelNew: '新通道',
  idLabel: '通道标识',
  idHint: '小写字母开头，仅字母、数字与 _ -，最长 64 个字符',
  agentLabel: '绑定助手',
  enabledLabel: '启用',
  descriptionLabel: '说明',
  skillsLabel: '默认技能',
  skillsEmpty: '暂无可用技能',
  advanced: '高级',
  overrideUrlLabel: '出站覆盖地址',
  overrideUrlHint: '仅本通道覆盖全局消息回调，多数场景留空',
  overrideHeadersLabel: '出站请求头',
  copyUrl: '复制收件地址',
  rotateSecret: '轮换密钥',
  rotating: '轮换中…',
  test: '发送测试',
  testing: '测试中…',
  remove: '删除',
  confirmRemoveTitle: '删除该收件通道？',
  confirmRemoveBody: '将从本页列表移除。若尚未点保存，仅丢掉未提交的修改；已保存过的需再点保存才会从服务器删除。',
  confirmRemoveOk: '删除',
  confirmRotateTitle: '轮换该通道密钥？',
  confirmRotateBody: '旧密钥将立即失效，外部系统需尽快换成新密钥。新密钥只显示一次。',
  confirmRotateOk: '轮换',
  secretModalTitle: '新密钥',
  secretModalBody: '仅展示一次，请立即复制到外部系统。',
  secretCopy: '复制密钥',
  secretClose: '关闭',
  techDetails: '技术说明',
  toastSaved: '已保存外部来信通道',
  toastCopiedUrl: '已复制收件地址',
  toastCopiedSecret: '已复制新密钥',
  toastTestOk: '测试已发出',
  toastRotateOk: '密钥已轮换',
  errIdRequired: '请填写通道标识',
  errAgentRequired: '请选择绑定助手',
  errIdFormat: '通道标识格式不正确',
  errIdDuplicate: '通道标识重复',
  errBadHeaderLine: '出站请求头格式不正确：请使用每行 KEY=VALUE',
  loadFailed: '无法加载外部来信配置',
} as const
```

`validateChannelsForm` 消息全部改用上述键（可带「第 n 条：」前缀）。

- [ ] **步骤 4：测试通过 + commit**

```powershell
npm test -- src/pages/InboxSettings.test.ts
git add web/chat/src/strings.ts web/chat/src/pages/InboxSettings.tsx web/chat/src/pages/InboxSettings.test.ts
git commit -m "feat(web): 外部来信校验与文案人话化"
```

---

## 任务 4：外部来信页 UI（ConfirmDialog + Modal）

**文件：**
- 修改：`web/chat/src/pages/InboxSettings.tsx`
- 创建：`web/chat/src/pages/InboxSettings.test.tsx`

- [ ] **步骤 1：编写失败 jsdom 测试**

覆盖：

1. 标题为 `INBOX.title`，无 `<h1>Inbox</h1>`；空列表显示 `emptyTitle`。
2. 点删除 → 出现 ConfirmDialog（`confirmRemoveTitle`），确认前行仍在；确认后行消失；`window.confirm` 未被调用。
3. mock `rotateInboxSecret`：点轮换 → ConfirmDialog → 确认 → Modal 含明文 secret，无 `settings-drawer`。

渲染需 mock：`getInboxChannels`、`getUIConfig`、`listSkills`（返回空数组即可）。Gate 若页面不用可省略。

参考 `ModelSettings.test.tsx` 的 `confirm-ok` / `GateContext` 模式。

- [ ] **步骤 2：运行确认失败**

```powershell
npm test -- src/pages/InboxSettings.test.tsx
```

- [ ] **步骤 3：实现 UI**

- `PageHeader` + `ToastRegion`/`useToast`；`EmptyState` 空列表。
- 每通道：`Field`/`Input`/`Select`/`Textarea`；出站 URL/headers 放 `<details className="settings-advanced">`。
- 删除：`pendingRemoveIndex` + `ConfirmDialog`。
- 轮换：`pendingRotateId` + `ConfirmDialog` → API → `secretModal` 用 `<Modal open title={...}>`（删 drawer markup）。
- 页头 ASCII `<pre>` → `details`「技术说明」短文或折叠。
- 保存/测试/复制：Toast。

- [ ] **步骤 4：测试通过 + commit**

```powershell
npm test -- src/pages/InboxSettings.test.ts src/pages/InboxSettings.test.tsx
git add web/chat/src/pages/InboxSettings.tsx web/chat/src/pages/InboxSettings.test.tsx web/chat/src/strings.ts
git commit -m "feat(web): 外部来信页人话化与确认/密钥弹窗"
```

---

## 任务 5：微信页不掉队小补

**文件：**
- 修改：`web/chat/src/strings.ts`（`WEIXIN` 或增量键）
- 修改：`web/chat/src/pages/WeixinChannelSettings.tsx`
- 修改：`web/chat/src/pages/WeixinChannelSettings.test.ts`（或新建 `.tsx` 若仅有纯函数测）

- [ ] **步骤 1：失败测试——渲染含人话标题「微信」**

若现有测试无渲染：新增最小 jsdom，mock `getWeixinSettings` / login status，断言 `PageHeader`/`WEIXIN.title`，且保留运营时配置表单隐藏等既有行为（抽 1 条回归即可）。

- [ ] **步骤 2：实现**

- 顶部改为 `PageHeader title="微信" description={导航 desc 同源文案}`。
- `setError` 裸红字改为 `push(friendlyError(...))` 或保留轻量 inline + Toast（与规格「裸错误→Toast」一致）。
- 检查登出 / 停止进程：若无确认，为登出与停止补 `ConfirmDialog`（启动/重启可不确认）。**不要**改 `POLL_MS`、login 状态机、`isAdmin` 分支结构。

- [ ] **步骤 3：测试 + commit**

```powershell
npm test -- src/pages/WeixinChannelSettings.test.ts
# 若有新 tsx：一并跑
git add web/chat/src/pages/WeixinChannelSettings.tsx web/chat/src/pages/WeixinChannelSettings.test.ts web/chat/src/strings.ts
git commit -m "fix(web): 微信设置页 PageHeader 与反馈对齐"
```

---

## 任务 6：全量验证 + dist + DoD

**文件：**
- `internal/ui/dist/**`
- `docs/superpowers/specs/2026-09-13-messaging-settings-humanize-design.md`

- [ ] **步骤 1：全量**

```powershell
cd web/chat
npm test
npx tsc --noEmit
npm run build
```

- [ ] **步骤 2：抽检**

```powershell
Select-String -Path web/chat/src/pages/WebhookSettings.tsx,web/chat/src/pages/InboxSettings.tsx -Pattern 'Webhook</h1>|Inbox</h1>|window\.confirm|settings-drawer'
```

预期：无英文主标题、无 `window.confirm`、Inbox 无 `settings-drawer`。

- [ ] **步骤 3：规格状态 → 已交付，勾选 §9 DoD**

- [ ] **步骤 4：Commit（不要擅自 push public）**

```powershell
git add internal/ui/dist docs/superpowers/specs/2026-09-13-messaging-settings-humanize-design.md
git commit -m "chore(ui): 重建嵌入产物并关闭消息组人话化"
```

可选：`git push real main`（用户未禁止时可推 real）。

---

## 规格覆盖自检

| 规格 | 任务 |
|------|------|
| §3.1 消息回调 | 1–2 |
| §3.1 外部来信 | 3–4 |
| §3.1/4.3/5.3 微信小补 | 5 |
| §4 术语 / §5 Confirm+Modal | 2–4 |
| §7/§9 DoD + dist | 6 |
| 非目标（Go/E2E/F/抽壳） | 无任务 |

占位符扫描：无 TBD；Inbox 删除语义（先本地删再保存）已在 `confirmRemoveBody` 写明。

---

## 执行交接

计划已保存。两种执行方式：

**1. 子代理驱动（推荐）** — 每任务新子代理 + 任务间审查  

**2. 内联执行** — 本会话 executing-plans  

选哪种方式？
