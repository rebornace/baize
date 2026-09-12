# P3-C 外部工具服务（MCP）迁入连接器外壳 + 对外提供能力页人话化 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 把「外部工具服务（MCP）」接入页迁入 P3-B 的连接器外壳/两步编辑器（stdio/HTTP 两种连接方式 + 工具审批），并把「对外提供能力（MCP 导出）」页在不改后端 API 的前提下全面人话化。

**架构：** 前端纯改造（React + TypeScript + Vitest，无后端 Go 改动）。扩展 `ConnectorKind='mcp'`、判别联合 `SavedConnection` 统一三类连接的第一步/第二步提交；新增共享 `Textarea` 原语与 `connectorForms/lines.ts`、`mcp.ts` 纯函数；MCP 第二步只保留「需人工审批」。MCP 每次 PUT 都会实时重跑发现，故第二步保存权限必须回传完整 `mcp` 配置（由页面缓存第一步负载实现）。

**技术栈：** React 18、TypeScript、Vite、Vitest + jsdom + RTL 无关（项目用 `react-dom/client` 直接渲染）、lucide-react。测试命令 `npm test`（= `vitest run`），类型 `npx tsc --noEmit`，构建 `npm run build`（`tsc && vite build`），目录 `web/chat`。

规格：`docs/superpowers/specs/2026-09-12-webui-refresh-p3c-mcp-pages-design.md`

---

## 文件结构

新增：

- `web/chat/src/pages/connectorForms/lines.ts` — 多行文本解析纯函数（从旧 `McpSettings.tsx` 下沉）。
- `web/chat/src/pages/connectorForms/lines.test.ts`
- `web/chat/src/pages/connectorForms/mcp.ts` — MCP 表单类型、校验、连接器↔表单、行摘要纯函数。
- `web/chat/src/pages/connectorForms/mcp.test.ts`
- `web/chat/src/components/ui/Textarea.tsx` 与 `.test.tsx`

修改：

- `pages/connectorForms/types.ts` — `ConnectorKind` 增 `'mcp'`；新增 `SavedConnection` 判别联合。
- `components/ui/index.ts`、`styles/components.css` — 导出/样式 Textarea。
- `strings.ts` — MCP 连接文案、导出页文案、`invalid_mcp` 错误映射。
- `components/settings/ConnectorShell.tsx`（+测试）— mcp 标题/空态/`summary` 行。
- `components/settings/ConnectorEditorModal.tsx`（+测试）— 通用 `SavedConnection`、MCP 第一步字段、第二步仅审批列、`initial.mcp`。
- `pages/OpenApiSettings.tsx`、`pages/PluginSettings.tsx`（+其测试随签名更新）— 缓存连接负载供第二步回传。
- `pages/McpSettings.tsx`（重写为薄页）、`pages/McpSettings.test.tsx`（重写）。
- `pages/McpExportSettings.tsx`（人话化）、`pages/McpExportSettings.test.tsx`（补交互）。
- `pages/InboxSettings.tsx`、`pages/WebhookSettings.tsx` — 改引 `lines.ts`。
- `settingsNav.ts` — 两条导航展示名/描述。
- `internal/ui/dist/**` — 末尾重建产物。

删除：

- `pages/connectorDelete.ts`、`pages/connectorDelete.test.ts`（MCP 迁外壳后无引用）。
- 旧 `McpSettings.tsx` 内抽屉/`settings-mcp*` 标记随重写消失。

注意：`.settings-drawer*` CSS 仍被 `InboxSettings.tsx`、`ToolsSettings.tsx` 使用，**本批不删除**。

---

## 任务 1：下沉多行解析纯函数到 `connectorForms/lines.ts`

把 `McpSettings.tsx` 里的多行解析函数原样搬到共享模块，并把三个引用页改到新位置；旧 `McpSettings.tsx` 在本任务先改为从新模块 re-export，保证旧测试不破（任务 6 重写页面时再删 re-export）。

**文件：**
- 创建：`web/chat/src/pages/connectorForms/lines.ts`
- 测试：`web/chat/src/pages/connectorForms/lines.test.ts`
- 修改：`pages/McpSettings.tsx`（改为 re-export）、`pages/InboxSettings.tsx:12`、`pages/WebhookSettings.tsx:11`、`pages/McpExportSettings.tsx:16`

- [ ] **步骤 1：编写失败测试** `lines.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import {
  formatKeyValueMap, formatStringList, parseArgsText, parseKeyValueLines, parseLineList,
} from './lines'

describe('parseKeyValueLines', () => {
  it('parses KEY=VALUE lines and trims', () => {
    const r = parseKeyValueLines('A=1\n B = two \n\n')
    expect(r).toEqual({ ok: true, value: { A: '1', B: 'two' } })
  })
  it('empty text becomes an empty map', () => {
    expect(parseKeyValueLines('   ')).toEqual({ ok: true, value: {} })
  })
  it('rejects a line without = or with empty key', () => {
    expect(parseKeyValueLines('nope')).toMatchObject({ ok: false })
    expect(parseKeyValueLines('=v')).toMatchObject({ ok: false })
  })
})

describe('list formatting / parsing', () => {
  it('formatKeyValueMap round trips', () => {
    expect(formatKeyValueMap({ A: '1', B: 'x' })).toBe('A=1\nB=x')
    expect(formatKeyValueMap(undefined)).toBe('')
  })
  it('formatStringList joins lines / empty', () => {
    expect(formatStringList(['a', 'b'])).toBe('a\nb')
    expect(formatStringList(undefined)).toBe('')
  })
  it('parseArgsText splits on whitespace single-line and on newlines multi-line', () => {
    expect(parseArgsText('-y pkg')).toEqual(['-y', 'pkg'])
    expect(parseArgsText('-y\npkg\n')).toEqual(['-y', 'pkg'])
    expect(parseArgsText('  ')).toEqual([])
  })
  it('parseLineList keeps one entry per non-empty line', () => {
    expect(parseLineList('a\n b \n\n')).toEqual(['a', 'b'])
  })
})
```

- [ ] **步骤 2：运行确认失败**：`cd web/chat; npx vitest run src/pages/connectorForms/lines.test.ts`，预期 FAIL（找不到模块）。

- [ ] **步骤 3：创建 `lines.ts`**（行为与现有函数完全一致，仅移动；错误文案沿用现有中文，避免改 Inbox/Webhook 行为）：

```ts
/** 多行 KEY=VALUE 文本 <-> 对象；空文本得到空对象，非法行返回中文错误。 */
export function parseKeyValueLines(text: string): { ok: true; value: Record<string, string> } | { ok: false; message: string } {
  const trimmed = text.trim()
  if (trimmed === '') return { ok: true, value: {} }
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const row = line.trim()
    if (row === '') continue
    const eq = row.indexOf('=')
    if (eq <= 0) return { ok: false, message: `无效键值行：${row}` }
    const key = row.slice(0, eq).trim()
    const value = row.slice(eq + 1).trim()
    if (!key) return { ok: false, message: `无效键值行：${row}` }
    out[key] = value
  }
  return { ok: true, value: out }
}

export function formatKeyValueMap(map: Record<string, string> | undefined): string {
  if (!map) return ''
  return Object.entries(map).map(([k, v]) => `${k}=${v}`).join('\n')
}

export function formatStringList(list: string[] | undefined): string {
  if (!list || list.length === 0) return ''
  return list.join('\n')
}

/** 单行按空白拆，多行按每行一个拆（兼容旧 args 输入习惯）。 */
export function parseArgsText(text: string): string[] {
  const trimmed = text.trim()
  if (trimmed === '') return []
  if (trimmed.includes('\n')) {
    return trimmed.split('\n').map((s) => s.trim()).filter((s) => s !== '')
  }
  return trimmed.split(/\s+/).filter((s) => s !== '')
}

export function parseLineList(text: string): string[] {
  return text.split('\n').map((s) => s.trim()).filter((s) => s !== '')
}
```

- [ ] **步骤 4：改引用**：
  - `InboxSettings.tsx:12`、`WebhookSettings.tsx:11`、`McpExportSettings.tsx:16`：这三个文件都在 `pages/` 目录，统一把 `from './McpSettings'` 改为 `from './connectorForms/lines'`（仅引入它们各自用到的 `formatKeyValueMap`、`parseKeyValueLines`）。
  - `McpSettings.tsx`：删除这 5 个函数本体，在文件顶部加 `export { formatKeyValueMap, parseArgsText, parseKeyValueLines, parseLineList } from './connectorForms/lines'`（`mcpConnectorIds`/`validateMcpForm` 等本任务暂不动，旧测试仍依赖这些 re-export 与本文件函数）。

- [ ] **步骤 5：运行相关测试与类型检查**：`npx vitest run src/pages/connectorForms/lines.test.ts src/pages/McpSettings.test.ts src/pages/McpExportSettings.test.tsx`，再 `npx tsc --noEmit`；预期全绿。

- [ ] **步骤 6：Commit**：`git add -A; git commit -m "refactor(web): 下沉连接器多行解析函数到 connectorForms/lines"`

---

## 任务 2：共享 `Textarea` 原语

**文件：**
- 创建：`web/chat/src/components/ui/Textarea.tsx`、`Textarea.test.tsx`
- 修改：`components/ui/index.ts`、`styles/components.css`

- [ ] **步骤 1：编写失败测试** `Textarea.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Textarea } from './Textarea'

let host: HTMLDivElement
let root: ReturnType<typeof createRoot>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); root = createRoot(host) })
afterEach(() => { act(() => root.unmount()); host.remove() })

it('renders controlled value, disabled and invalid', async () => {
  const onChange = vi.fn()
  await act(async () => {
    root.render(<Textarea value="A=1" rows={3} disabled invalid onChange={onChange} />)
    await Promise.resolve()
  })
  const el = host.querySelector('textarea')!
  expect(el.value).toBe('A=1')
  expect(el.disabled).toBe(true)
  expect(el.rows).toBe(3)
  expect(el.className).toContain('invalid')
})
```

- [ ] **步骤 2：运行确认失败**：`npx vitest run src/components/ui/Textarea.test.tsx`，FAIL（模块不存在）。

- [ ] **步骤 3：实现 `Textarea.tsx`**（仿 `Input.tsx`，套用 `ui-input` 的高度/padding 不合适，故用新类 `ui-textarea`，复用同一套令牌）：

```tsx
import type { TextareaHTMLAttributes } from 'react'

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  invalid?: boolean
}

export function Textarea({ invalid, className = '', ...rest }: TextareaProps) {
  const invalidCls = invalid ? ' invalid' : ''
  return <textarea className={`ui-textarea${invalidCls}${className ? ` ${className}` : ''}`} {...rest} />
}
```

`components/ui/index.ts` 末尾加：`export { Textarea, type TextareaProps } from './Textarea'`

`styles/components.css` 在 `.ui-input.invalid:focus-visible { ... }` 规则块之后追加（只许用令牌）：

```css
.ui-textarea {
  width: 100%;
  box-sizing: border-box;
  padding: 0.5rem 0.625rem;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  line-height: 1.5;
  resize: vertical;
  min-height: 72px;
}
.ui-textarea:focus-visible {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.ui-textarea:disabled {
  background: var(--surface-2);
  color: var(--text-muted);
  cursor: not-allowed;
}
.ui-textarea.invalid { border-color: var(--danger); }
.ui-textarea.invalid:focus-visible { box-shadow: 0 0 0 3px var(--danger-soft); }
```

- [ ] **步骤 4：运行确认通过**：`npx vitest run src/components/ui/Textarea.test.tsx`，PASS。

- [ ] **步骤 5：Commit**：`git add -A; git commit -m "feat(web): 新增共享 Textarea 表单原语"`

---

## 任务 3：扩展类型与统一连接负载 `SavedConnection`，补齐 strings

**文件：**
- 修改：`pages/connectorForms/types.ts`
- 修改：`strings.ts`（仅 MCP 连接相关文案与 `invalid_mcp` 映射；导出页 `MCP_EXPORTS` 在任务 8 定义）
- 测试：`strings.connector.test.ts`（追加，若不存在则创建）

- [ ] **步骤 1：失败测试**（`strings.connector.test.ts` 追加用例）：

```ts
it('maps invalid_mcp to a human title', () => {
  expect(connectorErrorText(new ApiError(400, 'invalid_mcp', 'x')).title).toBe(CONNECTORS.errMcpConnect)
})
it('maps reachable invalid_mcp backend messages', () => {
  expect(connectorErrorText(new ApiError(400, 'invalid_mcp', 'invalid_mcp: mcp.command is required')).title).toContain('连接配置不完整')
  expect(connectorErrorText(new ApiError(400, 'invalid_mcp', 'invalid_mcp: unsupported mcp transport: ws')).title).toContain('连接配置不完整')
  expect(connectorErrorText(new ApiError(400, 'invalid_mcp', 'invalid_mcp: mcp.url is required')).title).toContain('远程服务地址')
})
```

并在同文件确认 `import { CONNECTORS, connectorErrorText, permissionSummary } from './strings'` 已含 `CONNECTORS`。

- [ ] **步骤 2：运行确认失败**：`npx vitest run src/strings.connector.test.ts`，FAIL（`errMcpConnect` 不存在）。

- [ ] **步骤 3：改 `types.ts`**（整文件替换为）：

```ts
import type { ImportFormat, MCPConfig } from '../../api'

export type ConnectorKind = 'openapi' | 'plugin' | 'mcp'

export interface ConnectionFormValues {
  kind: ConnectorKind
  id: string
  baseUrl: string
  /** 业务系统：本次是否提供了新文档（文件内容或非空 URL）。 */
  hasSpec: boolean
  /** 编辑既有连接时为 true，允许不带新文档保存。 */
  editing?: boolean
}

/** 旧两页第一步字段错误键；MCP 使用 McpFieldErrors。 */
export type FieldErrors = Partial<Record<'id' | 'baseUrl' | 'spec', string>>

/** 第一步保存成功后的判别联合连接负载（供第二步整表回传）。 */
export type SavedConnection =
  | { kind: 'openapi'; id: string; baseUrl: string; spec?: { content?: string; url?: string }; importFormat: ImportFormat }
  | { kind: 'plugin'; id: string; baseUrl: string }
  | { kind: 'mcp'; id: string; mcp: MCPConfig }

export type PermissionFlag = 'login' | 'approval'

export interface ToolPermission {
  login: boolean
  approval: boolean
}

/** key 为工具名。 */
export type PermissionSelection = Record<string, ToolPermission>
```

> 注意：`types.ts` 位于 `pages/connectorForms/`，到 `api.ts`（`src/api.ts`）的相对路径是 `'../../api'`。

- [ ] **步骤 4：在 `strings.ts` 的 `CONNECTORS` 常量内追加键**（插在 `errorGeneric` 之前）：

```ts
  // ---- MCP（外部工具服务） ----
  mcpTitle: '外部工具服务',
  mcpDesc: '接入标准 MCP 工具服务（本地子进程或远程 HTTP），扩展助手可用能力。',
  addMcp: '接入外部工具',
  editMcp: '编辑外部工具',
  mcpEmptyTitle: '还没有接入外部工具服务',
  mcpEmptyDesc: '接入遵循 MCP 标准的本地或远程工具服务后，助手即可调用其工具。',
  fieldTransport: '连接方式',
  transportStdio: '本地子进程（stdio）',
  transportHttp: '远程服务（Streamable HTTP）',
  fieldCommand: '启动命令',
  fieldCommandHint: '本地启动该工具服务的可执行命令，如 npx。',
  fieldArgs: '启动参数',
  fieldArgsHint: '空格分隔，或每行一个。',
  fieldEnv: '环境变量',
  fieldEnvHint: '每行一个 KEY=VALUE。密钥建议用 ${VAR}、env:VAR 或 file:路径 占位符，不要直接写死。',
  fieldUrl: '服务地址',
  fieldHeaders: '请求头',
  fieldHeadersHint: '每行一个 KEY=VALUE，例如 Authorization=Bearer ${TOKEN}。',
  errCommandRequired: '请填写启动命令',
  errMcpUrlRequired: '请填写服务地址',
  errMcpUrlHttp: '服务地址需以 http:// 或 https:// 开头',
  errEnvLine: (row: string) => `环境变量存在无法识别的行：${row}`,
  errHeadersLine: (row: string) => `请求头存在无法识别的行：${row}`,
  errMcpConnect: '无法连接到这个外部工具服务，请检查配置后重试。',
  permsIntroMcp: '勾选后，助手每次调用该工具前都会请你确认；不勾则直接执行。',
  mcpStdioSummary: (command: string) => `本地程序 · ${command}`,
  mcpHttpSummary: (url: string) => `远程服务 · ${url}`,
```

- [ ] **步骤 5：在 `connectorErrorText` 的 ApiError 分支、`CONNECTOR_CODE_TITLES` 查表之后，补充 invalid_mcp 专用细分分支**（放在 `base_url is required` 判断之前；**不要**把 `invalid_mcp` 加进 `CONNECTOR_CODE_TITLES`——否则顶部 byCode 提前返回会让下面按 message 的细分成为死代码）：

```ts
    if (e.code === 'invalid_mcp') {
      // 后端契约（internal/connector/apply.go、mcp/errors.go）：可达 message 为
      // "invalid_mcp: mcp.command is required" / "...mcp.url is required" /
      // "...unsupported mcp transport: X"；连接失败为 "invalid_mcp: <底层错误>"；
      // 空工具与配置解析失败只有裸串 "invalid_mcp"（无法细分，走通用兜底）。
      if (/command is required|unsupported mcp transport/.test(e.message)) return { title: '连接配置不完整，请检查启动命令或连接方式。' }
      if (/url is required/.test(e.message)) return { title: '请填写远程服务地址。' }
      if (/401|403|unauthor|forbidden/i.test(e.message)) {
        return { title: '该服务需要鉴权，请在请求头中提供有效的 API Key；交互式 OAuth 登录暂不支持。' }
      }
      return { title: CONNECTORS.errMcpConnect, detail: `${e.code}: ${e.message}` }
    }
```

- [ ] **步骤 6：运行确认通过**：`npx vitest run src/strings.connector.test.ts`，PASS；`npx tsc --noEmit` 无错（`SavedConnection` 暂未使用会因未导出报错吗——它已 `export`，不会）。

- [ ] **步骤 7：Commit**：`git add -A; git commit -m "feat(web): 增加 MCP 文案、invalid_mcp 人话错误与 SavedConnection 类型"`

---

## 任务 4：MCP 纯函数模块 `connectorForms/mcp.ts`

**文件：**
- 创建：`web/chat/src/pages/connectorForms/mcp.ts`、`mcp.test.ts`

- [ ] **步骤 1：编写失败测试** `mcp.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import type { ConnectorInfo, ToolInfo } from '../../api'
import {
  connectorToMcpForm, mcpConnectorIds, mcpSummary, validateMcp, type McpFormValues,
} from './mcp'

const stdio = (over: Partial<McpFormValues> = {}): McpFormValues => ({
  id: 'analytics', transport: 'stdio', command: 'npx', argsText: '-y srv', envText: 'A=1',
  url: '', headersText: '', ...over,
})
const http = (over: Partial<McpFormValues> = {}): McpFormValues => ({
  id: 'remote', transport: 'http', command: '', argsText: '', envText: '',
  url: 'https://mcp.example.com', headersText: 'Authorization=Bearer t', ...over,
})

describe('validateMcp', () => {
  it('accepts a stdio form and builds the mcp config', () => {
    const r = validateMcp(stdio())
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.id).toBe('analytics')
      expect(r.mcp).toEqual({ transport: 'stdio', command: 'npx', args: ['-y', 'srv'], env: { A: '1' } })
    }
  })
  it('omits empty env map', () => {
    const r = validateMcp(stdio({ envText: '  ' }))
    if (r.ok) expect(r.mcp.env).toBeUndefined()
  })
  it('requires id pattern', () => {
    const r = validateMcp(stdio({ id: 'Bad Id' }))
    if (!r.ok) expect(r.fieldErrors.id).toBeTruthy()
    else throw new Error('expected failure')
  })
  it('requires stdio command', () => {
    const r = validateMcp(stdio({ command: '  ' }))
    if (!r.ok) expect(r.fieldErrors.command).toBeTruthy()
    else throw new Error('expected failure')
  })
  it('flags bad env line on the env field', () => {
    const r = validateMcp(stdio({ envText: 'nope' }))
    if (!r.ok) expect(r.fieldErrors.env).toContain('nope')
    else throw new Error('expected failure')
  })
  it('accepts http form with headers', () => {
    const r = validateMcp(http())
    if (r.ok) expect(r.mcp).toEqual({ transport: 'http', url: 'https://mcp.example.com', headers: { Authorization: 'Bearer t' } } )
    else throw new Error('expected success')
  })
  it('requires http url and http(s) scheme', () => {
    const a = validateMcp(http({ url: '' }))
    const b = validateMcp(http({ url: 'ftp://x' }))
    if (!a.ok) expect(a.fieldErrors.url).toBeTruthy()
    if (!b.ok) expect(b.fieldErrors.url).toBeTruthy()
  })
  it('flags bad headers line', () => {
    const r = validateMcp(http({ headersText: 'noequals' }))
    if (!r.ok) expect(r.fieldErrors.headers).toContain('noequals')
    else throw new Error('expected failure')
  })
})

describe('mcpSummary', () => {
  it('summarizes stdio command and http url', () => {
    expect(mcpSummary({ transport: 'stdio', command: 'npx', args: ['x'] })).toBe('本地程序 · npx')
    expect(mcpSummary({ transport: 'http', url: 'https://h/mcp' })).toBe('远程服务 · https://h/mcp')
  })
  it('falls back gracefully', () => {
    expect(mcpSummary(undefined)).toBe('—')
  })
})

describe('connectorToMcpForm', () => {
  it('maps a connector into editable text fields', () => {
    const c: ConnectorInfo = {
      id: 'a', type: 'mcp',
      mcp: { transport: 'http', url: 'https://h', headers: { K: 'V' } },
      require_approval: ['t1'],
    }
    const f = connectorToMcpForm(c)
    expect(f).toMatchObject({ id: 'a', transport: 'http', url: 'https://h', headersText: 'K=V' })
  })
})

describe('mcpConnectorIds', () => {
  it('keeps mcp connector ids, deduped and ordered', () => {
    const tools = [
      { name: 'a', source: 'mcp', connector_id: 'c1' },
      { name: 'b', source: 'mcp', connector_id: 'c1' },
      { name: 'c', source: 'plugin', connector_id: 'c9' },
      { name: 'd', source: 'mcp', connector_id: 'c2' },
    ] as unknown as ToolInfo[]
    expect(mcpConnectorIds(tools)).toEqual(['c1', 'c2'])
  })
})
```

- [ ] **步骤 2：运行确认失败**：`npx vitest run src/pages/connectorForms/mcp.test.ts`，FAIL（模块不存在）。

- [ ] **步骤 3：实现 `mcp.ts`**：

```ts
import type { ConnectorInfo, MCPConfig, ToolInfo } from '../../api'
import { CONNECTORS } from '../../strings'
import { CONNECTOR_ID_RE } from './validate'
import { formatKeyValueMap, parseArgsText, parseKeyValueLines } from './lines'

export type McpTransport = 'stdio' | 'http'

export interface McpFormValues {
  id: string
  transport: McpTransport
  command: string
  argsText: string
  envText: string
  url: string
  headersText: string
}

export type McpFieldErrors = Partial<Record<'id' | 'command' | 'url' | 'env' | 'headers', string>>

const HTTP_URL_RE = /^https?:\/\/\S+$/i

export type McpValidationResult =
  | { ok: true; id: string; mcp: MCPConfig }
  | { ok: false; fieldErrors: McpFieldErrors }

export function validateMcp(v: McpFormValues): McpValidationResult {
  const fieldErrors: McpFieldErrors = {}
  const id = v.id.trim()
  if (!id) fieldErrors.id = CONNECTORS.errIdRequired
  else if (!CONNECTOR_ID_RE.test(id)) fieldErrors.id = CONNECTORS.errIdPattern

  if (v.transport === 'stdio') {
    const command = v.command.trim()
    if (!command) fieldErrors.command = CONNECTORS.errCommandRequired
    const envParsed = parseKeyValueLines(v.envText)
    if (!envParsed.ok) fieldErrors.env = CONNECTORS.errEnvLine(badLine(v.envText))
    if (Object.keys(fieldErrors).length > 0) return { ok: false, fieldErrors }
    return {
      ok: true, id,
      mcp: {
        transport: 'stdio', command,
        args: parseArgsText(v.argsText),
        env: Object.keys(envParsed.value).length > 0 ? envParsed.value : undefined,
      },
    }
  }

  const url = v.url.trim()
  if (!url) fieldErrors.url = CONNECTORS.errMcpUrlRequired
  else if (!HTTP_URL_RE.test(url)) fieldErrors.url = CONNECTORS.errMcpUrlHttp
  const headersParsed = parseKeyValueLines(v.headersText)
  if (!headersParsed.ok) fieldErrors.headers = CONNECTORS.errHeadersLine(badLine(v.headersText))
  if (Object.keys(fieldErrors).length > 0) return { ok: false, fieldErrors }
  return {
    ok: true, id,
    mcp: {
      transport: 'http', url,
      headers: Object.keys(headersParsed.value).length > 0 ? headersParsed.value : undefined,
    },
  }
}

/** 取首个非法 KEY=VALUE 行用于错误提示。 */
function badLine(text: string): string {
  for (const line of text.split('\n')) {
    const row = line.trim()
    if (row === '') continue
    const eq = row.indexOf('=')
    if (eq <= 0 || !row.slice(0, eq).trim()) return row
  }
  return ''
}

export function mcpSummary(mcp: MCPConfig | undefined): string {
  if (!mcp) return '—'
  if (mcp.transport === 'http') return CONNECTORS.mcpHttpSummary(mcp.url ?? '')
  return CONNECTORS.mcpStdioSummary(mcp.command ?? '')
}

export function connectorToMcpForm(c: ConnectorInfo): McpFormValues {
  const mcp = c.mcp
  return {
    id: c.id,
    transport: mcp?.transport === 'http' ? 'http' : 'stdio',
    command: mcp?.command ?? '',
    argsText: (mcp?.args ?? []).join('\n'),
    envText: formatKeyValueMap(mcp?.env),
    url: mcp?.url ?? '',
    headersText: formatKeyValueMap(mcp?.headers),
  }
}

export function mcpConnectorIds(tools: ToolInfo[]): string[] {
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const t of tools) {
    if (t.source !== 'mcp' || !t.connector_id || seen.has(t.connector_id)) continue
    seen.add(t.connector_id)
    ordered.push(t.connector_id)
  }
  return ordered
}
```

- [ ] **步骤 4：运行确认通过**：`npx vitest run src/pages/connectorForms/mcp.test.ts`，PASS；`npx tsc --noEmit` 无错。

- [ ] **步骤 5：Commit**：`git add -A; git commit -m "feat(web): 新增 MCP 表单校验/摘要/收集纯函数"`

---

## 任务 5：`ConnectorShell` 支持 mcp 类型（标题/空态/`summary` 行）

**文件：**
- 修改：`components/settings/ConnectorShell.tsx`、`ConnectorShell.test.tsx`

- [ ] **步骤 1：失败测试**（在 `ConnectorShell.test.tsx` 追加；渲染方式参考该文件现有用例，用 `MemoryRouter` 包裹）：

```tsx
it('renders mcp kind title and a transport summary row without a base url', async () => {
  await renderShell({
    kind: 'mcp',
    rows: [{ id: 'a1', summary: '本地程序 · npx', toolCount: 2, loginNames: [], approvalNames: ['t'] }],
  })
  expect(host.textContent).toContain('外部工具服务')
  expect(host.textContent).toContain('本地程序 · npx')
  expect(host.textContent).toContain('1 个需审批')
  expect(host.textContent).not.toContain('需本人登录')
})
it('empty mcp list shows the mcp empty state and add button', async () => {
  await renderShell({ kind: 'mcp', rows: [] })
  expect(host.textContent).toContain('还没有接入外部工具服务')
  expect([...host.querySelectorAll('button')].some((b) => b.textContent!.includes('接入外部工具'))).toBe(true)
})
```

> 若该测试文件尚无 `renderShell` 辅助，按现有 openapi/plugin 用例的渲染方式补一个最小 helper（props 给 `loading={false}`、空回调）。

- [ ] **步骤 2：运行确认失败**：`npx vitest run src/components/settings/ConnectorShell.test.tsx`，FAIL。

- [ ] **步骤 3：改 `ConnectorShell.tsx`**：
  - `ConnectorRowData` 增加可选字段 `summary?: string`。
  - 标题/描述/按钮/空态从二元 `isOpenapi` 改为按 kind 取值：

```tsx
  const labels = {
    openapi: { title: CONNECTORS.openapiTitle, desc: CONNECTORS.openapiDesc, add: CONNECTORS.addOpenapi, emptyTitle: CONNECTORS.openapiEmptyTitle, emptyDesc: CONNECTORS.openapiEmptyDesc },
    plugin:  { title: CONNECTORS.pluginTitle,  desc: CONNECTORS.pluginDesc,  add: CONNECTORS.addPlugin,  emptyTitle: CONNECTORS.pluginEmptyTitle,  emptyDesc: CONNECTORS.pluginEmptyDesc },
    mcp:     { title: CONNECTORS.mcpTitle,     desc: CONNECTORS.mcpDesc,     add: CONNECTORS.addMcp,     emptyTitle: CONNECTORS.mcpEmptyTitle,     emptyDesc: CONNECTORS.mcpEmptyDesc },
  }[kind]
```

  - 把现有 `title/description/addLabel/emptyTitle/emptyDesc` 改用 `labels.*`。
  - Card 的 `description` 改为 `{row.summary ?? row.baseUrl ?? '—'}`。
  - 其余（删除 ConfirmDialog、`permissionSummary`、工具链接、下拉）不变。

- [ ] **步骤 4：运行确认通过**：`npx vitest run src/components/settings/ConnectorShell.test.tsx`，PASS。

- [ ] **步骤 5：Commit**：`git add -A; git commit -m "feat(web): 连接器外壳支持 mcp 标题与传输摘要行"`

---

## 任务 6：通用化 `ConnectorEditorModal`（SavedConnection + MCP 第一步 + 第二步仅审批）

这是本批核心。第一步提交统一为判别联合 `SavedConnection`；页面缓存「连接级负载」供第二步整表回传；MCP 第二步只渲染审批列。openapi/plugin 的既有行为必须保持，已有测试随签名更新。

**文件：**
- 修改：`components/settings/ConnectorEditorModal.tsx`、`ConnectorEditorModal.test.tsx`
- 依赖任务 1–5 的 `Textarea`、`lines.ts`、`mcp.ts`、strings、types

- [ ] **步骤 1：先改/补失败测试**（在 `ConnectorEditorModal.test.tsx`）。把既有断言：
  - `onSavePermissions` 期望由 `('o1','https://x', ['login'], ['create_ticket'])` 改为 `('o1', ['login'], ['create_ticket'])`。
  - `onSavedInfo` 期望 `('p1')` 改为 `expect.objectContaining({ id: 'p1' })`。
  - 新增 MCP 用例：

```tsx
describe('ConnectorEditorModal mcp', () => {
  const mcpProps = (over: Partial<Props> = {}): Props => ({
    ...baseProps, kind: 'mcp', onSaveInfo: vi.fn(async () => [{ name: 'query' }]), ...over,
  })
  const mcpEditInitial = {
    id: 'a1', baseUrl: '',
    mcp: { transport: 'stdio' as const, command: 'npx', args: ['x'] },
    tools: [{ name: 'query' }, { name: 'write' }], loginNames: [], approvalNames: ['write'],
  }

  it('stdio: requires command and submits mcp config, then step2 shows approval only', async () => {
    const onSaveInfo = vi.fn(async () => [{ name: 'query' }])
    await render(mcpProps({ onSaveInfo }))
    // 默认 stdio：不填命令先保存
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'a1')
    await act(async () => { btn('保存连接').click(); await Promise.resolve() })
    expect(host.textContent).toContain('请填写启动命令')
    expect(onSaveInfo).not.toHaveBeenCalled()
    // 填命令
    const cmd = [...host.querySelectorAll('input')].find((i) => i.placeholder === 'npx')!
    await setValue(cmd, 'npx')
    await act(async () => { btn('保存连接').click(); await Promise.resolve() })
    await flush()
    expect(onSaveInfo).toHaveBeenCalledWith({ kind: 'mcp', id: 'a1', mcp: { transport: 'stdio', command: 'npx', args: [] } })
    expect(host.textContent).toContain('工具权限')
    // 第二步没有「需本人登录」复选框，只有审批
    expect(host.querySelector('input[data-flag="login"]')).toBeNull()
    expect(host.querySelector('input[data-tool="query"][data-flag="approval"]')).toBeTruthy()
  })

  it('http: requires url and submits url+headers', async () => {
    const onSaveInfo = vi.fn(async () => [{ name: 'q' }])
    await render(mcpProps({ onSaveInfo }))
    await act(async () => {
      const sel = host.querySelector('select')!
      const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value')!.set!
      setter.call(sel, 'http'); sel.dispatchEvent(new Event('change', { bubbles: true }))
      await Promise.resolve()
    })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'r1')
    const url = [...host.querySelectorAll('input')].find((i) => (i.placeholder ?? '').includes('mcp'))!
    await setValue(url, 'https://mcp.example.com')
    const headers = host.querySelector('textarea')!
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')!.set!
      setter.call(headers, 'Authorization=Bearer t'); headers.dispatchEvent(new Event('input', { bubbles: true })); await Promise.resolve()
    })
    await act(async () => { btn('保存连接').click(); await Promise.resolve() })
    await flush()
    expect(onSaveInfo).toHaveBeenCalledWith({
      kind: 'mcp', id: 'r1',
      mcp: { transport: 'http', url: 'https://mcp.example.com', headers: { Authorization: 'Bearer t' } },
    })
  })

  it('finish permissions calls onSavePermissions with approval list only (no login flag)', async () => {
    const onSavePermissions = vi.fn(async () => {})
    await render(mcpProps({ editing: true, initial: mcpEditInitial, onSavePermissions }))
    await act(async () => { btn('设置工具权限').click(); await Promise.resolve() })
    const writeBox = host.querySelector('input[data-tool="write"][data-flag="approval"]') as HTMLInputElement
    expect(writeBox.checked).toBe(true)
    await act(async () => { btn('完成').click(); await Promise.resolve() })
    await flush()
    expect(onSavePermissions).toHaveBeenCalledWith('a1', [], ['write'])
  })
})
```

- [ ] **步骤 2：运行确认失败**：`npx vitest run src/components/settings/ConnectorEditorModal.test.tsx`，MCP 用例 FAIL。

- [ ] **步骤 3：重写 `ConnectorEditorModal.tsx`**。要点（完整实现见下）：
  - props 的 `onSaveInfo: (conn: SavedConnection) => Promise<{name:string}[]>`；`onSavePermissions: (id, loginNames, approvalNames) => Promise<void>`（去掉 `baseUrl` 形参）；`onSavedInfo?: (conn: SavedConnection) => void`。
  - `ConnectorEditorInitial` 增加可选 `mcp?: MCPConfig`。
  - 新增 MCP 第一步状态：`transport/command/argsText/envText/url/headersText` 与字段错误；open 时用 `connectorToMcpForm(initial.mcp?)` 风格初始化（mcp 用 `initial.mcp`，编辑回显）。
  - 保存第一步按 kind 分支校验并构造 `SavedConnection`；成功后 `setSavedConn(conn)` 并 `onSavedInfo(conn)`。
  - 第二步 mcp 只渲染审批复选框；`finish` 对 mcp 传 `loginNames=[]`（页面也不会发 `require_login`）。
  - 「直接进入工具权限」（编辑态）可用：进入前若 `savedConn` 为空，用 `initial` 兜底构造连接负载并回调一次 `onSavedInfo`，确保页面拿到回传所需的连接级字段（mcp 用 `initial.mcp`，openapi/plugin 用 `baseUrl`）。

实现骨架（`ConnectorEditorModal.tsx`）：

```tsx
import { type ChangeEvent, useEffect, useRef, useState } from 'react'
import { Button, Field, Input, Modal, Select, Textarea } from '../ui'
import { CONNECTORS } from '../../strings'
import type { ImportFormat, MCPConfig } from '../../api'
import { validateConnection } from '../../pages/connectorForms/validate'
import {
  connectorToMcpForm, validateMcp, type McpFormValues, type McpFieldErrors,
} from '../../pages/connectorForms/mcp'
import {
  emptySelection, selectionFromLists, toNameLists, toggleTool,
} from '../../pages/connectorForms/permissions'
import type {
  ConnectorKind, FieldErrors, PermissionSelection, SavedConnection,
} from '../../pages/connectorForms/types'

export interface ConnectorEditorInitial {
  id: string
  baseUrl: string
  tools: { name: string }[]
  loginNames: string[]
  approvalNames: string[]
  mcp?: MCPConfig
}

const EMPTY_MCP_FORM: McpFormValues = {
  id: '', transport: 'stdio', command: '', argsText: '', envText: '', url: '', headersText: '',
}

export interface ConnectorEditorModalProps {
  kind: ConnectorKind
  open: boolean
  editing: boolean
  initial: ConnectorEditorInitial
  onClose: () => void
  formatError: (e: unknown) => string
  onSaveInfo: (conn: SavedConnection) => Promise<{ name: string }[]>
  onSavePermissions: (id: string, loginNames: string[], approvalNames: string[]) => Promise<void>
  onSavedInfo?: (conn: SavedConnection) => void
}
```

  state 与 open 初始化（在现有 id/baseUrl/spec 状态基础上新增）：

```tsx
  const [mcpForm, setMcpForm] = useState<McpFormValues>(EMPTY_MCP_FORM)
  const [mcpErrors, setMcpErrors] = useState<McpFieldErrors>({})
  const [savedConn, setSavedConn] = useState<SavedConnection | null>(null)
  const isMcp = kind === 'mcp'
  const isOpenapi = kind === 'openapi'
```

  open effect 内追加：

```tsx
    setMcpForm(init.mcp
      ? connectorToMcpForm({ id: init.id, type: 'mcp', mcp: init.mcp })
      : { ...EMPTY_MCP_FORM })
    setMcpErrors({})
    setSavedConn(null)
```

  `setMcp` 小工具：`const patchMcp = (p: Partial<McpFormValues>) => { setMcpForm((f) => ({ ...f, ...p })); setMcpErrors((e) => ({ ...e })) }`（具体清错在各 onChange 内按字段处理）。

- [ ] **步骤 4：第一步保存分支**（替换原 `saveInfo`）：

```tsx
  const saveInfo = async () => {
    let conn: SavedConnection | null = null
    if (isMcp) {
      // MCP 复用顶部共享「连接编号」输入（id state），传输字段在 mcpForm。
      const result = validateMcp({ ...mcpForm, id })
      if (!result.ok) {
        setFieldErrors((p) => ({ ...p, id: result.fieldErrors.id }))
        setMcpErrors({ command: result.fieldErrors.command, url: result.fieldErrors.url, env: result.fieldErrors.env, headers: result.fieldErrors.headers })
        return
      }
      setFieldErrors({})
      setMcpErrors({})
      conn = { kind: 'mcp', id: result.id, mcp: result.mcp }
    } else {
      const hasSpec = specContent != null || specUrl.trim() !== ''
      const result = validateConnection({ kind, id, baseUrl, hasSpec, editing })
      if (!result.ok) { setFieldErrors(result.fieldErrors); return }
      setFieldErrors({})
      if (kind === 'openapi') {
        conn = {
          kind: 'openapi', id: result.id, baseUrl: result.baseUrl, importFormat,
          spec: hasSpec
            ? { content: specContent ?? undefined, url: specContent == null ? specUrl.trim() : undefined }
            : undefined,
        }
      } else {
        conn = { kind: 'plugin', id: result.id, baseUrl: result.baseUrl }
      }
    }
    setFormError(null); setSaving(true)
    try {
      const discovered = await props.onSaveInfo(conn)
      const nextTools = discovered.length > 0 ? discovered : initial.tools
      const fallback = editing
        ? selectionFromLists(nextTools, isMcp ? [] : initial.loginNames, initial.approvalNames)
        : emptySelection(nextTools)
      setTools(nextTools)
      setSelection(reselect(nextTools, fallback))
      setSavedConn(conn)
      setStep(2)
      props.onSavedInfo?.(conn)
    } catch (e) {
      setFormError(props.formatError(e))
    } finally { setSaving(false) }
  }
```

- [ ] **步骤 5：第二步与「直接进入工具权限」**：

```tsx
  // 编辑态不重存第一步、直接去第二步：也要让页面拿到回传所需连接级字段。
  const connFromInitial = (): SavedConnection => {
    if (kind === 'mcp') return { kind: 'mcp', id: initial.id, mcp: initial.mcp ?? { transport: 'stdio' } }
    if (kind === 'openapi') return { kind: 'openapi', id: initial.id, baseUrl: initial.baseUrl, importFormat: 'auto' }
    return { kind: 'plugin', id: initial.id, baseUrl: initial.baseUrl }
  }

  const gotoPermissions = () => {
    const fallback = selectionFromLists(
      initial.tools, isMcp ? [] : initial.loginNames, initial.approvalNames,
    )
    setTools(initial.tools)
    setSelection(reselect(initial.tools, fallback))
    if (!savedConn) { const c = connFromInitial(); setSavedConn(c); props.onSavedInfo?.(c) }
    setStep(2)
  }

  const finish = async () => {
    setSaving(true)
    try {
      const { loginNames, approvalNames } = toNameLists(selection)
      // MCP 无 login 位：恒传空 login 名单（页面据此省略 require_login）。
      await props.onSavePermissions(id.trim(), isMcp ? [] : loginNames, approvalNames)
      props.onClose()
    } catch (e) {
      setFormError(props.formatError(e))
      setSaving(false)
    }
  }
```

  标题三元改三分支：

```tsx
  const title = editing
    ? { openapi: CONNECTORS.editOpenapi, plugin: CONNECTORS.editPlugin, mcp: CONNECTORS.editMcp }[kind]
    : { openapi: CONNECTORS.addOpenapi, plugin: CONNECTORS.addPlugin, mcp: CONNECTORS.addMcp }[kind]
```

- [ ] **步骤 6：第一步 JSX**。在现有共享「连接编号」`Field` 之后：
  - 对 openapi/plugin 保留现有 `fieldBaseUrl`（对 mcp 不渲染该 Field）；openapi 的 spec 文件/URL/格式块原样保留。
  - mcp 渲染（`{isMcp && (...)}`）：

```tsx
{isMcp && (
  <>
    <Field label={CONNECTORS.fieldTransport} required>
      <Select value={mcpForm.transport} disabled={saving}
        onChange={(e) => setMcpForm((f) => ({ ...f, transport: e.target.value === 'http' ? 'http' : 'stdio' }))}>
        <option value="stdio">{CONNECTORS.transportStdio}</option>
        <option value="http">{CONNECTORS.transportHttp}</option>
      </Select>
    </Field>
    {mcpForm.transport === 'stdio' ? (
      <>
        <Field label={CONNECTORS.fieldCommand} hint={CONNECTORS.fieldCommandHint} required error={mcpErrors.command}>
          <Input value={mcpForm.command} disabled={saving} placeholder="npx"
            onChange={(e) => { setMcpForm((f) => ({ ...f, command: e.target.value })); setMcpErrors((p) => ({ ...p, command: undefined })) }} />
        </Field>
        <Field label={CONNECTORS.fieldArgs} hint={CONNECTORS.fieldArgsHint}>
          <Textarea rows={3} value={mcpForm.argsText} disabled={saving} placeholder="@bytebase/dbhub"
            onChange={(e) => setMcpForm((f) => ({ ...f, argsText: e.target.value }))} />
        </Field>
        <Field label={CONNECTORS.fieldEnv} hint={CONNECTORS.fieldEnvHint} error={mcpErrors.env}>
          <Textarea rows={3} value={mcpForm.envText} disabled={saving} placeholder="DSN=${DSN}"
            onChange={(e) => { setMcpForm((f) => ({ ...f, envText: e.target.value })); setMcpErrors((p) => ({ ...p, env: undefined })) }} />
        </Field>
      </>
    ) : (
      <>
        <Field label={CONNECTORS.fieldUrl} required error={mcpErrors.url}>
          <Input value={mcpForm.url} disabled={saving} placeholder="https://mcp.example.com/mcp"
            onChange={(e) => { setMcpForm((f) => ({ ...f, url: e.target.value })); setMcpErrors((p) => ({ ...p, url: undefined })) }} />
        </Field>
        <Field label={CONNECTORS.fieldHeaders} hint={CONNECTORS.fieldHeadersHint} error={mcpErrors.headers}>
          <Textarea rows={3} value={mcpForm.headersText} disabled={saving} placeholder="Authorization=Bearer ${TOKEN}"
            onChange={(e) => { setMcpForm((f) => ({ ...f, headersText: e.target.value })); setMcpErrors((p) => ({ ...p, headers: undefined })) }} />
        </Field>
      </>
    )}
  </>
)}
```

  共享「服务地址」Field 用 `{!isMcp && (...)}` 包裹。`Field` 会把 `invalid` 注入唯一子元素；`Textarea`/`Input`/`Select` 均支持 `invalid` prop。

- [ ] **步骤 7：第二步 JSX 仅审批列**。把现有两个 `label.ui-checkbox-row` 改为：login 列仅 `{!isMcp && (...)}`；审批列始终渲染。即：

```tsx
{!isMcp && (
  <label className="ui-checkbox-row">
    <input type="checkbox" data-tool={t.name} data-flag="login"
      checked={selection[t.name]?.login ?? false} disabled={saving}
      onChange={() => setSelection((s) => toggleTool(s, t.name, 'login'))} />
    <span>{CONNECTORS.permLogin}</span>
  </label>
)}
<label className="ui-checkbox-row">
  <input type="checkbox" data-tool={t.name} data-flag="approval"
    checked={selection[t.name]?.approval ?? false} disabled={saving}
    onChange={() => setSelection((s) => toggleTool(s, t.name, 'approval'))} />
  <span>{CONNECTORS.permApproval}</span>
</label>
```

  第二步引导语对 mcp 使用不带「需本人登录」语义的文案：`{isMcp ? CONNECTORS.permsIntroMcp : CONNECTORS.permsIntro}`（strings 增 `permsIntroMcp: '勾选后，助手每次调用该工具前都会请你确认；不勾则直接执行。'`）。

- [ ] **步骤 8：更新 openapi/plugin 两页接线**（签名变化，必须同任务改完否则 tsc 失败）：
  - 两页各新增 `const [savedConn, setSavedConn] = useState<SavedConnection | null>(null)`（按自身类型）；`onSavedInfo={(c) => { setSavedConn(c); push(...); void load() }}`；打开编辑器时重置 `savedConn`（create 置 null；edit 用 `initial` 预构造对应负载并 setSavedConn，使「直接进入工具权限」也能回传）。
  - `onSaveInfo` 入参改为 `conn: SavedConnection`，按 `conn.kind` 构造 PUT（替换原 `input.baseUrl/spec`）。
  - `onSavePermissions(id, loginNames, approvalNames)` 用 `savedConn` 拼整表：openapi 发 `{type:'openapi', base_url: savedConn.baseUrl, require_login, require_approval}`；plugin 发 `{type:'http', base_url: savedConn.baseUrl, require_login, require_approval}`。
  - 插件页 `handleSaveInfo` 的参数类型由 `{id,baseUrl}` 改为 `SavedConnection`（仅处理 `conn.kind==='plugin'`）。

- [ ] **步骤 9：更新 openapi/plugin 页测试与编辑器测试到新签名**，运行：
  - `npx vitest run src/components/settings/ConnectorEditorModal.test.tsx src/pages/PluginSettings.test.tsx src/pages/OpenApiSettings.test.tsx`
  - 预期全绿（既有 openapi/plugin 行为不回归，MCP 新用例通过）。
  - `npx tsc --noEmit` 无错。

- [ ] **步骤 10：Commit**：`git add -A; git commit -m "feat(web): 连接器编辑器通用化并支持 MCP 两步接入（仅人工审批）"`

---

## 任务 7：重写 `McpSettings` 为薄页，删除 `connectorDelete`

**文件：**
- 重写：`pages/McpSettings.tsx`
- 重写：`pages/McpSettings.test.tsx`（替换旧 `McpSettings.test.ts`，旧文件删除）
- 删除：`pages/connectorDelete.ts`、`pages/connectorDelete.test.ts`

- [ ] **步骤 1：先写失败测试** `pages/McpSettings.test.tsx`（沿用 `PluginSettings.test.tsx` 的 jsdom 渲染/fetch mock 模式）：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { McpSettings } from './McpSettings'
import { mcpConnectorIds } from './connectorForms/mcp'
import type { ToolInfo } from '../api'

function json(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
let host: HTMLDivElement
let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); fetchMock = vi.fn(); vi.stubGlobal('fetch', fetchMock) })
afterEach(() => { host.remove(); vi.unstubAllGlobals() })
const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const btn = (t: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(t))!
const setValue = (el: Element, v: string) => act(async () => {
  const s = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  s.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})

const existing = {
  id: 'legacy', type: 'mcp',
  mcp: { transport: 'stdio', command: 'npx', args: ['srv'] },
  require_login: [], require_approval: ['write'], tools: [{ name: 'write' }],
}

describe('mcpConnectorIds', () => {
  it('filters/dedupes/orders', () => {
    const tools = [
      { name: 'a', source: 'mcp', connector_id: 'c1' },
      { name: 'b', source: 'mcp', connector_id: 'c1' },
      { name: 'c', source: 'http', connector_id: 'x' },
      { name: 'd', source: 'mcp', connector_id: 'c2' },
    ] as unknown as ToolInfo[]
    expect(mcpConnectorIds(tools)).toEqual(['c1', 'c2'])
  })
})

describe('McpSettings page', () => {
  it('lists an mcp connector with transport summary and approval count', async () => {
    fetchMock.mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u.endsWith('/v0/tools')) return json({ tools: [{ name: 'write', connector_id: 'legacy', source: 'mcp' }] })
      if (u.includes('/v0/connectors/legacy')) return json(existing)
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    expect(host.textContent).toContain('外部工具服务')
    expect(host.textContent).toContain('本地程序 · npx')
    expect(host.textContent).toContain('1 个需审批')
  })

  it('creates an mcp connector then saves approval in a second PUT that resends mcp and omits require_login', async () => {
    const created = { ...existing, id: 'a1', mcp: { transport: 'stdio', command: 'npx', args: [] }, require_approval: [], tools: [{ name: 'query' }] }
    let made = false
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (u.endsWith('/v0/tools')) return json({ tools: made ? [{ name: 'query', connector_id: 'a1', source: 'mcp' }] : [] })
      if (init?.method === 'PUT' && u.includes('/v0/connectors/a1')) { made = true; return json(created) }
      if (!init?.method && u.includes('/v0/connectors/a1')) return json(created)
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    await act(async () => { btn('接入外部工具').click(); await Promise.resolve() })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'a1')
    const cmd = [...host.querySelectorAll('input')].find((i) => i.placeholder === 'npx')!
    await setValue(cmd, 'npx')
    await act(async () => { btn('保存连接').click(); await Promise.resolve() }); await flush(4)
    expect(host.textContent).toContain('工具权限')
    await act(async () => {
      (host.querySelector('input[data-tool="query"][data-flag="approval"]') as HTMLInputElement).click()
      await Promise.resolve()
    })
    await act(async () => { btn('完成').click(); await Promise.resolve() }); await flush(4)
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(2)
    const first = JSON.parse((puts[0][1] as RequestInit).body as string)
    const second = JSON.parse((puts[1][1] as RequestInit).body as string)
    expect(first).toMatchObject({ type: 'mcp', mcp: { transport: 'stdio', command: 'npx' } })
    expect(first.require_login).toBeUndefined()
    expect(second.mcp).toEqual({ transport: 'stdio', command: 'npx', args: [] })
    expect(second.require_approval).toEqual(['query'])
    expect(second.require_login).toBeUndefined()
  })
})
```

- [ ] **步骤 2：运行确认失败**：`npx vitest run src/pages/McpSettings.test.tsx`，FAIL（页面仍为旧抽屉版，按钮文案不符）。

- [ ] **步骤 3：重写 `pages/McpSettings.tsx`**：

```tsx
import { useCallback, useEffect, useState } from 'react'
import {
  deleteConnector, getConnector, listTools, putConnector,
  type ConnectorInfo, type ToolInfo,
} from '../api'
import { ToastRegion, useToast } from '../components/ui'
import { ConnectorShell, type ConnectorRowData } from '../components/settings/ConnectorShell'
import { ConnectorEditorModal, type ConnectorEditorInitial } from '../components/settings/ConnectorEditorModal'
import { CONNECTORS, connectorErrorText } from '../strings'
import { mcpConnectorIds, mcpSummary } from './connectorForms/mcp'
import type { SavedConnection } from './connectorForms/types'

function toRow(info: ConnectorInfo, fallbackCount: number): ConnectorRowData {
  return {
    id: info.id,
    summary: mcpSummary(info.mcp),
    toolCount: info.tools?.length ?? fallbackCount,
    loginNames: [],
    approvalNames: info.require_approval ?? [],
  }
}

export function McpSettings() {
  const { toasts, push, dismiss } = useToast()
  const [rows, setRows] = useState<ConnectorRowData[]>([])
  const [connectors, setConnectors] = useState<ConnectorInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const emptyInitial: ConnectorEditorInitial = { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] }
  const [editor, setEditor] = useState<{ open: boolean; editing: boolean; initial: ConnectorEditorInitial }>({
    open: false, editing: false, initial: emptyInitial,
  })
  const [savedConn, setSavedConn] = useState<SavedConnection | null>(null)

  const load = useCallback(async () => {
    try {
      const tools = await listTools()
      const countById = new Map<string, number>()
      for (const t of tools) {
        if (t.source === 'mcp' && t.connector_id) countById.set(t.connector_id, (countById.get(t.connector_id) ?? 0) + 1)
      }
      const infos = await Promise.all(mcpConnectorIds(tools).map((id) => getConnector(id)))
      setConnectors(infos)
      setRows(infos.map((c) => toRow(c, countById.get(c.id) ?? 0)))
      setLoadError(null)
    } catch (e) {
      setLoadError(connectorErrorText(e).title)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => { void load() }, [load])

  const openCreate = () => { setSavedConn(null); setEditor({ open: true, editing: false, initial: emptyInitial }) }
  const openEdit = (id: string) => {
    const c = connectors.find((x) => x.id === id)
    if (!c) return
    const conn: SavedConnection = { kind: 'mcp', id: c.id, mcp: c.mcp ?? { transport: 'stdio' } }
    setSavedConn(conn)
    setEditor({
      open: true, editing: true,
      initial: {
        id: c.id, baseUrl: '',
        tools: (c.tools ?? []).map((t) => ({ name: t.name })),
        loginNames: [], approvalNames: c.require_approval ?? [], mcp: c.mcp,
      },
    })
  }

  const handleDelete = async (id: string) => {
    await deleteConnector(id)
    push({ tone: 'success', title: `${CONNECTORS.deleted} ${id}` })
    await load()
  }

  const handleSaveInfo = async (conn: SavedConnection) => {
    if (conn.kind !== 'mcp') return []
    const c = await putConnector(conn.id, { type: 'mcp', mcp: conn.mcp })
    setSavedConn(conn)
    return (c.tools ?? []).map((t) => ({ name: t.name }))
  }
  const handleSavePermissions = async (_id: string, _login: string[], approvalNames: string[]) => {
    if (!savedConn || savedConn.kind !== 'mcp') return
    await putConnector(savedConn.id, { type: 'mcp', mcp: savedConn.mcp, require_approval: approvalNames })
    push({ tone: 'success', title: `${CONNECTORS.saved} ${savedConn.id}` })
    await load()
  }

  return (
    <>
      <ConnectorShell kind="mcp" rows={rows} loading={loading} loadError={loadError}
        onCreate={openCreate} onEdit={openEdit} onDelete={handleDelete} />
      <ConnectorEditorModal kind="mcp" open={editor.open} editing={editor.editing} initial={editor.initial}
        onClose={() => setEditor((e) => ({ ...e, open: false }))}
        formatError={(e) => connectorErrorText(e).title}
        onSaveInfo={handleSaveInfo} onSavePermissions={handleSavePermissions}
        onSavedInfo={(c) => { setSavedConn(c); push({ tone: 'success', title: `${CONNECTORS.saved} ${c.id}` }); void load() }} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </>
  )
}
```

  注意：删除整个旧文件内容（含 re-export、抽屉、`validateMcpForm` 等）。`ToolInfo` 若未使用则不导入（上面仅在测试用，页面不需要，移除该导入）。

- [ ] **步骤 4：删除旧文件**：`git rm web/chat/src/pages/McpSettings.test.ts web/chat/src/pages/connectorDelete.ts web/chat/src/pages/connectorDelete.test.ts`。确认无残留引用：`rg "connectorDelete|from './McpSettings'" web/chat/src`（Inbox/Webhook/McpExport 已在任务 1 改引 lines.ts）。

- [ ] **步骤 5：运行**：`npx vitest run src/pages/McpSettings.test.tsx`；再 `npx tsc --noEmit`；预期绿/无错。

- [ ] **步骤 6：Commit**：`git add -A; git commit -m "feat(web): MCP 接入页迁入连接器外壳两步编辑器，移除旧抽屉与 window.confirm"`

---

## 任务 8：「对外提供能力」页人话化（不改 API）

把 `McpExportSettings.tsx` 的标题/文案集中到 strings，两处 `window.confirm` 换 `ConfirmDialog`，裸成功/错误文本换 `Toast` + 人话内联错误，一次性 Key 抽屉换 `Modal`，headers 录入用共享 `Textarea`。三段结构与所有 `/v0/settings/mcp-export*` 调用不变。纯函数 `mcpExportEndpointUrl / identityToForm / validateIdentityForm / isKeyActive` 保留（可留在页面文件）。

**文件：**
- 修改：`strings.ts`（新增 `MCP_EXPORTS` 常量）、`pages/McpExportSettings.tsx`、`pages/McpExportSettings.test.tsx`

- [ ] **步骤 1：strings.ts 新增常量**（放在 `CONNECTORS` 之后）：

```ts
// ---- 设置页：对外提供能力（MCP 导出） ----
export const MCP_EXPORTS = {
  title: '对外提供能力',
  intro:
    '把助手的能力以标准 MCP 服务对外开放，供 Cursor 等其他客户端调用。调用方凭下方创建的专用密钥访问。',
  introToolsLink: '具体哪些功能对外可用，在「助手功能」页按功能配置。',
  endpointTitle: '接入地址',
  endpointEnabled: '已启用',
  endpointDisabled: '已关闭（进程配置 mcp_export.enabled=false）',
  copyEndpoint: '复制地址',
  endpointCopied: '已复制接入地址',
  exampleTitle: '客户端配置示例',
  identityTitle: '调用方身份',
  identityIntro: '每把密钥必须绑定一个身份；调用时可附带统一的鉴权方式与请求头。',
  identityEmpty: '还没有调用方身份。',
  identityName: '名称',
  identityScheme: '鉴权方式（可选）',
  identityHeaders: '请求头（每行 KEY=VALUE）',
  createIdentity: '新建身份',
  keyTitle: '访问密钥',
  keyIntro: '密钥明文只在创建时显示一次，列表仅显示前缀；撤销后不可恢复。',
  keyEmpty: '还没有访问密钥。',
  keyName: '名称',
  keyBindIdentity: '绑定身份',
  keyNeedIdentityFirst: '请先创建身份',
  createKey: '新建密钥',
  revoke: '撤销',
  revoked: '已撤销',
  edit: '编辑',
  delete: '删除',
  save: '保存',
  cancel: '取消',
  tokenTitle: '密钥仅显示这一次',
  tokenBody: (name: string) => `请立即复制「${name}」的密钥并保存到安全位置，关闭后将无法再次查看。`,
  copyToken: '复制密钥',
  tokenCopied: '已复制密钥',
  tokenSaved: '我已保存',
  deleteIdentityTitle: '删除这个调用方身份？',
  deleteIdentityBody: (name: string) => `删除身份「${name}」后，其名下密钥也会一并删除。`,
  revokeKeyTitle: '撤销这把密钥？',
  revokeKeyBody: (name: string, prefix: string) => `撤销密钥「${name}」（${prefix}…）后不可恢复。`,
  errNameRequired: '请填写名称',
  errKeyNameRequired: '请填写密钥名称',
  errKeyIdentityRequired: '请选择绑定的身份',
  errBadHeaderLine: (row: string) => `请求头存在无法识别的行：${row}`,
  savedIdentity: '已保存调用方身份',
  deletedIdentity: (name: string) => `已删除身份 ${name}`,
  revokedKey: (name: string) => `已撤销密钥 ${name}`,
  copyFailed: '复制失败，请手动复制',
} as const
```

  `validateIdentityForm` 目前返回硬编码「名称不能为空」与 `无效键值行：…`；页面在本任务改为捕获失败后用 `MCP_EXPORTS.errNameRequired` / `errBadHeaderLine` 呈现（或在该纯函数内改用 strings；二选一，保持测试同步）。

- [ ] **步骤 2：补失败测试**（在 `McpExportSettings.test.tsx` 追加渲染交互用例；render/flush/fetch mock 仿 `PluginSettings.test.tsx`，组件需用 `MemoryRouter` 包裹因 intro 含 `Link`）：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { McpExportSettings } from './McpExportSettings'

function json(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' }, ...init })
}
let host: HTMLDivElement
let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); fetchMock = vi.fn(); vi.stubGlobal('fetch', fetchMock) })
afterEach(() => { host.remove(); vi.unstubAllGlobals() })
const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const btn = (t: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(t))!
const setValue = (el: Element, v: string) => act(async () => {
  const s = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  s.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})
async function renderExport() {
  await act(async () => { createRoot(host).render(<MemoryRouter><McpExportSettings /></MemoryRouter>); await Promise.resolve() })
  await flush()
}

const settings = { enabled: true, endpoint_path: '/v0/mcp/export' }
const identities = [{ id: 'ops', name: 'Ops', scheme: 'Bearer', headers: {} }]
const oneKey = [{ id: 'k1', name: 'cursor-dev', identity_id: 'ops', prefix: 'mcp_ab', revoked_at: null }]

it('asks via ConfirmDialog before revoking a key and toasts on success', async () => {
  fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
    const u = String(url)
    if (u.endsWith('/mcp-export')) return json(settings)
    if (u.includes('/identities')) return json(identities)
    if (u.includes('/keys/') && init?.method === 'DELETE') return json({ status: 'ok' })
    if (u.endsWith('/keys')) return json(oneKey)
    return json({})
  })
  await renderExport()
  await act(async () => { btn('撤销').click(); await Promise.resolve() })
  // 确认弹窗出现（复用 Modal）
  expect(host.textContent).toContain('撤销这把密钥？')
  await act(async () => {
    (host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await Promise.resolve()
  })
  await flush()
  const del = fetchMock.mock.calls.find(([u, i]) => String(u).includes('/keys/k1') && (i as RequestInit)?.method === 'DELETE')
  expect(del).toBeTruthy()
  expect(host.textContent).toContain('已撤销密钥 cursor-dev')
})

it('shows the one-time token in a Modal with a copy button', async () => {
  fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
    const u = String(url)
    if (u.endsWith('/mcp-export')) return json(settings)
    if (u.endsWith('/identities')) return json(identities)
    if (u.includes('/keys/') && init?.method === 'DELETE') return json({ status: 'ok' })
    if (u.endsWith('/keys')) {
      if (init?.method === 'POST') {
        return json({ id: 'k2', name: 'cursor-dev', identity_id: 'ops', token: 'SECRET-TOKEN', prefix: 'mcp_cd' })
      }
      return json(oneKey)
    }
    return json({})
  })
  await renderExport()
  // 「新建密钥」区名称输入：取最后一个文本输入框
  const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
  await setValue(inputs[inputs.length - 1], 'cursor-dev')
  await act(async () => { btn('新建密钥').click(); await Promise.resolve() })
  await flush()
  expect(host.textContent).toContain('密钥仅显示这一次')
  expect(host.textContent).toContain('SECRET-TOKEN')
  expect(btn('复制密钥')).toBeTruthy()
})
```

  备注：若页面把「撤销/删除」按钮在无数据时也渲染，选择器以 `data-testid="confirm-ok"` 为准（如上）；身份删除按钮不会匹配到该 testid。

- [ ] **步骤 3：改造 `McpExportSettings.tsx`**（按点替换，保持三段）：

  导入改为：

```tsx
import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Button, ConfirmDialog, Field, Input, Modal, PageHeader, Select, Textarea,
  ToastRegion, useToast,
} from '../components/ui'
import { parseKeyValueLines } from './connectorForms/lines'
import { MCP_EXPORTS } from '../strings'
// 其余 api 导入不变；移除 formatKeyValueMap 改由本地薄封装或继续从 lines 引入
import { formatKeyValueMap } from './connectorForms/lines'
```

  - 用 `const { toasts, push, dismiss } = useToast()` 替换 `status` 状态；所有成功处 `push({ tone:'success', title: ... })`；复制失败用 `push({ tone:'danger', title: MCP_EXPORTS.copyFailed })`。删除 `status` 相关 `<p>`。
  - 顶部用 `PageHeader`，description 传 ReactNode（其类型为 `ReactNode`，可内嵌链接）：

```tsx
<PageHeader
  title={MCP_EXPORTS.title}
  description={
    <>
      {MCP_EXPORTS.intro}{' '}
      <Link to="/settings/tools" className="settings-link">{MCP_EXPORTS.introToolsLink}</Link>
    </>
  }
/>
```

  - 两处 `window.confirm` 换受控 ConfirmDialog：新增

```tsx
const [confirm, setConfirm] = useState<
  | { kind: 'identity'; id: string; name: string }
  | { kind: 'key'; id: string; name: string; prefix: string }
  | null>(null)
const [busy, setBusy] = useState(false)
const [confirmError, setConfirmError] = useState<string | null>(null)
```

    列表按钮改为 `() => setConfirm({ kind:'key', id: key.id, name: key.name, prefix: key.prefix })`；确认弹窗：

```tsx
<ConfirmDialog
  open={confirm !== null}
  danger
  title={confirm?.kind === 'identity' ? MCP_EXPORTS.deleteIdentityTitle : MCP_EXPORTS.revokeKeyTitle}
  body={confirm ? (confirm.kind === 'identity'
    ? MCP_EXPORTS.deleteIdentityBody(confirm.name)
    : MCP_EXPORTS.revokeKeyBody(confirm.name, confirm.prefix)) : ''}
  confirmText={MCP_EXPORTS.delete}
  busy={busy}
  error={confirmError}
  onCancel={() => { if (!busy) { setConfirm(null); setConfirmError(null) } }}
  onConfirm={() => void runConfirm()}
/>
```

    `runConfirm` 按 `confirm.kind` 调 `deleteMCPExportIdentity` 或 `revokeMCPExportKey`，成功 toast + `setConfirm(null)` + `load()`，失败 `setConfirmError(String(...))`，finally `setBusy(false)`。

  - 一次性 Key 弹窗换 `Modal`：

```tsx
<Modal
  open={tokenModal !== null}
  title={MCP_EXPORTS.tokenTitle}
  onClose={() => setTokenModal(null)}
  footer={<><Button variant="secondary" onClick={() => void copyToken()}>{MCP_EXPORTS.copyToken}</Button>
    <Button variant="primary" onClick={() => setTokenModal(null)}>{MCP_EXPORTS.tokenSaved}</Button></>}
>
  {tokenModal && (
    <>
      <p className="settings-meta">{MCP_EXPORTS.tokenBody(tokenModal.name)}</p>
      <pre className="settings-muted">{tokenModal.token}</pre>
    </>
  )}
</Modal>
```

    `copyToken` 用 `navigator.clipboard.writeText(tokenModal.token)`，成功 toast `tokenCopied`，失败 toast `copyFailed`。
  - 表单中文标签/按钮改用 `MCP_EXPORTS.*`；`<textarea className="settings-textarea">` 两处 headers 输入换为 `<Field label=... hint=...><Textarea rows={3} .../></Field>`（编辑/新建表单都换）；普通文本输入用 `<Field><Input/></Field>`，身份/密钥下拉用 `<Field><Select>...</Select></Field>`。原生提交/按钮逐步替换为 `Button`，类型 `type="submit"`/`"button"` 正确。
  - 名称为空的前端拦截改用 `MCP_EXPORTS.errNameRequired / errKeyNameRequired / errKeyIdentityRequired`，通过局部 `formError` 状态内联显示（保留一个 `ui-inline-error`）。

- [ ] **步骤 4：运行**：`npx vitest run src/pages/McpExportSettings.test.tsx`，PASS；`npx tsc --noEmit` 无错；确认文件内不再出现 `window.confirm`、`settings-drawer`、裸 `MCP 导出` 标题。

- [ ] **步骤 5：Commit**：`git add -A; git commit -m "feat(web): 对外提供能力页人话化，确认弹窗/Toast/密钥弹窗原语化"`

---

## 任务 9：导航与首页展示改名（路由不变）

**文件：**
- 修改：`settingsNav.ts:74-87`、`settingsNav.test.ts`

- [ ] **步骤 1：失败测试**（在 `settingsNav.test.ts` 的 `uses the friendly names` 用例内追加）：

```ts
    expect(byTo['/settings/mcp']).toBe('外部工具服务')
    expect(byTo['/settings/mcp-export']).toBe('对外提供能力')
```

- [ ] **步骤 2：运行确认失败**：`npx vitest run src/settingsNav.test.ts`，FAIL。

- [ ] **步骤 3：改 `settingsNav.ts`**（仅 `label`/`desc`，`to`/`badge`/`group`/`operator`/`icon` 不变）：

```ts
  {
    to: '/settings/mcp', label: '外部工具服务', group: 'connect', icon: Boxes, operator: 'locked',
    badge: 'mcp',
    desc: '接入标准 MCP 工具服务（本地子进程或远程 HTTP），扩展助手可用能力',
  },
```

```ts
  {
    to: '/settings/mcp-export', label: '对外提供能力', group: 'connect', icon: Share2, operator: 'locked',
    badge: 'mcpExport',
    desc: '把助手的能力以标准 MCP 服务对外开放，供其他客户端调用',
  },
```

  首页卡片由 `SETTINGS_GROUPS + settingsNavItems` 自动渲染，无需改 `SettingsHome.tsx`；徽章文案「N 个出口」保持不变。

- [ ] **步骤 4：运行确认通过**：`npx vitest run src/settingsNav.test.ts src/pages/SettingsHome.test.tsx`，PASS。

- [ ] **步骤 5：Commit**：`git add -A; git commit -m "feat(web): 导航将 MCP/MCP 导出改名为外部工具服务/对外提供能力"`

---

## 任务 10：全量验证与重建嵌入产物

无 Go 逻辑改动，但仍跑后端测试兜底；前端全量测试 + 类型检查 + 构建，重建 `internal/ui/dist` 并提交。

**文件：** 产物 `internal/ui/dist/**`（由构建生成）

- [ ] **步骤 1：前端全量测试**：`cd web/chat; npm test`，预期全绿。重点确认连接器三页、编辑器、导出页、导航/徽章、Inbox/Webhook（lines 引用迁移）均通过。

- [ ] **步骤 2：类型检查与构建**：`npm run build`（= `tsc && vite build`），预期无 TS 错误并产出新的 `internal/ui/dist`（确认 vite 的构建输出目录配置确实指向 `internal/ui/dist`，与 P3-A/B 一致）。

- [ ] **步骤 3：后端兜底**：仓库根 `go test ./...`，预期全绿（本批不应改任何 `.go` 文件；若发现 Go 改动说明越界，回退）。

- [ ] **步骤 4：静态清扫检查**：
  - `rg "window.confirm" web/chat/src/pages/McpSettings.tsx web/chat/src/pages/McpExportSettings.tsx` 应无输出。
  - `rg "settings-mcp|settings-drawer" web/chat/src/pages/McpSettings.tsx web/chat/src/pages/McpExportSettings.tsx` 应无输出。
  - `rg "connectorDelete" web/chat/src` 应无输出（文件已删）。
  - `rg "from './McpSettings'|from \"./McpSettings\"" web/chat/src` 应无输出（三页已改引 `connectorForms/lines`）。
  - 确认 `.settings-drawer*` CSS 仍保留（Inbox/Tools 在用）。

- [ ] **步骤 5：提交产物**：`git add internal/ui/dist; git commit -m "build(web): rebuild embedded assets after P3-C MCP pages"`

- [ ] **步骤 6（人工，交回主会话）：浏览器/手机走查**：按既有 P3 走查清单，覆盖 stdio 与 HTTP 两种新建、编辑回显、第二步审批勾选/清空、删除确认失败可重试、导出页身份/密钥的新建/删除/撤销与一次性密钥弹窗、移动端长内容 Modal 内滚、暗色下 Textarea 与条件字段。

---

## 自检对照（规格覆盖）

- 规格 §4 命名/导航 → 任务 9；§5 列表外壳（summary/审批计数）→ 任务 5 + 7。
- 规格 §6 两步编辑器（stdio/http 字段、发现 30s busy、回传完整 mcp、仅审批列、空数组清空、编辑直达）→ 任务 4/6/7。
- 规格 §7 Textarea + lines 下沉 + 删 connectorDelete → 任务 1/2/7。
- 规格 §8 导出页三段人话化/ConfirmDialog/Toast/密钥 Modal → 任务 8。
- 规格 §9 invalid_mcp 人话与细分 → 任务 3。
- 规格 §10 测试 → 各任务 TDD + 任务 10 全量。
- 规格 §12 后端契约不改、§11 不做项（OAuth/capture/export_db_readonly/会话泄漏/Webhook-Inbox 人话化/结构化编辑器）→ 计划均无对应实现任务，符合预期。
- 类型一致性：`SavedConnection`、`ConnectorEditorInitial.mcp?`、`onSaveInfo(conn)`、`onSavePermissions(id,login,approval)`、`onSavedInfo(conn)`、`McpFormValues/McpFieldErrors`、`MCP_EXPORTS` 在定义任务（3/4/6/8）与使用任务间命名一致。
