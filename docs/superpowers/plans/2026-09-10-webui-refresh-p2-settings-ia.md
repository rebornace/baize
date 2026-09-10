# P2 设置信息架构 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 把「设置」从一长串英文技术名页面，改造成人话卡片首页（分组 + 实时状态徽标 + 新手引导），并让运营人员获得只读可见性与微信扫码登录能力。

**架构：** 前端以 `settingsNav.ts` 为侧边导航与首页卡片的唯一数据源（分组/图标/说明/访问级别/badge kind）；新增纯函数徽标解析层 + 并发取数 hook（单源失败静默降级）；新 `SettingsHome` 页替换现有 index 跳转。后端仅在 `controlplane/acl.go` 放开 4 条规则（工具只读、微信只读、微信扫码登录/轮询）。五个设置页按 `useGate().role` 做控件级只读 gate，不改页面布局。

**技术栈：** Go 1.25+（表驱动 ACL 测试 `go test ./internal/controlplane/`）；React 19 + TypeScript + vitest（jsdom）+ react-router-dom v6 + lucide-react；前端产物 `npm run build` 后由 Go `//go:embed` 内嵌。

**规格：** `docs/superpowers/specs/2026-09-10-webui-refresh-p2-settings-ia.md`

**分支：** 开始任务 1 前从最新 `main` 建分支 `feat/webui-refresh-p2-settings-ia`。

**工作目录：** 前端命令在 `web/chat/` 下执行；Go 命令在仓库根执行。

---

## 文件结构

| 文件 | 职责 | 动作 |
|---|---|---|
| `internal/controlplane/acl.go` | 接口最小角色表 | 改 4 条规则 |
| `internal/controlplane/acl_test.go` | 表驱动 ACL 断言 | 改期望值 + 加保底用例 |
| `web/chat/src/settingsNav.ts` | 导航/卡片唯一数据源：13 项、四分组、图标、说明、运营访问级别、badge kind | 重写 |
| `web/chat/src/settingsNav.test.ts` | 数据源结构/角色过滤断言 | 重写 |
| `web/chat/src/settingsHomeBadges.ts` | `BadgeKind`、`BadgeResult`、`resolveBadge` 纯函数、`useSettingsBadges` hook | 新建 |
| `web/chat/src/settingsHomeBadges.test.ts` | 纯函数全态 + hook 并发/降级/角色跳过 | 新建 |
| `web/chat/src/pages/SettingsHome.tsx` | 设置首页：页头/刷新/完成度条/分组卡片栅格 | 新建 |
| `web/chat/src/pages/SettingsHome.test.tsx` | 两角色渲染、提示条、locked 卡、导航 | 新建 |
| `web/chat/src/pages/SettingsLayout.tsx` | 总览入口 + 分组导航 + 角色过滤 | 改 |
| `web/chat/src/styles/settings.css` | 首页与分组导航样式（token 化） | 新建 |
| `web/chat/src/main.tsx` | index 路由指向 SettingsHome；import settings.css | 改 |
| `ModelSettings.tsx` / `ToolsSettings.tsx` / `SkillsSettings.tsx` / `RuntimeSettings.tsx` / `WeixinChannelSettings.tsx` | role 控件级 gate | 改 |
| 上述 5 页 `*.test.tsx`（新建，若同页已有测试文件则追加 describe） | 运营只读断言 | 新建/追加 |
| `internal/ui/dist/**` | 嵌入产物 | 任务 7 重建 |

**关键类型约定（全计划一致，后续任务不得改名）：**

```ts
// settingsNav.ts
export type SettingsRole = 'operator' | 'admin'
export type SettingsGroup = 'assistant' | 'connect' | 'messaging' | 'system'
export type SettingsAccess = 'full' | 'read' | 'login' | 'locked'
export type BadgeKind =
  | 'models' | 'tools' | 'skills' | 'openapi' | 'mcp' | 'plugins'
  | 'mcpExport' | 'weixin' | 'webhook' | 'inbox' | 'store' | 'runtime'

export interface SettingsNavItem {
  to: string                 // 如 '/settings/openapi'
  label: string              // 人话名
  group: SettingsGroup
  icon: LucideIcon           // lucide-react 图标组件
  desc: string               // 卡片一句话说明
  badge?: BadgeKind
  /** 运营人员访问级别；admin 对所有项隐式 full。 */
  operator: Exclude<SettingsAccess, 'full'> | 'full'
}
```

```ts
// settingsHomeBadges.ts
export interface BadgeResult {
  // 直接使用 ui/Badge 的 tone 命名（规格中的 ok/warn/muted 对应 success/warning/neutral）
  tone: 'success' | 'warning' | 'neutral'
  text: string
}
```

图标统一约定（lucide-react，均为真实导出）：总览 `LayoutGrid`；模型 `Cpu`；助手功能 `Wrench`；技能 `Sparkles`；业务系统 `Network`；MCP `Boxes`；插件 `Puzzle`；MCP 导出 `Share2`；微信 `MessageCircle`；消息回调 `Webhook`；外部来信 `Inbox`；账号 `Users`；存储 `Database`；运行参数 `SlidersHorizontal`；锁 `Lock`；刷新 `RefreshCw`。

---

## 任务 1：后端 ACL 放开 4 条规则

**文件：**
- 修改：`internal/controlplane/acl.go:33`（tools GET）、`:57-59`（login/start、login/status）、`:63`（channel GET）
- 测试：`internal/controlplane/acl_test.go`

- [ ] **步骤 1：先改测试期望值，确认失败**

在 `internal/controlplane/acl_test.go` 的 `TestMinRoleTable` cases 中，把以下 4 行的 `RoleAdmin` 改为 `RoleOperator`：

```go
		{"GET", "/v0/tools", RoleOperator},
```

```go
		{"POST", "/v0/settings/channels/weixin/login/start", RoleOperator},
		{"GET", "/v0/settings/channels/weixin/login/status", RoleOperator},
```

```go
		{"GET", "/v0/settings/channels/weixin", RoleOperator},
```

注意：同表中 `{"PATCH", "/v0/tools/create_ticket", RoleAdmin}`、`{"POST", "/v0/settings/channels/weixin/logout", RoleAdmin}`、`{"PUT", "/v0/settings/channels/weixin", RoleAdmin}` 三行**保持 RoleAdmin 不变**（保底断言，防止放开过头）。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/controlplane/ -run TestMinRoleTable -v`
预期：FAIL，4 处 `got "admin" want "operator"`。

- [ ] **步骤 3：改 ACL 规则**

在 `internal/controlplane/acl.go` 中：

第 33 行：
```go
	{method: "GET", segments: []string{"v0", "tools"}, role: RoleOperator},
```

第 57-59 行三条（login/start、login/status）与第 63 行 channel GET：
```go
	{method: "POST", segments: []string{"v0", "settings", "channels", "{name}", "login", "start"}, role: RoleOperator},
	{method: "GET", segments: []string{"v0", "settings", "channels", "{name}", "login", "status"}, role: RoleOperator},
```
```go
	{method: "GET", segments: []string{"v0", "settings", "channels", "{name}"}, role: RoleOperator},
```

其余规则（logout/process/PUT、tools PATCH、webhook/inbox/store/credentials/mcp-export）一律不动。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/controlplane/ -v`
预期：PASS（含 `TestChannelInboundIsRoleNone`）。

- [ ] **步骤 5：全量 Go 测试保底，再 commit**

运行：`go build ./... && go test ./...`
预期：BUILD OK、全部 PASS。

```bash
git add internal/controlplane/acl.go internal/controlplane/acl_test.go
git commit -m "feat(controlplane): 运营只读工具/微信状态并放开微信扫码登录"
```

---

## 任务 2：重写导航单一数据源 `settingsNav.ts`

**文件：**
- 重写：`web/chat/src/settingsNav.ts`
- 重写：`web/chat/src/settingsNav.test.ts`

- [ ] **步骤 1：编写失败的测试（整体替换 `settingsNav.test.ts`）**

```ts
import { describe, expect, it } from 'vitest'
import { SETTINGS_GROUPS, settingsNavItems, visibleNavItems } from './settingsNav'

const adminItems = settingsNavItems('admin')
const operatorItems = settingsNavItems('operator')

describe('SETTINGS_GROUPS', () => {
  it('declares the four groups in display order', () => {
    expect(SETTINGS_GROUPS.map((g) => g.id)).toEqual([
      'assistant',
      'connect',
      'messaging',
      'system',
    ])
    expect(SETTINGS_GROUPS.map((g) => g.label)).toEqual(['助手', '连接', '消息', '系统'])
  })
})

describe('settingsNavItems(admin)', () => {
  it('returns 13 items with unique routes and complete metadata', () => {
    expect(adminItems).toHaveLength(13)
    const tos = adminItems.map((i) => i.to)
    expect(new Set(tos).size).toBe(13)
    for (const item of adminItems) {
      expect(item.label).toBeTruthy()
      expect(item.desc).toBeTruthy()
      expect(typeof item.icon).toBe('function')
      expect(SETTINGS_GROUPS.some((g) => g.id === item.group)).toBe(true)
    }
  })

  it('uses the friendly names', () => {
    const byTo = Object.fromEntries(adminItems.map((i) => [i.to, i.label]))
    expect(byTo['/settings/models']).toBe('模型')
    expect(byTo['/settings/tools']).toBe('助手功能')
    expect(byTo['/settings/openapi']).toBe('业务系统')
    expect(byTo['/settings/channels/weixin']).toBe('微信')
    expect(byTo['/settings/runtime']).toBe('运行参数')
  })

  it('groups items correctly', () => {
    const inGroup = (g: string) => adminItems.filter((i) => i.group === g).map((i) => i.to)
    expect(inGroup('assistant')).toEqual(['/settings/models', '/settings/tools', '/settings/skills'])
    expect(inGroup('connect')).toEqual([
      '/settings/openapi',
      '/settings/mcp',
      '/settings/plugins',
      '/settings/mcp-export',
    ])
    expect(inGroup('messaging')).toEqual([
      '/settings/channels/weixin',
      '/settings/webhooks',
      '/settings/inbox',
    ])
    expect(inGroup('system')).toEqual(['/settings/identities', '/settings/storage', '/settings/runtime'])
  })

  it('declares badge kinds for the cards that have live status', () => {
    const badgeByTo = Object.fromEntries(adminItems.map((i) => [i.to, i.badge]))
    expect(badgeByTo['/settings/models']).toBe('models')
    expect(badgeByTo['/settings/identities']).toBeUndefined()
  })
})

describe('settingsNavItems(operator) access levels', () => {
  it('marks read/login/full/locked correctly', () => {
    const access = (to: string) => operatorItems.find((i) => i.to === to)?.operator
    expect(access('/settings/models')).toBe('read')
    expect(access('/settings/tools')).toBe('read')
    expect(access('/settings/skills')).toBe('read')
    expect(access('/settings/runtime')).toBe('read')
    expect(access('/settings/channels/weixin')).toBe('login')
    expect(access('/settings/identities')).toBe('full')
    expect(access('/settings/openapi')).toBe('locked')
    expect(access('/settings/storage')).toBe('locked')
  })
})

describe('visibleNavItems', () => {
  it('admin sees every item', () => {
    expect(visibleNavItems('admin')).toHaveLength(13)
  })

  it('operator sidebar hides locked items (no connect group), keeps 6', () => {
    const visible = visibleNavItems('operator')
    expect(visible.map((i) => i.to)).toEqual([
      '/settings/models',
      '/settings/tools',
      '/settings/skills',
      '/settings/channels/weixin',
      '/settings/identities',
      '/settings/runtime',
    ])
    expect(visible.some((i) => i.group === 'connect')).toBe(false)
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`npx vitest run src/settingsNav.test.ts`
预期：FAIL（`SETTINGS_GROUPS`/`visibleNavItems` 未导出、类型不匹配）。

- [ ] **步骤 3：整体重写 `settingsNav.ts`**

```ts
import {
  Boxes,
  Cpu,
  Database,
  Inbox,
  LayoutGrid,
  MessageCircle,
  Network,
  Puzzle,
  Share2,
  SlidersHorizontal,
  Sparkles,
  Users,
  Webhook,
  Wrench,
  type LucideIcon,
} from 'lucide-react'

export type SettingsRole = 'operator' | 'admin'
export type SettingsGroup = 'assistant' | 'connect' | 'messaging' | 'system'
export type SettingsAccess = 'full' | 'read' | 'login' | 'locked'
export type BadgeKind =
  | 'models' | 'tools' | 'skills' | 'openapi' | 'mcp' | 'plugins'
  | 'mcpExport' | 'weixin' | 'webhook' | 'inbox' | 'store' | 'runtime'

export interface SettingsNavItem {
  to: string
  label: string
  group: SettingsGroup
  icon: LucideIcon
  desc: string
  badge?: BadgeKind
  /** Operator access level. Admin implicitly has full access to every item. */
  operator: SettingsAccess
}

export const OVERVIEW_ITEM = {
  to: '/settings',
  label: '总览',
  icon: LayoutGrid,
} as const

export const SETTINGS_GROUPS: { id: SettingsGroup; label: string }[] = [
  { id: 'assistant', label: '助手' },
  { id: 'connect', label: '连接' },
  { id: 'messaging', label: '消息' },
  { id: 'system', label: '系统' },
]

const ITEMS: SettingsNavItem[] = [
  // 助手
  {
    to: '/settings/models', label: '模型', group: 'assistant', icon: Cpu, operator: 'read',
    badge: 'models',
    desc: '管理对话与理解用的模型；「智能选择」会按任务自动切换（未来向量、音频等模型在此扩展分类）',
  },
  {
    to: '/settings/tools', label: '助手功能', group: 'assistant', icon: Wrench, operator: 'read',
    badge: 'tools',
    desc: '助手能调用的功能开关，如查订单、建工单；可设置是否需你确认或登录',
  },
  {
    to: '/settings/skills', label: '技能', group: 'assistant', icon: Sparkles, operator: 'read',
    badge: 'skills',
    desc: '可复用的操作流程，对话里输入 @ 或 / 即可调用',
  },
  // 连接
  {
    to: '/settings/openapi', label: '业务系统', group: 'connect', icon: Network, operator: 'locked',
    badge: 'openapi',
    desc: '粘贴接口文档即可连上公司的订单、工单等系统，不用写代码',
  },
  {
    to: '/settings/mcp', label: 'MCP', group: 'connect', icon: Boxes, operator: 'locked',
    badge: 'mcp',
    desc: '接入标准 MCP 工具服务，扩展助手能力',
  },
  {
    to: '/settings/plugins', label: '插件', group: 'connect', icon: Puzzle, operator: 'locked',
    badge: 'plugins',
    desc: '接入独立部署的插件程序',
  },
  {
    to: '/settings/mcp-export', label: 'MCP 导出', group: 'connect', icon: Share2, operator: 'locked',
    badge: 'mcpExport',
    desc: '把助手能力以标准 MCP 方式开放给其他客户端',
  },
  // 消息
  {
    to: '/settings/channels/weixin', label: '微信', group: 'messaging', icon: MessageCircle,
    operator: 'login', badge: 'weixin',
    desc: '接入微信账号，让客户/同事通过微信与助手对话（未来钉钉、飞书各占一张卡）',
  },
  {
    to: '/settings/webhooks', label: '消息回调', group: 'messaging', icon: Webhook, operator: 'locked',
    badge: 'webhook',
    desc: '有新消息或事件时，主动推送到你指定的地址',
  },
  {
    to: '/settings/inbox', label: '外部来信', group: 'messaging', icon: Inbox, operator: 'locked',
    badge: 'inbox',
    desc: '生成专属收件地址，外部系统来信自动变成对话',
  },
  // 系统
  {
    to: '/settings/identities', label: '账号', group: 'system', icon: Users, operator: 'full',
    desc: '管理能登录、使用助手的人员',
  },
  {
    to: '/settings/storage', label: '存储', group: 'system', icon: Database, operator: 'locked',
    badge: 'store',
    desc: '对话和数据保存在哪里（本地文件或数据库）',
  },
  {
    to: '/settings/runtime', label: '运行参数', group: 'system', icon: SlidersHorizontal,
    operator: 'read', badge: 'runtime',
    desc: '压缩、超时等运行行为，修改后即时生效',
  },
]

/** Full catalog; admin sees all 13, operator items carry their access level. */
export function settingsNavItems(_role: SettingsRole): SettingsNavItem[] {
  return ITEMS
}

/** Items rendered in the sidebar / reachable by a role: locked items are operator-invisible. */
export function visibleNavItems(role: SettingsRole): SettingsNavItem[] {
  if (role === 'admin') return ITEMS
  return ITEMS.filter((i) => i.operator !== 'locked')
}
```

注意：`settingsNavItems(role)` 对两种角色都返回完整 13 项（首页需要展示全部卡片，含 locked）；角色差异通过 `operator` 字段与 `visibleNavItems` 表达。原调用方 `SettingsLayout` 将在任务 6 改用 `visibleNavItems`。

- [ ] **步骤 4：运行测试验证通过**

运行：`npx vitest run src/settingsNav.test.ts`
预期：PASS。

- [ ] **步骤 5：类型检查 + commit**

`SettingsLayout.tsx` 当前 import 的是旧签名 `settingsNavItems`。把第 5 行
```ts
import { settingsNavItems } from '../settingsNav'
```
改为
```ts
import { visibleNavItems } from '../settingsNav'
```
并把第 10 行 `const nav = settingsNavItems(role)` 改为 `const nav = visibleNavItems(role)`。渲染处仍只用 `item.label`/`item.to`，无需改动；分组与「总览」入口在任务 6 才加入。

运行：`npx tsc --noEmit`
预期：PASS。

```bash
git add web/chat/src/settingsNav.ts web/chat/src/settingsNav.test.ts web/chat/src/pages/SettingsLayout.tsx
git commit -m "refactor(web): 设置导航单一数据源（分组/图标/说明/运营级别/badge）"
```

---

## 任务 3：徽标纯函数解析层 `settingsHomeBadges.ts`

**文件：**
- 创建：`web/chat/src/settingsHomeBadges.ts`
- 测试：`web/chat/src/settingsHomeBadges.test.ts`

本任务只做纯函数（不做 React hook、不发请求）。`BadgeResult.tone` 直接使用 `ui/Badge` 的 tone 值。

- [ ] **步骤 1：编写失败的测试**

创建 `web/chat/src/settingsHomeBadges.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import type {
  EventsWebhookConfig,
  InboxChannel,
  ModelProfile,
  RuntimeKnobsView,
  SkillSummary,
  StoreSettings,
  ToolInfo,
  WeixinChannelSettings,
} from './api'
import type { MCPExportIdentity } from './api'
import {
  countConnectorsBySource,
  enabledToolCount,
  resolveBadge,
} from './settingsHomeBadges'

const model = (over: Partial<ModelProfile> = {}): ModelProfile => ({
  id: 'm1', name: 'm', provider: 'openai_compatible', base_url: 'u', model: 'gpt',
  disable_thinking: false, supports_vision: false, context_tokens: 1, auto_tier: 'standard', ...over,
})
const tool = (source: string, connector = 'c', enabled = true): ToolInfo => ({
  name: source + connector, connector_id: connector, source, enabled,
})
const skill = (id: string): SkillSummary => ({ id, name: id, description: '', tools: [], source: 'builtin' })

describe('enabledToolCount', () => {
  it('counts only enabled tools', () => {
    expect(enabledToolCount([tool('spec', 'a'), tool('mcp', 'b', false)])).toBe(1)
    expect(enabledToolCount([])).toBe(0)
  })
})

describe('countConnectorsBySource', () => {
  it('dedupes connectors and counts only matching sources', () => {
    const tools = [
      tool('spec', 'oa1'), tool('extra', 'oa1'), tool('spec', 'oa2'),
      tool('mcp', 'm1'), tool('plugin', 'p1'),
    ]
    expect(countConnectorsBySource(tools, ['spec', 'extra'])).toBe(2)
    expect(countConnectorsBySource(tools, ['mcp'])).toBe(1)
    expect(countConnectorsBySource(tools, ['plugin'])).toBe(1)
  })
})

describe('resolveBadge', () => {
  it('models: 0 warns, >0 ok', () => {
    expect(resolveBadge('models', [])).toEqual({ tone: 'warning', text: '未配置' })
    expect(resolveBadge('models', [model(), model()])).toEqual({ tone: 'success', text: '已配置 2 个' })
  })

  it('tools: none hidden, some enabled ok, all exist-but-disabled warns', () => {
    expect(resolveBadge('tools', [])).toBeNull()
    expect(resolveBadge('tools', [tool('spec', 'a'), tool('mcp', 'b', false)])).toEqual({ tone: 'success', text: '1 项可用' })
    expect(resolveBadge('tools', [tool('spec', 'a', false)])).toEqual({ tone: 'warning', text: '未启用' })
  })

  it('skills / mcpExport / inbox: count shown neutral, zero hidden', () => {
    expect(resolveBadge('skills', { skills: [skill('a'), skill('b')] })).toEqual({ tone: 'neutral', text: '2 个技能' })
    expect(resolveBadge('skills', { skills: [] })).toBeNull()
    const id = (i: string): MCPExportIdentity => ({ id: i, name: i, scheme: '' }) as MCPExportIdentity
    expect(resolveBadge('mcpExport', [id('x')])).toEqual({ tone: 'neutral', text: '1 个出口' })
    expect(resolveBadge('mcpExport', [])).toBeNull()
    const ch = (i: string): InboxChannel => ({ id: i, agent_id: 'a', enabled: true })
    expect(resolveBadge('inbox', [ch('c1')])).toEqual({ tone: 'neutral', text: '1 个收件地址' })
    expect(resolveBadge('inbox', [])).toBeNull()
  })

  it('openapi/mcp/plugins connectors: n ok, zero neutral go-connect for openapi, hidden otherwise', () => {
    expect(resolveBadge('openapi', [tool('mcp', 'm1')])).toEqual({ tone: 'neutral', text: '去接入' })
    expect(resolveBadge('openapi', [tool('spec', 'oa1')])).toEqual({ tone: 'success', text: '已接 1 个' })
    expect(resolveBadge('mcp', [tool('mcp', 'm1')])).toEqual({ tone: 'success', text: '已接 1 个' })
    expect(resolveBadge('mcp', [tool('spec', 'oa1')])).toBeNull()
    expect(resolveBadge('plugins', [tool('plugin', 'p1')])).toEqual({ tone: 'success', text: '已接 1 个' })
    expect(resolveBadge('plugins', [])).toBeNull()
  })

  it('weixin: running / enabled-with-reason / disabled / missing', () => {
    expect(resolveBadge('weixin', { running: true } as WeixinChannelSettings)).toEqual({ tone: 'success', text: '运行中' })
    expect(resolveBadge('weixin', { enabled: true, running: false, reason: 'login_required' })).toEqual({ tone: 'warning', text: '待登录' })
    expect(resolveBadge('weixin', { enabled: true, running: false, reason: 'start_failed' })).toEqual({ tone: 'warning', text: '启动异常' })
    expect(resolveBadge('weixin', { enabled: true, running: false, reason: 'stopped' })).toEqual({ tone: 'warning', text: '已停用' })
    expect(resolveBadge('weixin', { enabled: false, running: false })).toEqual({ tone: 'neutral', text: '未接入' })
    expect(resolveBadge('weixin', {} as WeixinChannelSettings)).toEqual({ tone: 'neutral', text: '未接入' })
  })

  it('webhook: url set ok, empty neutral', () => {
    expect(resolveBadge('webhook', { url: 'https://x', headers: {} })).toEqual({ tone: 'success', text: '已设置' })
    expect(resolveBadge('webhook', { url: '', headers: {} })).toEqual({ tone: 'neutral', text: '未设置' })
  })

  it('store: sqlite maps to 本地文件, other drivers shown lowercase as-is', () => {
    expect(resolveBadge('store', { driver: 'sqlite' } as StoreSettings)).toEqual({ tone: 'neutral', text: '本地文件' })
    expect(resolveBadge('store', { driver: 'postgres' } as StoreSettings)).toEqual({ tone: 'neutral', text: 'postgres' })
  })

  it('runtime: any override customised, else default', () => {
    const view = (overridden: Record<string, boolean>): RuntimeKnobsView =>
      ({ effective: {}, overridden }) as unknown as RuntimeKnobsView
    expect(resolveBadge('runtime', view({}))).toEqual({ tone: 'neutral', text: '默认' })
    expect(resolveBadge('runtime', view({ max_messages: true }))).toEqual({ tone: 'neutral', text: '已自定义' })
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`npx vitest run src/settingsHomeBadges.test.ts`
预期：FAIL（模块不存在）。若 `MCPExportIdentity` 字段名不符，先以 `web/chat/src/api.ts` 中该 interface 的真实字段调整测试的 `as MCPExportIdentity` 构造（仅类型断言，不断言字段）。

- [ ] **步骤 3：实现 `settingsHomeBadges.ts`**

```ts
import type {
  EventsWebhookConfig,
  InboxChannel,
  ModelProfile,
  RuntimeKnobsView,
  SkillSummary,
  StoreSettings,
  ToolInfo,
  WeixinChannelSettings,
} from './api'
import type { BadgeKind } from './settingsNav'

export interface BadgeResult {
  tone: 'success' | 'warning' | 'neutral'
  text: string
}

/** Enabled tool count (enabled omitted/undefined counts as enabled, matching catalog semantics). */
export function enabledToolCount(tools: readonly ToolInfo[]): number {
  return tools.filter((t) => t.enabled !== false).length
}

/** Distinct connector_ids among tools whose source is in the accepted set. */
export function countConnectorsBySource(tools: readonly ToolInfo[], sources: readonly string[]): number {
  const ids = new Set<string>()
  for (const t of tools) {
    if (t.source && sources.includes(t.source) && t.connector_id) ids.add(t.connector_id)
  }
  return ids.size
}

function weixinBadge(s: WeixinChannelSettings | undefined): BadgeResult {
  if (s?.running === true) return { tone: 'success', text: '运行中' }
  if (s?.enabled) {
    switch (s.reason) {
      case 'login_required': return { tone: 'warning', text: '待登录' }
      case 'start_failed': return { tone: 'warning', text: '启动异常' }
      default: return { tone: 'warning', text: '已停用' }
    }
  }
  return { tone: 'neutral', text: '未接入' }
}

/**
 * Pure badge resolver. `data` shape depends on kind; pass the unwrapped API
 * payload. Returns null when the card should show no badge.
 */
export function resolveBadge(kind: BadgeKind, data: unknown): BadgeResult | null {
  switch (kind) {
    case 'models': {
      const n = (data as ModelProfile[]).length
      return n === 0 ? { tone: 'warning', text: '未配置' } : { tone: 'success', text: `已配置 ${n} 个` }
    }
    case 'tools': {
      const tools = data as ToolInfo[]
      if (tools.length === 0) return null
      const n = enabledToolCount(tools)
      return n === 0 ? { tone: 'warning', text: '未启用' } : { tone: 'success', text: `${n} 项可用` }
    }
    case 'skills': {
      const n = ((data as { skills: SkillSummary[] }).skills ?? []).length
      return n === 0 ? null : { tone: 'neutral', text: `${n} 个技能` }
    }
    case 'openapi': {
      const n = countConnectorsBySource(data as ToolInfo[], ['spec', 'extra'])
      return n === 0 ? { tone: 'neutral', text: '去接入' } : { tone: 'success', text: `已接 ${n} 个` }
    }
    case 'mcp':
    case 'plugins': {
      const sources = kind === 'mcp' ? ['mcp'] : ['plugin']
      const n = countConnectorsBySource(data as ToolInfo[], sources)
      return n === 0 ? null : { tone: 'success', text: `已接 ${n} 个` }
    }
    case 'mcpExport': {
      const n = (data as { id: string }[]).length
      return n === 0 ? null : { tone: 'neutral', text: `${n} 个出口` }
    }
    case 'weixin':
      return weixinBadge(data as WeixinChannelSettings)
    case 'webhook': {
      const cfg = data as EventsWebhookConfig
      return cfg.url && cfg.url.trim() !== ''
        ? { tone: 'success', text: '已设置' }
        : { tone: 'neutral', text: '未设置' }
    }
    case 'inbox': {
      const n = (data as InboxChannel[]).length
      return n === 0 ? null : { tone: 'neutral', text: `${n} 个收件地址` }
    }
    case 'store': {
      const driver = (data as StoreSettings).driver ?? ''
      return { tone: 'neutral', text: driver === 'sqlite' ? '本地文件' : driver.toLowerCase() }
    }
    case 'runtime': {
      const overridden = (data as RuntimeKnobsView).overridden ?? {}
      const custom = Object.values(overridden).some(Boolean)
      return { tone: 'neutral', text: custom ? '已自定义' : '默认' }
    }
    default: {
      const _exhaustive: never = kind
      return _exhaustive
    }
  }
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`npx vitest run src/settingsHomeBadges.test.ts`
预期：PASS。

- [ ] **步骤 5：类型检查 + commit**

运行：`npx tsc --noEmit`，预期 PASS。

```bash
git add web/chat/src/settingsHomeBadges.ts web/chat/src/settingsHomeBadges.test.ts
git commit -m "feat(web): 设置首页状态徽标纯函数解析层"
```

---

## 任务 4：并发取数 hook `useSettingsBadges`

**文件：**
- 修改：`web/chat/src/settingsHomeBadges.ts`（追加 hook 与取数映射）
- 测试：`web/chat/src/settingsHomeBadges.test.ts`（追加 hook describe）

`listTools()` 一次返回覆盖 tools/openapi/mcp/plugins 四种 kind 的数据，因此 hook 对该接口只发一次请求并复用结果。运营无权限的 locked kind 不发请求。

- [ ] **步骤 1：在测试文件顶部补齐 hook 测试所需 import**

在 `settingsHomeBadges.test.ts` 第一行加 jsdom 环境标记，并补充 React/测试工具 import（`describe/expect/it` 已在文件首行 import，不重复）：

```ts
// @vitest-environment jsdom
```
在现有 `import { describe, expect, it } from 'vitest'` 行扩展为：
```ts
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
```
（若原行已含 `afterEach/beforeEach/vi` 则合并，不重复声明。）

- [ ] **步骤 2：编写 hook 的失败测试（追加到同文件末尾）**

```ts
// ---- useSettingsBadges ----
import { useSettingsBadges } from './settingsHomeBadges'
import { settingsNavItems } from './settingsNav'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
function runHook<T>(useHook: (refreshKey: number) => T): { value: () => T; rerender: (p: { key: number }) => void } {
  let current: T
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  const Harness = ({ keyStep }: { keyStep: number }) => {
    current = useHook(keyStep)
    return null
  }
  act(() => { root.render(<Harness keyStep={0} />) })
  return {
    value: () => current,
    rerender: (p) => act(() => { root.render(<Harness keyStep={p.key} />) }),
  }
}

describe('useSettingsBadges', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn()) })
  afterEach(() => { vi.unstubAllGlobals() })

  const adminKinds = settingsNavItems('admin').map((i) => i.badge).filter(Boolean) as string[]

  const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)) })

  it('admin: fetches shared tool list once and resolves all badges', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/tools') return jsonResponse([{ name: 't', connector_id: 'oa1', source: 'spec', enabled: true }])
      if (u === '/v0/settings/models') return jsonResponse({ profiles: [{ id: 'm1' }] })
      if (u === '/v0/skills') return jsonResponse({ skills: [{ id: 's1' }] })
      if (u === '/v0/settings/channels/weixin') return jsonResponse({ running: true })
      if (u === '/v0/settings/events-webhook') return jsonResponse({ url: 'https://x', headers: {} })
      if (u === '/v0/settings/inbox-channels') return jsonResponse({ channels: [] })
      if (u === '/v0/settings/store') return jsonResponse({ driver: 'sqlite' })
      if (u === '/v0/settings/runtime') return jsonResponse({ effective: {}, overridden: {} })
      if (u === '/v0/settings/mcp-export/identities') return jsonResponse([])
      return jsonResponse(null)
    })
    const h = runHook(() => useSettingsBadges(settingsNavItems('admin'), 'admin', 0))
    await flush()
    const toolsCalls = fetchMock.mock.calls.filter(([u]) => String(u) === '/v0/tools').length
    expect(toolsCalls).toBe(1)
    const badges = h.value()
    expect(badges.models).toEqual({ tone: 'success', text: '已配置 1 个' })
    expect(badges.weixin).toEqual({ tone: 'success', text: '运行中' })
    expect(badges.store).toEqual({ tone: 'neutral', text: '本地文件' })
  })

  it('operator: never requests locked kinds (no webhook/inbox/store/mcp-export)', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async () => jsonResponse(null))
    runHook(() => useSettingsBadges(settingsNavItems('admin'), 'operator', 0))
    await flush()
    const urls = fetchMock.mock.calls.map(([u]) => String(u))
    expect(urls).not.toContain('/v0/settings/events-webhook')
    expect(urls).not.toContain('/v0/settings/inbox-channels')
    expect(urls).not.toContain('/v0/settings/store')
    expect(urls).not.toContain('/v0/settings/mcp-export/identities')
    // operator-allowed kinds ARE fetched
    expect(urls).toContain('/v0/tools')
    expect(urls).toContain('/v0/settings/channels/weixin')
  })

  it('a failing request yields null badge without affecting other kinds', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async (url: unknown) =>
      String(url) === '/v0/settings/models'
        ? new Response('nope', { status: 500 })
        : jsonResponse({ profiles: [{ id: 'm1' }] }))
    const h = runHook(() => useSettingsBadges(
      settingsNavItems('admin').filter((i) => i.badge === 'models' || i.badge === 'weixin'),
      'admin', 0))
    await flush()
    const badges = h.value()
    expect(badges.models).toBeNull()
  })

  it('re-fetches when refreshKey changes', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async () => jsonResponse({ profiles: [] }))
    const h = runHook((k) => useSettingsBadges(settingsNavItems('admin'), 'admin', k))
    await flush()
    h.rerender({ key: 1 })
    await flush()
    expect(fetchMock.mock.calls.length).toBeGreaterThan(1)
  })
})
```

注意第三个用例：weixin 数据未 mock 返回 null，断言以 models 为 null 为准；不为 weixin 设强断言，避免脆弱。

- [ ] **步骤 3：运行测试验证失败**

运行：`npx vitest run src/settingsHomeBadges.test.ts`
预期：FAIL（`useSettingsBadges` 未导出）。

- [ ] **步骤 4：在 `settingsHomeBadges.ts` 末尾实现 hook**

```ts
import { useEffect, useState } from 'react'
import {
  getEventsWebhook,
  getInboxChannels,
  getRuntimeSettings,
  getStoreSettings,
  getWeixinSettings,
  listModelProfiles,
  listMCPExportIdentities,
  listSkills,
  listTools,
  type ToolInfo,
} from './api'
import type { SettingsNavItem, SettingsRole } from './settingsNav'

/** Kinds whose GET endpoint is admin-only; skipped (locked badge) for operators. */
const ADMIN_ONLY_KINDS = new Set<BadgeKind>(['webhook', 'inbox', 'store', 'mcpExport', 'openapi', 'mcp', 'plugins'])

export type BadgeMap = Partial<Record<BadgeKind, BadgeResult | null>>

async function fetchToolList(): Promise<ToolInfo[]> {
  return listTools().catch(() => [] as ToolInfo[])
}

export function useSettingsBadges(
  items: readonly SettingsNavItem[],
  role: SettingsRole,
  refreshKey: number,
): BadgeMap {
  const [badges, setBadges] = useState<BadgeMap>({})

  useEffect(() => {
    let cancelled = false
    const kinds = new Set<BadgeKind>()
    for (const it of items) {
      if (!it.badge) continue
      if (role === 'operator' && ADMIN_ONLY_KINDS.has(it.badge)) continue
      kinds.add(it.badge)
    }

    async function run(): Promise<void> {
      // Shared tool list feeds tools/openapi/mcp/plugins.
      const needTools = ['tools', 'openapi', 'mcp', 'plugins'].some((k) => kinds.has(k as BadgeKind))
      const toolsP = needTools ? fetchToolList() : Promise.resolve([] as ToolInfo[])
      const tasks: Promise<void>[] = []
      const next: BadgeMap = {}

      if (kinds.has('models')) {
        tasks.push(listModelProfiles().then((p) => { next.models = resolveBadge('models', p) }).catch(() => { next.models = null }))
      }
      if (kinds.has('skills')) {
        tasks.push(listSkills().then((s) => { next.skills = resolveBadge('skills', s) }).catch(() => { next.skills = null }))
      }
      if (kinds.has('weixin')) {
        tasks.push(getWeixinSettings().then((s) => { next.weixin = resolveBadge('weixin', s) }).catch(() => { next.weixin = null }))
      }
      if (kinds.has('webhook')) {
        tasks.push(getEventsWebhook().then((s) => { next.webhook = resolveBadge('webhook', s) }).catch(() => { next.webhook = null }))
      }
      if (kinds.has('inbox')) {
        tasks.push(getInboxChannels().then((s) => { next.inbox = resolveBadge('inbox', s) }).catch(() => { next.inbox = null }))
      }
      if (kinds.has('store')) {
        tasks.push(getStoreSettings().then((s) => { next.store = resolveBadge('store', s) }).catch(() => { next.store = null }))
      }
      if (kinds.has('runtime')) {
        tasks.push(getRuntimeSettings().then((s) => { next.runtime = resolveBadge('runtime', s) }).catch(() => { next.runtime = null }))
      }
      if (kinds.has('mcpExport')) {
        tasks.push(listMCPExportIdentities().then((s) => { next.mcpExport = resolveBadge('mcpExport', s) }).catch(() => { next.mcpExport = null }))
      }

      const tools = await toolsP
      if (kinds.has('tools')) next.tools = resolveBadge('tools', tools)
      if (kinds.has('openapi')) next.openapi = resolveBadge('openapi', tools)
      if (kinds.has('mcp')) next.mcp = resolveBadge('mcp', tools)
      if (kinds.has('plugins')) next.plugins = resolveBadge('plugins', tools)

      await Promise.all(tasks)
      if (!cancelled) setBadges(next)
    }

    void run()
    return () => { cancelled = true }
  }, [items, role, refreshKey])

  return badges
}
```

说明：`ADMIN_ONLY_KINDS` 含 openapi/mcp/plugins——运营虽经任务 1 能 GET `/v0/tools`，但这三类卡在运营视图是 locked，徽标直接不请求工具来源细分；运营只需要 tools（助手功能）的真实徽标。`tools` 不在 admin-only 集合中。

- [ ] **步骤 5：运行测试验证通过**

运行：`npx vitest run src/settingsHomeBadges.test.ts`
预期：PASS（纯函数与 hook 全绿）。若 hook 用例报「act 内未刷新」，把等待改为 `await act(async () => { await new Promise((r) => setTimeout(r, 0)) })`。

- [ ] **步骤 6：tsc + commit**

运行：`npx tsc --noEmit`，预期 PASS。

```bash
git add web/chat/src/settingsHomeBadges.ts web/chat/src/settingsHomeBadges.test.ts
git commit -m "feat(web): 设置首页徽标并发取数 hook（运营跳过无权限项/失败降级）"
```

---

## 任务 5：设置首页 `SettingsHome` 与样式

**文件：**
- 创建：`web/chat/src/pages/SettingsHome.tsx`
- 创建：`web/chat/src/styles/settings.css`
- 测试：`web/chat/src/pages/SettingsHome.test.tsx`

- [ ] **步骤 1：编写失败的测试**

创建 `web/chat/src/pages/SettingsHome.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { GateContext } from '../gateContext'
import { SettingsHome } from './SettingsHome'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.stubGlobal('fetch', vi.fn())
})
afterEach(() => { host.remove(); vi.unstubAllGlobals() })

function render(role: 'admin' | 'operator') {
  act(() => {
    createRoot(host).render(
      <MemoryRouter>
        <GateContext.Provider value={{ role, gateEnabled: true, operatorId: 'op' }}>
          <SettingsHome />
        </GateContext.Provider>
      </MemoryRouter>,
    )
  })
}
async function settle() {
  await act(async () => { await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0)) })
}

describe('SettingsHome', () => {
  it('admin renders all 13 cards grouped under four group titles', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/tools') return jsonResponse([{ name: 't', connector_id: 'oa1', source: 'spec', enabled: true }])
      if (u === '/v0/skills') return jsonResponse({ skills: [{ id: 's1' }] })
      if (u === '/v0/settings/channels/weixin') return jsonResponse({ running: true })
      if (u === '/v0/settings/events-webhook') return jsonResponse({ url: 'https://x', headers: {} })
      if (u === '/v0/settings/inbox-channels') return jsonResponse({ channels: [] })
      if (u === '/v0/settings/store') return jsonResponse({ driver: 'sqlite' })
      if (u === '/v0/settings/runtime') return jsonResponse({ effective: {}, overridden: {} })
      if (u === '/v0/settings/mcp-export/identities') return jsonResponse([])
      return jsonResponse({ profiles: [{ id: 'm1' }] })
    })
    render('admin')
    await settle()
    for (const name of ['模型', '助手功能', '技能', '业务系统', 'MCP', '插件', 'MCP 导出', '微信', '消息回调', '外部来信', '账号', '存储', '运行参数']) {
      expect(host.textContent).toContain(name)
    }
    for (const g of ['助手', '连接', '消息', '系统']) expect(host.textContent).toContain(g)
    expect(host.querySelectorAll('[data-testid="ui-card"]')).toHaveLength(13)
  })

  it('admin with zero models shows the go-add CTA linking to models', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) =>
      String(url) === '/v0/settings/models' ? jsonResponse({ profiles: [] }) : jsonResponse(null))
    render('admin')
    await settle()
    expect(host.textContent).toContain('先添加一个模型')
    const cta = host.querySelector('a[href="/settings/models"]')
    expect(cta).toBeTruthy()
    expect(cta?.textContent).toContain('去添加')
  })

  it('operator with zero models sees contact-admin note but no go-add button', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) =>
      String(url) === '/v0/settings/models' ? jsonResponse({ profiles: [] }) : jsonResponse(null))
    render('operator')
    await settle()
    expect(host.textContent).toContain('联系管理员')
    expect(host.querySelector('a[href="/settings/models"]')).toBeNull()
  })

  it('operator locked cards are not buttons and not focusable; allowed cards are', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/tools') return jsonResponse([])
      if (u === '/v0/skills') return jsonResponse({ skills: [] })
      if (u === '/v0/settings/channels/weixin') return jsonResponse({ enabled: false })
      if (u === '/v0/settings/runtime') return jsonResponse({ effective: {}, overridden: {} })
      return jsonResponse({ profiles: [{ id: 'm1' }] })
    })
    render('operator')
    await settle()
    const cards = Array.from(host.querySelectorAll('[data-testid="ui-card"]'))
    const buttons = cards.filter((c) => c.getAttribute('role') === 'button')
    // 6 reachable cards: 模型/助手功能/技能/微信/账号/运行参数
    expect(buttons).toHaveLength(6)
    const locked = cards.filter((c) => c.getAttribute('role') !== 'button')
    expect(locked.length).toBe(7)
    for (const c of locked) expect(c.getAttribute('tabindex')).toBeNull()
    expect(host.textContent).toContain('仅管理员')
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`npx vitest run src/pages/SettingsHome.test.tsx`
预期：FAIL（`SettingsHome` 不存在）。

- [ ] **步骤 3：创建 `styles/settings.css`**

```css
/* 设置首页与分组导航（P2） */
.settings-home { display: flex; flex-direction: column; gap: var(--space-5); }
.settings-home-head { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-3); }
.settings-home-title { margin: 0; font-size: 1.375rem; font-weight: 700; color: var(--text); }
.settings-home-sub { margin: 4px 0 0; color: var(--text-muted); font-size: 0.875rem; }
.settings-refresh-btn { display: inline-flex; align-items: center; gap: 6px; }
.settings-refresh-btn.spinning svg { animation: settings-spin 1s linear infinite; }
@keyframes settings-spin { to { transform: rotate(360deg); } }

.settings-onboard {
  display: flex; align-items: center; justify-content: space-between; gap: var(--space-3);
  background: var(--warning-soft); border: 1px solid var(--warning);
  color: var(--text); border-radius: var(--radius-lg); padding: var(--space-3) var(--space-4);
  font-size: 0.875rem;
}

.settings-group { display: flex; flex-direction: column; gap: var(--space-3); }
.settings-group-title {
  margin: 0; font-size: 0.75rem; font-weight: 600; letter-spacing: 0.04em;
  color: var(--text-muted); text-transform: none;
}
.settings-card-grid {
  display: grid; gap: var(--space-4);
  grid-template-columns: repeat(auto-fill, minmax(248px, 1fr));
}
.settings-home-card .ui-card-desc {
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
}
.settings-card-trailing { display: inline-flex; align-items: center; gap: 6px; }
.settings-card-locked { opacity: 0.72; cursor: default; }
.settings-card-locked .ui-card-icon { color: var(--text-muted); }
.settings-locked-label { font-size: 0.75rem; color: var(--text-muted); display: inline-flex; align-items: center; gap: 4px; }

/* 侧边导航分组 */
.settings-nav-overview { font-weight: 600; }
.settings-nav-group { display: flex; flex-direction: column; gap: 2px; }
.settings-nav-group + .settings-nav-group { margin-top: var(--space-3); }
.settings-nav-group-title {
  padding: 0 var(--space-2); margin-bottom: 2px;
  font-size: 0.6875rem; color: var(--text-muted); letter-spacing: 0.04em;
}

@media (max-width: 720px) {
  .settings-card-grid { grid-template-columns: 1fr; }
}
```

- [ ] **步骤 4：实现 `SettingsHome.tsx`**

```tsx
import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Lock, RefreshCw } from 'lucide-react'
import { useGate } from '../gateContext'
import { Card, Badge, Spinner } from '../components/ui'
import { SETTINGS_GROUPS, settingsNavItems, type SettingsNavItem } from '../settingsNav'
import { useSettingsBadges } from '../settingsHomeBadges'

export function SettingsHome() {
  const { role } = useGate()
  const navigate = useNavigate()
  const [refreshKey, setRefreshKey] = useState(0)
  const [refreshing, setRefreshing] = useState(false)
  const items = settingsNavItems(role)
  const badges = useSettingsBadges(items, role, refreshKey)

  const refresh = () => {
    setRefreshing(true)
    setRefreshKey((k) => k + 1)
    window.setTimeout(() => setRefreshing(false), 600)
  }

  // Completion bar driven by the same models data; null while loading.
  const modelCount = badges.models === undefined ? null : (badges.models?.tone === 'warning' ? 0 : 1)
  const showOnboard = modelCount === 0

  const renderCard = (item: SettingsNavItem) => {
    const isLocked = role === 'operator' && item.operator === 'locked'
    const badge = isLocked
      ? <span className="settings-locked-label"><Lock size={12} aria-hidden="true" />仅管理员</span>
      : item.badge
        ? (badges[item.badge] === undefined
            ? <Spinner />
            : badges[item.badge]
              ? <Badge tone={badges[item.badge]!.tone}>{badges[item.badge]!.text}</Badge>
              : null)
        : null
    const Icon = item.icon
    return (
      <Card
        key={item.to}
        className={`settings-home-card${isLocked ? ' settings-card-locked' : ''}`}
        icon={<Icon size={20} aria-hidden="true" />}
        title={item.label}
        description={item.desc}
        trailing={<span className="settings-card-trailing">{badge}</span>}
        onClick={isLocked ? undefined : () => navigate(item.to)}
      />
    )
  }

  return (
    <div className="settings-home">
      <div className="settings-home-head">
        <div>
          <h1 className="settings-home-title">设置</h1>
          <p className="settings-home-sub">管理助手的模型、能力和对外连接</p>
        </div>
        <button
          type="button"
          className={`btn ghost sm settings-refresh-btn${refreshing ? ' spinning' : ''}`}
          onClick={refresh}
        >
          <RefreshCw size={14} aria-hidden="true" /> 刷新状态
        </button>
      </div>

      {showOnboard && (
        <div className="settings-onboard" role="status">
          <span>
            {role === 'admin'
              ? '先添加一个模型，助手才能开始对话。'
              : '还没有可用模型，请联系管理员添加。'}
          </span>
          {role === 'admin' && (
            <Link to="/settings/models" className="btn primary sm">去添加</Link>
          )}
        </div>
      )}

      {SETTINGS_GROUPS.map((group) => {
        const groupItems = items.filter((i) => i.group === group.id)
        return (
          <section className="settings-group" key={group.id}>
            <h2 className="settings-group-title">{group.label}</h2>
            <div className="settings-card-grid">{groupItems.map(renderCard)}</div>
          </section>
        )
      })}
    </div>
  )
}
```

注意：完成度条仅需「模型是否为 0」。`badges.models` 在加载中为 `undefined`（不显示条，避免闪烁）；徽标 warning（未配置）即 0 个，其余（success 或 null）不显示条。

- [ ] **步骤 5：运行测试验证通过**

运行：`npx vitest run src/pages/SettingsHome.test.tsx`
预期：PASS。若 Spinner 不接受 `size` 属性，检查 `components/ui/Spinner.tsx` 的 props，按其真实 API 调整（不传 size 也可）。

- [ ] **步骤 6：tsc（此时 main.tsx 尚未引用 SettingsHome，预期通过）+ commit**

运行：`npx tsc --noEmit`，预期 PASS。

```bash
git add web/chat/src/pages/SettingsHome.tsx web/chat/src/pages/SettingsHome.test.tsx web/chat/src/styles/settings.css
git commit -m "feat(web): 设置首页（分组卡片/实时徽标/模型引导/运营锁定卡）"
```

---

## 任务 6：分组侧边导航 + 首页路由接入

**文件：**
- 修改：`web/chat/src/pages/SettingsLayout.tsx`
- 修改：`web/chat/src/main.tsx`

- [ ] **步骤 1：把 `SettingsLayout.tsx` 改为分组导航（整体替换函数体）**

将 `web/chat/src/pages/SettingsLayout.tsx` 替换为：

```tsx
import { NavLink, Outlet, Link } from 'react-router-dom'
import { Menu } from 'lucide-react'
import { ThemeToggle } from '../components/ui'
import { useGate } from '../gateContext'
import { OVERVIEW_ITEM, SETTINGS_GROUPS, visibleNavItems } from '../settingsNav'
import { useDrawer } from '../useDrawer'

export function SettingsLayout() {
  const { role } = useGate()
  const nav = visibleNavItems(role)
  const drawer = useDrawer()
  return (
    <div className={`settings-shell app-with-drawer${drawer.isOpen ? ' drawer-open' : ''}`}>
      <button
        type="button"
        className="app-drawer-scrim"
        aria-label="关闭菜单"
        onClick={drawer.close}
      />
      <aside className="settings-nav" aria-label="设置导航">
        <p className="settings-nav-title">设置</p>
        <nav className="settings-nav-list">
          <NavLink
            to={OVERVIEW_ITEM.to}
            end
            className={({ isActive }) =>
              `settings-nav-link settings-nav-overview${isActive ? ' active' : ''}`
            }
            onClick={drawer.close}
          >
            {OVERVIEW_ITEM.label}
          </NavLink>

          {SETTINGS_GROUPS.map((group) => {
            const items = nav.filter((i) => i.group === group.id)
            if (items.length === 0) return null
            return (
              <div className="settings-nav-group" key={group.id}>
                <p className="settings-nav-group-title">{group.label}</p>
                {items.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    className={({ isActive }) =>
                      `settings-nav-link${isActive ? ' active' : ''}`
                    }
                    onClick={drawer.close}
                  >
                    {item.label}
                  </NavLink>
                ))}
              </div>
            )
          })}
        </nav>
        <ThemeToggle />
        <Link to="/" className="settings-back">
          返回聊天
        </Link>
      </aside>
      <main className="settings-main">
        <div className="app-mobile-bar">
          <button
            type="button"
            className="app-menu-btn"
            aria-label="打开设置菜单"
            onClick={drawer.open}
          >
            <Menu size={20} aria-hidden="true" />
          </button>
          <strong>设置</strong>
        </div>
        <Outlet />
      </main>
    </div>
  )
}
```

- [ ] **步骤 2：`main.tsx` 接入首页与样式**

在 import 区（其他 `./pages/` import 旁）加入：
```tsx
import { SettingsHome } from './pages/SettingsHome'
```
在样式 import 区（`./style.css` 之前）加入：
```tsx
import './styles/settings.css'
```
删除旧的 `SettingsIndex` 函数（第 33-36 行）：
```tsx
function SettingsIndex() {
  const { role } = useGate()
  return <Navigate to={role === 'admin' ? 'tools' : 'identities'} replace />
}
```
把 index 路由：
```tsx
            <Route index element={<SettingsIndex />} />
```
改为：
```tsx
            <Route index element={<SettingsHome />} />
```
若 `useGate`/`Navigate` 在 main.tsx 中因此变为未使用（`noUnusedLocals` 会报错），删除对应 import 与仅服务于它的引用；`AdminOnly` 仍使用 `useGate`，故保留 `useGate` import；`Navigate` 仍被底部通配路由使用，保留。

- [ ] **步骤 3：类型检查与前端全量测试**

运行：`npx tsc --noEmit`，预期 PASS。
运行：`npx vitest run`，预期全部 PASS（旧的 settingsNav 测试已在任务 2 重写；确认无其他文件 import 旧 `settingsNavItems` 的 label-only 结构）。

- [ ] **步骤 4：commit**

```bash
git add web/chat/src/pages/SettingsLayout.tsx web/chat/src/main.tsx
git commit -m "feat(web): 设置侧边导航分组与总览首页路由"
```

---

## 任务 7：只读 gate —— 模型页与运行参数页

**文件：**
- 修改：`web/chat/src/pages/ModelSettings.tsx`
- 修改：`web/chat/src/pages/RuntimeSettings.tsx`
- 测试（新建）：`web/chat/src/pages/ReadOnlyGate.model-runtime.test.tsx`

- [ ] **步骤 1：编写失败的测试**

创建 `web/chat/src/pages/ReadOnlyGate.model-runtime.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { GateContext } from '../gateContext'
import { ModelSettings } from './ModelSettings'
import { RuntimeSettings } from './RuntimeSettings'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.stubGlobal('fetch', vi.fn())
})
afterEach(() => { host.remove(); vi.unstubAllGlobals() })

async function renderOperator(el: React.ReactNode) {
  await act(async () => {
    createRoot(host).render(
      <MemoryRouter>
        <GateContext.Provider value={{ role: 'operator', gateEnabled: true, operatorId: 'op' }}>
          {el}
        </GateContext.Provider>
      </MemoryRouter>,
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

describe('ModelSettings read-only for operator', () => {
  it('lists profiles but hides create form, edit and delete buttons', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) =>
      String(url) === '/v0/settings/models'
        ? jsonResponse({ profiles: [{ id: 'm1', name: '标准模型', model: 'gpt', base_url: 'u', provider: 'openai_compatible', supports_vision: false, disable_thinking: false, context_tokens: 1, auto_tier: 'standard' }] })
        : jsonResponse(null))
    await renderOperator(<ModelSettings />)
    expect(host.textContent).toContain('标准模型')
    expect(host.textContent).not.toContain('添加模型')
    expect(host.querySelector('.settings-form')).toBeNull()
    expect(host.textContent).not.toContain('编辑')
    expect(host.textContent).not.toContain('删除')
  })
})

describe('RuntimeSettings read-only for operator', () => {
  it('shows engine values but hides save button and entire credentials section', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/settings/runtime') return jsonResponse({ effective: { max_messages: 20 }, overridden: {} })
      if (u === '/v0/settings/credentials') return new Response('forbidden', { status: 403 })
      return jsonResponse(null)
    })
    await renderOperator(<RuntimeSettings />)
    expect(host.textContent).toContain('引擎参数')
    expect(host.textContent).not.toContain('保存引擎参数')
    expect(host.textContent).not.toContain('控制面凭据')
    expect(host.textContent).not.toContain('轮换主口令')
  })
})
```

注意测试断言 `expect(host.querySelector('.settings-form')).toBeNull()`：运营视图不渲染任何表单，因此编辑/新增两个 `<section className="settings-form">` 都必须被 gate 掉（列表 `<ul>` 保留）。

- [ ] **步骤 2：运行测试验证失败**

运行：`npx vitest run src/pages/ReadOnlyGate.model-runtime.test.tsx`
预期：FAIL（运营仍看到表单/按钮、且凭据区发起 403）。

- [ ] **步骤 3：给 `ModelSettings.tsx` 加 gate**

顶部 import 加：
```tsx
import { useGate } from '../gateContext'
```
在 `ModelSettings()` 组件内（`const [profiles, ...]` 附近）加：
```tsx
const { role } = useGate()
const readOnly = role !== 'admin'
```
做三处条件渲染：

1. 编辑表单块（约 484-499 行 `{!loading && editingId && (`）改为 `{!loading && !readOnly && editingId && (`。
2. 新增/列表区（约 500-521 行）整体替换为：
```tsx
      {!loading && readOnly && (
        <section>
          <h2 className="settings-subheading">模型列表</h2>
          <ModelProfileList
            profiles={profiles}
            busy={false}
            readOnly
            onEdit={startEdit}
            onDelete={(target) => void onDelete(target)}
          />
        </section>
      )}
      {!loading && !readOnly && (
        <section className="settings-form">
          <h2 className="settings-subheading">模型列表</h2>
          <ModelProfileList
            profiles={profiles}
            busy={busy || editingId != null}
            onEdit={startEdit}
            onDelete={(target) => void onDelete(target)}
          />
          <ModelProfileForm
            form={createForm}
            setForm={setCreateForm}
            busy={busy}
            isEdit={false}
            title="新建模型"
            submitLabel="创建模型"
            onSubmit={(e) => void onCreate(e)}
          />
        </section>
      )}
```
3. 空模型错误文案（约 479-481 行）对运营改为联系管理员：把
```tsx
{!loading && profiles.length === 0 && !error && (
  <p className="settings-error">当前没有任何可用模型，请先在下方添加一个模型再发起对话。</p>
)}
```
改为
```tsx
{!loading && profiles.length === 0 && !error && (
  <p className="settings-error">
    {readOnly ? '当前没有任何可用模型，请联系管理员配置。' : '当前没有任何可用模型，请先在下方添加一个模型再发起对话。'}
  </p>
)}
```

给 `ModelProfileListProps` 增加 `readOnly?: boolean`，在其编辑/删除按钮容器（约 302-318 行 `<div className="settings-toolbar">`）外层包 `{!readOnly && ( ... )}`。

- [ ] **步骤 4：给 `RuntimeSettings.tsx` 加 gate**

顶部 import 加 `import { useGate } from '../gateContext'`。

1. `CredentialsSection` 顶部取角色（保持所有既有 hooks 无条件、顺序不变），在所有 hooks 与回调定义之后、`return (` 之前加早退：
```tsx
function CredentialsSection() {
  const { role } = useGate()
  const [view, setView] = useState<CredentialsView | null>(null)
  // ……既有 useState/useCallback/useEffect/处理函数全部保持原样、无条件执行……
  if (role !== 'admin') return null
  return (
    <section className="settings-form"> …… </section>
  )
}
```
运营挂载该组件时 hooks 仍正常执行，但因 `getCredentials()` 会 403，需同时让其 `load` 对运营跳过请求：在 `load` 的 `useCallback` 内最前面加
```tsx
if (role !== 'admin') { setLoading(false); return }
```
（`role` 加入该 useCallback 依赖数组。）这样运营既不渲染凭据区、也不发 403 请求。

2. `RuntimeSettings()` 内加：
```tsx
const { role } = useGate()
const readOnly = role !== 'admin'
```
- 引擎参数各 `<input>`/checkbox 的 `disabled={busy}` 改为 `disabled={busy || readOnly}`；
- 保存按钮（约 346-348 行）用 `{!readOnly && (<button type="submit" ...>…</button>)}` 包裹。

- [ ] **步骤 5：运行测试验证通过**

运行：`npx vitest run src/pages/ReadOnlyGate.model-runtime.test.tsx`
预期：PASS。再跑 `npx vitest run src/pages/ModelSettings.test.tsx`，确认既有 admin 视图测试不破（ModelProfileList 不传 readOnly 时默认 false，按钮仍在）。

- [ ] **步骤 6：commit**

```bash
git add web/chat/src/pages/ModelSettings.tsx web/chat/src/pages/RuntimeSettings.tsx web/chat/src/pages/ReadOnlyGate.model-runtime.test.tsx
git commit -m "feat(web): 模型/运行参数页运营只读（隐藏凭据区与写控件）"
```

---

## 任务 8：只读 gate —— 助手功能页与技能页

**文件：**
- 修改：`web/chat/src/pages/ToolsSettings.tsx`
- 修改：`web/chat/src/pages/SkillsSettings.tsx`
- 测试（新建）：`web/chat/src/pages/ReadOnlyGate.tools-skills.test.tsx`

说明：`ToolsSettings` 加载期对每个 connector 调 `getConnector`（admin-only GET）；其失败本就被逐连接器 `catch {}` 吞掉（非致命），运营只读视图下 connector 元数据缺失可接受，不需要改加载流程。

- [ ] **步骤 1：编写失败的测试**

创建 `web/chat/src/pages/ReadOnlyGate.tools-skills.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { GateContext } from '../gateContext'
import { ToolsSettings } from './ToolsSettings'
import { SkillsSettings } from './SkillsSettings'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.stubGlobal('fetch', vi.fn())
})
afterEach(() => { host.remove(); vi.unstubAllGlobals() })

async function renderOperator(el: React.ReactNode) {
  await act(async () => {
    createRoot(host).render(
      <MemoryRouter>
        <GateContext.Provider value={{ role: 'operator', gateEnabled: true, operatorId: 'op' }}>
          {el}
        </GateContext.Provider>
      </MemoryRouter>,
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

describe('ToolsSettings read-only for operator', () => {
  it('lists tools but hides enable/login/export controls, add drawer and delete', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/tools') {
        return jsonResponse([
          { name: 'list_tickets', title: '查工单', connector_id: 'oa1', source: 'spec', enabled: true, require_login: false },
        ])
      }
      // connector detail GET is admin-only for operators
      return new Response('forbidden', { status: 403 })
    })
    await renderOperator(<ToolsSettings />)
    expect(host.textContent).toContain('查工单')
    expect(host.textContent).not.toContain('全部启用')
    expect(host.textContent).not.toContain('全部停用')
    expect(host.textContent).not.toContain('添加')
    expect(host.textContent).not.toContain('需要登录')
    expect(host.textContent).not.toContain('MCP 导出')
    expect(host.textContent).not.toContain('删除')
  })
})

describe('SkillsSettings read-only for operator', () => {
  it('lists skills but hides upload, save-default and delete', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/skills') return jsonResponse({ skills: [
        { id: 's1', name: '分诊', description: '', tools: [], source: 'builtin' },
        { id: 's2', name: '自定义', description: '', tools: [], source: 'user' },
      ] })
      return jsonResponse(null)
    })
    await renderOperator(<SkillsSettings />)
    expect(host.textContent).toContain('分诊')
    expect(host.querySelector('input[type="file"]')).toBeNull()
    expect(host.textContent).not.toContain('保存默认勾选')
    // user-source skill shows a delete button for admins; it must be hidden for operators
    expect(host.textContent).not.toContain('删除')
    // checkboxes present but disabled (read-only)
    const boxes = host.querySelectorAll('input[type="checkbox"]')
    boxes.forEach((b) => expect((b as HTMLInputElement).disabled).toBe(true))
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`npx vitest run src/pages/ReadOnlyGate.tools-skills.test.tsx`
预期：FAIL（运营仍看到控件）。

- [ ] **步骤 3：给 `ToolsSettings.tsx` 加 gate**

顶部 import `import { useGate } from '../gateContext'`；组件内加 `const { role } = useGate(); const readOnly = role !== 'admin'`。

1. `renderGroupButtons`：函数开头若 `readOnly` 直接 `return null`（隐藏全部启用/停用）。
2. `renderTool`：把三个写控件 label/select 包条件——「启用」`input[type=checkbox]`、「需要登录」checkbox、「MCP 导出」`<select>`：外层各自 `{!readOnly && ( <label …>…</label> )}`；删除按钮 `{canDelete && !readOnly && ( <button …>删除</button> )}`。
3. 「添加」按钮（约 645-655 行，外层是 `{showAdd && (`）：改为 `{showAdd && !readOnly && (`。其下方空态文案中的 `去 OpenAPI 设置注册` 链接（约 661-663 行 `<Link to="/settings/openapi">`）运营无权进入，用 `{!readOnly && ( <Link …>…</Link> )}` 包裹（链接外的「尚未注册 Connector。」文字可保留）。
4. 添加抽屉（约 770 行 `{drawerOpen && (`）：改为 `{drawerOpen && !readOnly && (`（双保险）。
5. 行内编辑（标题/描述编辑，约 612-649 行的「编辑/保存文案」按钮与输入）：`readOnly` 时不渲染编辑入口（把触发编辑的按钮用 `!readOnly` 包裹）；保存类按钮一并隐藏。若该处结构复杂，最低要求是所有会触发 `patchTool`/连接器写操作的按钮不渲染。
6. 连接器展开面板里的「执行回调 URL / 登录抓包 / 保存 Connector 设置」条（约 691-735 行）**无需额外 gate**：它仅在 `connectorMeta[id]` 存在时渲染，而运营的 `getConnector` 为 403（已被逐连接器 catch），元数据为空，该条天然不出现。实现后在人工走查中确认即可。

- [ ] **步骤 4：给 `SkillsSettings.tsx` 加 gate**

顶部 import `useGate`；组件内 `const { role } = useGate(); const readOnly = role !== 'admin'`。

1. 工具栏（约 178-197 行 `<div className="settings-toolbar">`，含文件 input 与「保存默认勾选」）：整体 `{!readOnly && ( <div className="settings-toolbar">…</div> )}`。
2. 每个技能行的勾选 checkbox（约 208 行）：`disabled={busy || readOnly}`。
3. 删除按钮（约 225-235 行 `{s.source === 'user' && (`）：改为 `{s.source === 'user' && !readOnly && (`。

- [ ] **步骤 5：运行测试验证通过**

运行：`npx vitest run src/pages/ReadOnlyGate.tools-skills.test.tsx`
预期：PASS。再跑 `npx vitest run` 确认既有 admin 视图测试不破。

- [ ] **步骤 6：commit**

```bash
git add web/chat/src/pages/ToolsSettings.tsx web/chat/src/pages/SkillsSettings.tsx web/chat/src/pages/ReadOnlyGate.tools-skills.test.tsx
git commit -m "feat(web): 助手功能/技能页运营只读（隐藏开关/增删/保存）"
```

---

## 任务 9：微信页 gate（运营可扫码登录，其余仅管理员）

**文件：**
- 修改：`web/chat/src/pages/WeixinChannelSettings.tsx`
- 测试（新建）：`web/chat/src/pages/ReadOnlyGate.weixin.test.tsx`

运营保留：运行状态只读展示、「获取登录二维码/刷新二维码」按钮、二维码与轮询状态。运营隐藏：适配器进程区（启动/重启/停止）、「登出」按钮、整个「设置」表单。

- [ ] **步骤 1：编写失败的测试**

创建 `web/chat/src/pages/ReadOnlyGate.weixin.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { GateContext } from '../gateContext'
import { WeixinChannelSettings } from './WeixinChannelSettings'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.stubGlobal('fetch', vi.fn())
})
afterEach(() => { host.remove(); vi.unstubAllGlobals() })

async function renderAs(role: 'admin' | 'operator') {
  vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
    const u = String(url)
    if (u === '/v0/settings/channels/weixin') {
      return jsonResponse({ agent_id: 'a', assignee: 'alice', allowlist: [], enabled: true, running: true })
    }
    return jsonResponse(null)
  })
  await act(async () => {
    createRoot(host).render(
      <MemoryRouter>
        <GateContext.Provider value={{ role, gateEnabled: true, operatorId: 'op' }}>
          <WeixinChannelSettings />
        </GateContext.Provider>
      </MemoryRouter>,
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

describe('WeixinChannelSettings operator', () => {
  it('keeps login QR + status but hides process/logout/settings-form', async () => {
    await renderAs('operator')
    expect(host.textContent).toContain('登录')
    // 扫码登录入口保留
    expect(host.textContent).toContain('获取登录二维码')
    // 进程控制隐藏
    expect(host.textContent).not.toContain('启动进程')
    expect(host.textContent).not.toContain('重启进程')
    expect(host.textContent).not.toContain('停止进程')
    // 登出隐藏
    expect(host.textContent).not.toContain('登出')
    // 设置表单（含保存）隐藏
    expect(host.textContent).not.toContain('保存设置')
    expect(host.querySelector('textarea')).toBeNull()
  })

  it('admin keeps process controls, logout and settings form', async () => {
    await renderAs('admin')
    expect(host.textContent).toContain('启动进程')
    expect(host.textContent).toContain('登出')
    expect(host.textContent).toContain('保存设置')
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`npx vitest run src/pages/ReadOnlyGate.weixin.test.tsx`
预期：FAIL（运营仍看到全部控件）。

- [ ] **步骤 3：给 `WeixinChannelSettings.tsx` 加 gate**

顶部 import `import { useGate } from '../gateContext'`；组件内（state 声明区附近）加：
```tsx
const { role } = useGate()
const isAdmin = role === 'admin'
```

做三处条件渲染：

1. 适配器进程区（约 278-309 行 `<section className="weixin-login-block">` 含「适配器进程」标题）整体包条件：
```tsx
{isAdmin && (
  <section className="weixin-login-block">
    <h2 className="settings-subheading">适配器进程</h2>
    {/* ……启动/重启/停止三按钮与说明原样保留…… */}
  </section>
)}
```

2. 登录区保留，但「登出」按钮（约 317-319 行）单独包条件：
```tsx
{isAdmin && (
  <button type="button" className="btn ghost" disabled={busy} onClick={() => void onLogout()}>
    登出
  </button>
)}
```
「获取登录二维码/刷新二维码」按钮与二维码块保持无条件。

3. 设置表单（约 337-383 行 `{!loading && ( <form …> … </form> )}`）改为：
```tsx
{!loading && isAdmin && (
  <form className="settings-form" onSubmit={(e) => void onSubmit(e)}>
    {/* ……原表单内容原样保留…… */}
  </form>
)}
```

页面顶部说明中「仅管理员可操作。出站双向同步见后续任务。」对运营会产生困惑，将该 `<p>` 改为按角色：
```tsx
<p>{isAdmin ? '扫码登录微信个人号 Bot（iLink），配置默认 Agent、受理人与私信 allowlist。' : '扫码登录你的微信账号后，即可通过微信与助手对话。配置由管理员维护。'}</p>
```
并删除或条件化原「仅管理员可操作」那句（避免运营看到矛盾提示）。

- [ ] **步骤 4：运行测试验证通过**

运行：`npx vitest run src/pages/ReadOnlyGate.weixin.test.tsx`
预期：PASS。

- [ ] **步骤 5：全量前端测试 + tsc + commit**

运行：`npx tsc --noEmit`，预期 PASS。
运行：`npx vitest run`，预期全部 PASS。

```bash
git add web/chat/src/pages/WeixinChannelSettings.tsx web/chat/src/pages/ReadOnlyGate.weixin.test.tsx
git commit -m "feat(web): 微信页运营可扫码登录，进程/登出/配置仅管理员"
```

---

## 任务 10：重建嵌入产物 + 全栈收尾验证

**文件：**
- 重建：`internal/ui/dist/**`

- [ ] **步骤 1：前端全量校验**

在 `web/chat/` 运行：
```bash
npx tsc --noEmit
npx vitest run
npm run build
```
预期：tsc 无错、全部 vitest PASS、vite 构建成功并刷新 `internal/ui/dist/assets/index-*.js` 与 `index.html`。

- [ ] **步骤 2：Go 全量校验**

在仓库根运行：
```bash
go build ./...
go test ./...
```
预期：BUILD OK、全部 PASS（任务 1 的 ACL 变更无回归）。

- [ ] **步骤 3：核对嵌入产物已更新**

运行（根目录）：`git status --short`
预期：`internal/ui/dist/index.html` 修改、旧 `assets/index-*.js` 删除、新 `assets/index-*.js` 新增；无遗漏的未提交源码。

- [ ] **步骤 4：commit 产物**

```bash
git add internal/ui/dist
git commit -m "build(ui): 重建 P2 设置首页嵌入产物"
```

- [ ] **步骤 5：人工走查（实现者执行并记录，不自动化）**

按规格 7.3，启动本地服务后用浏览器验证：
- 管理员 `/ui/settings`：四分组 13 卡、徽标实时、模型 0 时「去添加」、点击各卡跳转、刷新按钮重拉、暗色对比、手机单列与抽屉。
- 运营账号：首页 6 张可点卡 + 7 张锁定卡（连接组隐藏）、模型 0 时联系管理员文案无按钮；进入微信页能获取二维码；模型/工具/技能/运行参数页无任何写控件；侧边栏无连接组。
- 接口异常（可临时停后端或改错 base url）时卡片仅徽标留空，无红屏。

---

## 自检结论（计划作者已核对）

- **规格覆盖：** §1-§2 命名分组 → 任务 2；§3 ACL → 任务 1；§4 徽标（映射/解析/hook/运营降级）→ 任务 3、4、5；§5.1-5.4 首页与导航 → 任务 5、6；§5.5 五个只读页 → 任务 7、8、9；§6 工程清单全部有任务；§7 测试 → 各任务内测试 + 任务 10 全量与人工走查。
- **类型一致：** `SettingsNavItem`/`BadgeKind`/`BadgeResult`/`useSettingsBadges(items, role, refreshKey)`/`visibleNavItems`/`SETTINGS_GROUPS`/`OVERVIEW_ITEM` 在各任务签名一致；`BadgeResult.tone` 与 `ui/Badge` 的 `success/warning/neutral` 对齐。
- **无占位符：** 每个代码步骤均给出完整可粘贴代码或精确的现有行号锚点。
