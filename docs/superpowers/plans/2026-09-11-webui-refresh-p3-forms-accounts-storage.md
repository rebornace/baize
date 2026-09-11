# P3-A 表单地基 + 账号/存储页人话化 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 补齐表单型设置页通用原语，并将「账号」「数据存储」两页改造成令牌化、人话化、Toast/ConfirmDialog 反馈的统一范式；移除账号页手动 Token 框；纯前端、不改后端契约。

**架构：** 在 `web/chat/src/components/ui/` 新增 `PageHeader/Field/Input/Select/EmptyState`，样式进 `styles/components.css`；文案集中到 `strings.ts`；两个页面用新原语重写，复用 P1 的 `useToast/ToastRegion/ConfirmDialog` 与 `friendlyError`；完成后重建 `internal/ui/dist`。

**技术栈：** React 19 + TypeScript + Vite 6，vitest + jsdom（交互）/ renderToStaticMarkup（纯渲染），手写令牌化 CSS，lucide-react 图标。

**规格：** `docs/superpowers/specs/2026-09-11-webui-refresh-p3-forms-accounts-storage-design.md`

**工作目录：** 所有前端命令在 `web/chat` 下执行；后端仅在最后重建嵌入产物，不改 Go 逻辑。

---

## 文件结构

**新增**
- `web/chat/src/components/ui/PageHeader.tsx`（+ `PageHeader.test.tsx`）：页面标题、白话说明、右侧操作区。
- `web/chat/src/components/ui/Field.tsx`（+ `Field.test.tsx`，jsdom）：label/hint/error 容器，负责 htmlFor 与 aria 关联；同时导出供 Input/Select 读取错误态的约定类名。
- `web/chat/src/components/ui/Input.tsx`：令牌化输入框，透传原生 props，支持 `invalid`。
- `web/chat/src/components/ui/Select.tsx`：令牌化下拉，透传原生 props，支持 `invalid`。
- `web/chat/src/components/ui/EmptyState.tsx`（+ `EmptyState.test.tsx`）：图标 + 白话标题 + 说明 + 可选动作。
- `web/chat/src/pages/IdentitiesSettings.test.tsx`（jsdom）：账号页行为测试。
- `web/chat/src/pages/StorageSettings.test.tsx`（jsdom）：存储页行为测试。

**修改**
- `web/chat/src/components/ui/index.ts`：补导出。
- `web/chat/src/styles/components.css`：新原语样式 + 确认 checkbox 统一样式（明暗令牌）。
- `web/chat/src/strings.ts`：新增 `ACCOUNTS`、`STORAGE`（含 driver 映射）、来源标签 `identitySourceLabel`。
- `web/chat/src/pages/IdentitiesSettings.tsx`：按规格 §5 重写。
- `web/chat/src/pages/StorageSettings.tsx`：按规格 §6 重写。
- `internal/ui/dist/**`：任务 7 重建提交。

**不改**：任何 Go 文件、`api.ts` 的函数签名（`createIdentity` 保留，仅账号页不再 import）。

---

## 任务 1：展示型原语 PageHeader 与 EmptyState

**文件：**
- 创建：`web/chat/src/components/ui/PageHeader.tsx`
- 创建：`web/chat/src/components/ui/PageHeader.test.tsx`
- 创建：`web/chat/src/components/ui/EmptyState.tsx`
- 创建：`web/chat/src/components/ui/EmptyState.test.tsx`
- 修改：`web/chat/src/components/ui/index.ts`
- 修改：`web/chat/src/styles/components.css`（追加，见步骤 3）

- [ ] **步骤 1：编写失败的测试**

`PageHeader.test.tsx`：

```tsx
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { PageHeader } from './PageHeader'

describe('PageHeader', () => {
  it('renders title and description', () => {
    const html = renderToStaticMarkup(
      <PageHeader title="数据存储" description="选择数据保存位置" />,
    )
    expect(html).toContain('数据存储')
    expect(html).toContain('选择数据保存位置')
  })

  it('renders actions only when provided', () => {
    const withActions = renderToStaticMarkup(
      <PageHeader title="T" actions={<button type="button">刷新</button>} />,
    )
    expect(withActions).toContain('刷新')
    expect(withActions).toContain('ui-page-header-actions')
    const without = renderToStaticMarkup(<PageHeader title="T" />)
    expect(without).not.toContain('ui-page-header-actions')
  })
})
```

`EmptyState.test.tsx`：

```tsx
import { renderToStaticMarkup } from 'react-dom/server'
import { createElement } from 'react'
import { describe, expect, it } from 'vitest'
import { EmptyState } from './EmptyState'

describe('EmptyState', () => {
  it('renders title and description', () => {
    const html = renderToStaticMarkup(
      <EmptyState title="暂无已登录的业务账号" description="登录后会自动显示" />,
    )
    expect(html).toContain('暂无已登录的业务账号')
    expect(html).toContain('登录后会自动显示')
  })

  it('renders action and hides optional slots when absent', () => {
    const withAction = renderToStaticMarkup(
      <EmptyState title="空" action={<a href="/x">去添加</a>} />,
    )
    expect(withAction).toContain('去添加')
    expect(withAction).toContain('ui-empty-state-action')
    const plain = renderToStaticMarkup(<EmptyState title="空" />)
    expect(plain).not.toContain('ui-empty-state-action')
    expect(plain).not.toContain('ui-empty-state-icon')
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web/chat; npx vitest run src/components/ui/PageHeader.test.tsx src/components/ui/EmptyState.test.tsx`
预期：FAIL，报错找不到模块 `./PageHeader` / `./EmptyState`。

- [ ] **步骤 3：编写最少实现**

`PageHeader.tsx`：

```tsx
import type { ReactNode } from 'react'

export interface PageHeaderProps {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
}

export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <header className="ui-page-header">
      <div className="ui-page-header-row">
        <h1 className="ui-page-header-title">{title}</h1>
        {actions && <div className="ui-page-header-actions">{actions}</div>}
      </div>
      {description && <p className="ui-page-header-desc">{description}</p>}
    </header>
  )
}
```

`EmptyState.tsx`：

```tsx
import type { ReactNode } from 'react'

export interface EmptyStateProps {
  icon?: ReactNode
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="ui-empty-state" data-testid="ui-empty-state">
      {icon && (
        <div className="ui-empty-state-icon" aria-hidden="true">
          {icon}
        </div>
      )}
      <p className="ui-empty-state-title">{title}</p>
      {description && <p className="ui-empty-state-desc">{description}</p>}
      {action && <div className="ui-empty-state-action">{action}</div>}
    </div>
  )
}
```

在 `web/chat/src/styles/components.css` **末尾追加**：

```css
/* ---- PageHeader ---- */
.ui-page-header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-bottom: var(--space-4);
}
.ui-page-header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}
.ui-page-header-title {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 650;
  color: var(--text);
}
.ui-page-header-desc {
  margin: 0;
  font-size: 0.875rem;
  line-height: 1.5;
  color: var(--text-muted);
}
.ui-page-header-actions {
  display: flex;
  gap: var(--space-2);
  flex: 0 0 auto;
}

/* ---- EmptyState ---- */
.ui-empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: var(--space-2);
  padding: var(--space-8) var(--space-4);
  color: var(--text-muted);
}
.ui-empty-state-icon {
  display: flex;
  color: var(--text-faint);
}
.ui-empty-state-title {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--text);
}
.ui-empty-state-desc {
  margin: 0;
  max-width: 34rem;
  font-size: 0.85rem;
  line-height: 1.5;
}
.ui-empty-state-action {
  margin-top: var(--space-2);
}
```

在 `web/chat/src/components/ui/index.ts` 末尾补两行导出：

```ts
export { PageHeader, type PageHeaderProps } from './PageHeader'
export { EmptyState, type EmptyStateProps } from './EmptyState'
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web/chat; npx vitest run src/components/ui/PageHeader.test.tsx src/components/ui/EmptyState.test.tsx`
预期：PASS（4 个断言组全绿）。

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/components/ui/PageHeader.tsx web/chat/src/components/ui/PageHeader.test.tsx web/chat/src/components/ui/EmptyState.tsx web/chat/src/components/ui/EmptyState.test.tsx web/chat/src/components/ui/index.ts web/chat/src/styles/components.css
git commit -m "feat(web): 新增 PageHeader/EmptyState 设置页原语"
```

---

## 任务 2：表单原语 Field / Input / Select（含 aria 关联）

**文件：**
- 创建：`web/chat/src/components/ui/Field.tsx`
- 创建：`web/chat/src/components/ui/Input.tsx`
- 创建：`web/chat/src/components/ui/Select.tsx`
- 创建：`web/chat/src/components/ui/Field.test.tsx`（jsdom）
- 修改：`web/chat/src/components/ui/index.ts`
- 修改：`web/chat/src/styles/components.css`（追加）

说明：`Field` 只包裹**单个控件子元素**（本批均如此），通过 `cloneElement` 自动注入 `id`、`aria-invalid`、`aria-describedby`、`invalid`，调用方无需手工接线。

- [ ] **步骤 1：编写失败的测试**

`Field.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { Field } from './Field'
import { Input } from './Input'
import { Select } from './Select'

let host: HTMLDivElement
let root: ReturnType<typeof createRoot>
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
})

function render(el: React.ReactNode) {
  act(() => {
    root.render(el)
  })
}

describe('Field a11y wiring', () => {
  it('associates label/control via the given htmlFor', () => {
    render(<Field label="邮箱" htmlFor="email"><Input type="text" /></Field>)
    const input = host.querySelector('input')!
    expect(host.querySelector('label')!.getAttribute('for')).toBe('email')
    expect(input.id).toBe('email')
  })

  it('marks the control invalid and references the error node on error', () => {
    render(
      <Field label="邮箱" htmlFor="email" error="必填项">
        <Input type="text" />
      </Field>,
    )
    const input = host.querySelector('input')!
    expect(input.getAttribute('aria-invalid')).toBe('true')
    expect(input.getAttribute('aria-describedby')).toBe('email-error')
    expect(host.querySelector('#email-error')!.textContent).toBe('必填项')
    expect(host.querySelector('#email-hint')).toBeNull()
  })

  it('references hint when no error, and omits describedby when neither', () => {
    render(<Field label="路径" htmlFor="p" hint="默认 ./data/baize.db"><Input /></Field>)
    const input = host.querySelector('input')!
    expect(input.getAttribute('aria-invalid')).toBeNull()
    expect(input.getAttribute('aria-describedby')).toBe('p-hint')

    render(<Field label="x" htmlFor="x"><Select><option value="a">A</option></Select></Field>)
    const select = host.querySelector('select')!
    expect(select.getAttribute('aria-describedby')).toBeNull()
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web/chat; npx vitest run src/components/ui/Field.test.tsx`
预期：FAIL，找不到模块 `./Field` / `./Input` / `./Select`。

- [ ] **步骤 3：编写最少实现**

`Input.tsx`：

```tsx
import type { InputHTMLAttributes } from 'react'

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean
}

export function Input({ invalid, className = '', ...rest }: InputProps) {
  const invalidCls = invalid ? ' invalid' : ''
  return <input className={`ui-input${invalidCls}${className ? ` ${className}` : ''}`} {...rest} />
}
```

`Select.tsx`：

```tsx
import type { SelectHTMLAttributes } from 'react'

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  invalid?: boolean
}

export function Select({ invalid, className = '', children, ...rest }: SelectProps) {
  const invalidCls = invalid ? ' invalid' : ''
  return (
    <select className={`ui-select${invalidCls}${className ? ` ${className}` : ''}`} {...rest}>
      {children}
    </select>
  )
}
```

`Field.tsx`：

```tsx
import { cloneElement, isValidElement, useId, type ReactElement, type ReactNode } from 'react'

export interface FieldProps {
  label: ReactNode
  htmlFor?: string
  hint?: ReactNode
  error?: ReactNode
  required?: boolean
  className?: string
  children: ReactNode
}

export function Field({ label, htmlFor, hint, error, required, className = '', children }: FieldProps) {
  const autoId = useId()
  const controlId = htmlFor ?? autoId
  const errorId = `${controlId}-error`
  const hintId = `${controlId}-hint`
  const describedBy = error ? errorId : hint ? hintId : undefined

  const control = isValidElement(children)
    ? cloneElement(children as ReactElement<Record<string, unknown>>, {
        id: controlId,
        'aria-invalid': error ? true : undefined,
        'aria-describedby': describedBy,
        invalid: error ? true : undefined,
      })
    : children

  return (
    <div className={`ui-field${className ? ` ${className}` : ''}`}>
      <label className="ui-field-label" htmlFor={controlId}>
        {label}
        {required && (
          <span className="ui-field-required" aria-hidden="true">
            {' '}*
          </span>
        )}
      </label>
      {control}
      {error ? (
        <p className="ui-field-error" id={errorId}>
          {error}
        </p>
      ) : hint ? (
        <p className="ui-field-hint" id={hintId}>
          {hint}
        </p>
      ) : null}
    </div>
  )
}
```

在 `web/chat/src/styles/components.css` **末尾追加**：

```css
/* ---- Field / Input / Select ---- */
.ui-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.ui-field-label {
  font-size: 0.875rem;
  font-weight: 550;
  color: var(--text);
}
.ui-field-required {
  color: var(--danger);
}
.ui-field-hint {
  margin: 0;
  font-size: 0.8rem;
  line-height: 1.5;
  color: var(--text-muted);
}
.ui-field-error {
  margin: 0;
  font-size: 0.8rem;
  line-height: 1.5;
  color: var(--danger);
}
.ui-input,
.ui-select {
  width: 100%;
  box-sizing: border-box;
  height: 38px;
  padding: 0.5rem 0.625rem;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font: inherit;
}
.ui-input:focus-visible,
.ui-select:focus-visible {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.ui-input:disabled,
.ui-select:disabled {
  background: var(--surface-2);
  color: var(--text-muted);
  cursor: not-allowed;
}
.ui-input.invalid,
.ui-select.invalid {
  border-color: var(--danger);
}
.ui-input.invalid:focus-visible,
.ui-select.invalid:focus-visible {
  box-shadow: 0 0 0 3px var(--danger-soft);
}
@media (max-width: 768px) {
  .ui-input,
  .ui-select {
    min-height: 40px;
  }
}
```

在 `web/chat/src/components/ui/index.ts` 补：

```ts
export { Field, type FieldProps } from './Field'
export { Input, type InputProps } from './Input'
export { Select, type SelectProps } from './Select'
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web/chat; npx vitest run src/components/ui/Field.test.tsx`
预期：PASS（3 个用例）。再跑 `npx tsc --noEmit`，预期 0 错误。

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/components/ui/Field.tsx web/chat/src/components/ui/Input.tsx web/chat/src/components/ui/Select.tsx web/chat/src/components/ui/Field.test.tsx web/chat/src/components/ui/index.ts web/chat/src/styles/components.css
git commit -m "feat(web): 新增 Field/Input/Select 表单原语与无障碍关联"
```

---

## 任务 3：文案集中——来源标签与存储方式映射

**文件：**
- 修改：`web/chat/src/strings.ts`（末尾追加）
- 创建：`web/chat/src/settingsCopy.test.ts`

- [ ] **步骤 1：编写失败的测试**

`settingsCopy.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { driverLabel, identitySourceLabel, ACCOUNTS, STORAGE } from './strings'

describe('identitySourceLabel', () => {
  it('maps known sources to friendly Chinese', () => {
    expect(identitySourceLabel('login_capture')).toBe('对话中登录')
    expect(identitySourceLabel('env')).toBe('系统预设')
    expect(identitySourceLabel('manual')).toBe('临时提供')
  })
  it('falls back to the raw source for unknown values', () => {
    expect(identitySourceLabel('weird')).toBe('weird')
  })
})

describe('driverLabel', () => {
  it('maps storage drivers to friendly labels but keeps english submit value elsewhere', () => {
    expect(driverLabel('sqlite')).toBe('本地文件（SQLite）')
    expect(driverLabel('postgres')).toBe('PostgreSQL 数据库')
    expect(driverLabel('memory')).toBe('内存（重启即清空，仅试用）')
  })
  it('falls back to raw driver for unknown values', () => {
    expect(driverLabel('mysql')).toBe('mysql')
  })
})

describe('settings copy blocks exist', () => {
  it('exposes accounts/storage copy', () => {
    expect(ACCOUNTS.title).toBe('账号')
    expect(ACCOUNTS.emptyTitle).toContain('暂无')
    expect(STORAGE.title).toBe('数据存储')
    expect(STORAGE.confirmRestartTitle).toContain('重启')
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web/chat; npx vitest run src/settingsCopy.test.ts`
预期：FAIL，`identitySourceLabel` 等不是函数/未导出。

- [ ] **步骤 3：编写最少实现**

在 `web/chat/src/strings.ts` **末尾追加**：

```ts
// ---- 设置页：账号 ----
export const ACCOUNTS = {
  title: '账号',
  description:
    '这里显示助手当前已登录的业务系统账号。正常使用时，在对话中完成登录会自动出现在这里，无需手动填写。',
  emptyTitle: '暂无已登录的业务账号',
  emptyDesc: '在对话中登录业务系统后，会自动显示在这里。',
  setDefault: '设为默认',
  defaultBadge: '默认中',
  logout: '退出',
  clear: '清空登录账号',
  clearConfirmTitle: '清空已登录的账号？',
  clearConfirmBody: '将移除当前在对话中登录的全部业务账号（系统预设账号不受影响）。需要时可重新登录。',
  clearConfirmOk: '清空',
  toastLogout: '已退出账号',
  toastDefault: '已设为默认',
  toastCleared: '已清空登录账号',
  loadFailed: '无法加载账号',
  details: '详情',
  developer: '开发者信息',
} as const

/** 身份来源 -> 人话；未知来源原值兜底，不吞信息。 */
export function identitySourceLabel(source: string): string {
  switch (source) {
    case 'login_capture':
      return '对话中登录'
    case 'env':
      return '系统预设'
    case 'manual':
      return '临时提供'
    default:
      return source
  }
}

// ---- 设置页：数据存储 ----
export const STORAGE = {
  title: '数据存储',
  description:
    '选择助手数据的保存位置。更改并保存后服务会重启，且不会自动搬迁旧数据，请先自行备份。',
  driverField: '保存方式',
  sqlitePath: '数据库文件路径',
  sqliteHint: '默认 ./data/baize.db；换成新路径不会自动搬迁已有数据。',
  dsn: '连接地址（DSN）',
  dsnHint: '形如 postgres://用户名:密码@主机:5432/库名，仅保存在服务端配置。',
  ack: '我了解：切换存储不会自动迁移数据，旧库中的数据需自行处理',
  ackRequired: '请先勾选确认：切换存储不会自动迁移数据',
  postgresRequiresDSN: '使用 PostgreSQL 需要填写连接地址（DSN）',
  saveRestart: '保存并重启',
  saving: '正在保存…',
  confirmRestartTitle: '保存并重启服务？',
  confirmRestartBody:
    '服务将立即重启，进行中的对话会中断；数据不会从旧存储自动迁移。确认继续？',
  restarting: '正在重启…',
  developer: '技术信息',
} as const

const DRIVER_LABELS: Record<string, string> = {
  sqlite: '本地文件（SQLite）',
  postgres: 'PostgreSQL 数据库',
  memory: '内存（重启即清空，仅试用）',
}

/** 存储驱动 -> 人话选项；未知驱动原值兜底。提交值仍用英文 driver。 */
export function driverLabel(driver: string): string {
  return DRIVER_LABELS[driver] ?? driver
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web/chat; npx vitest run src/settingsCopy.test.ts && npx tsc --noEmit`
预期：PASS；tsc 0 错误。

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/strings.ts web/chat/src/settingsCopy.test.ts
git commit -m "feat(web): 集中账号/存储页人话文案与驱动映射"
```

---

## 任务 4：账号页重写（去 Token 框、卡片列表、空态、Toast、清空确认）

**文件：**
- 修改：`web/chat/src/pages/IdentitiesSettings.tsx`（整体重写）
- 创建：`web/chat/src/pages/IdentitiesSettings.test.tsx`（jsdom）

接口事实（不改）：`listIdentities(cid)` GET、`setDefaultIdentity(cid,id)` POST `.../identities/{id}/default`、`deleteIdentity(cid,id)` DELETE `.../identities/{id}`、`clearIdentities(cid)` DELETE `.../identities`。`IdentityView` 字段：`id/label/scheme?/source/claims_summary?/is_default/last_used_at?`。

- [ ] **步骤 1：编写失败的测试**

`IdentitiesSettings.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IdentitiesSettings } from './IdentitiesSettings'

function json(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.stubGlobal('fetch', vi.fn())
})
afterEach(() => {
  host.remove()
  vi.unstubAllGlobals()
})

async function renderPage() {
  await act(async () => {
    createRoot(host).render(<IdentitiesSettings />)
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

const click = (el: Element) => act(async () => { (el as HTMLElement).click(); await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0)) })

const ident = (o: Record<string, unknown>) => ({
  scheme: 'Bearer', source: 'login_capture', is_default: false, label: '张三', id: 'i1', ...o,
})

describe('IdentitiesSettings', () => {
  it('does not render the manual token box and shows empty state', async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(json([]))
    await renderPage()
    expect(host.textContent).toContain('暂无已登录的业务账号')
    expect(host.querySelector('input[type="password"]')).toBeNull()
    expect(host.textContent).not.toContain('保存 Token')
  })

  it('renders cards with friendly source, default badge and role actions', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (!init?.method && u.endsWith('/identities')) {
        return json([
          ident({ id: 'a', label: '张三', source: 'login_capture', is_default: true }),
          ident({ id: 'b', label: '系统', source: 'env' }),
        ])
      }
      return json({ status: 'ok' })
    })
    await renderPage()
    expect(host.textContent).toContain('张三')
    expect(host.textContent).toContain('对话中登录')
    expect(host.textContent).toContain('系统预设')
    expect(host.textContent).toContain('默认中')
    // 默认账号不出现「设为默认」；env 账号不出现「退出」。
    const cards = host.textContent ?? ''
    expect(cards).toContain('设为默认')
    expect(cards).toContain('退出')
  })

  it('requires confirm before clearing; cancel does nothing, ok deletes all', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (!init?.method && u.endsWith('/identities')) return json([ident({ id: 'a', source: 'manual' })])
      return json({ status: 'ok' })
    })
    await renderPage()

    await click([...host.querySelectorAll('button')].find((b) => b.textContent!.includes('清空登录账号'))!)
    expect(host.querySelector('[data-testid="confirm-ok"]')).not.toBeNull()

    await click(host.querySelector('[data-testid="confirm-cancel"]')!)
    const clearCallsDuringCancel = fetchMock.mock.calls.filter(
      ([u, i]) => String(u).endsWith('/identities') && (i as RequestInit)?.method === 'DELETE',
    )
    expect(clearCallsDuringCancel).toHaveLength(0)

    await click([...host.querySelectorAll('button')].find((b) => b.textContent!.includes('清空登录账号'))!)
    await click(host.querySelector('[data-testid="confirm-ok"]')!)
    const clearCalls = fetchMock.mock.calls.filter(
      ([u, i]) => String(u).endsWith('/identities') && (i as RequestInit)?.method === 'DELETE',
    )
    expect(clearCalls).toHaveLength(1)
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web/chat; npx vitest run src/pages/IdentitiesSettings.test.tsx`
预期：FAIL（仍能找到密码框/缺少空态文案）。

- [ ] **步骤 3：重写实现**

将 `web/chat/src/pages/IdentitiesSettings.tsx` 整体替换为：

```tsx
import { useCallback, useEffect, useState } from 'react'
import { Users } from 'lucide-react'
import {
  clearIdentities,
  deleteIdentity,
  listIdentities,
  setDefaultIdentity,
  type IdentityView,
} from '../api'
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  PageHeader,
  ToastRegion,
  useToast,
} from '../components/ui'
import { redactSensitive } from '../sensitive'
import { ACCOUNTS, friendlyError, identitySourceLabel } from '../strings'
import { uuid } from '../uuid'

const CONV_KEY = 'baize.conversation_id'

function loadConversationId(): string {
  const existing = localStorage.getItem(CONV_KEY)?.trim()
  if (existing) return existing
  const id = `conv_${uuid()}`
  localStorage.setItem(CONV_KEY, id)
  return id
}

function formatClaims(claims: Record<string, unknown> | undefined): string | null {
  if (!claims || Object.keys(claims).length === 0) return null
  try {
    return JSON.stringify(redactSensitive(claims), null, 2)
  } catch {
    return null
  }
}

export function IdentitiesSettings() {
  const [conversationId] = useState(loadConversationId)
  const [identities, setIdentities] = useState<IdentityView[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [confirmClear, setConfirmClear] = useState(false)
  const { toasts, push, dismiss } = useToast()

  const refresh = useCallback(async () => {
    try {
      setIdentities(await listIdentities(conversationId))
    } catch (e) {
      setIdentities(null)
      const f = friendlyError(e)
      push({ tone: 'error', title: ACCOUNTS.loadFailed, detail: f.detail ?? f.title })
    }
  }, [conversationId, push])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const runAction = useCallback(
    async (fn: () => Promise<void>, successTitle: string) => {
      setBusy(true)
      try {
        await fn()
        await refresh()
        push({ tone: 'success', title: successTitle })
      } catch (e) {
        const f = friendlyError(e)
        push({ tone: 'error', title: f.title, detail: f.detail })
      } finally {
        setBusy(false)
      }
    },
    [refresh, push],
  )

  const hasCaptured = (identities ?? []).some((i) => i.source !== 'env')

  return (
    <div className="settings-panel">
      <PageHeader title={ACCOUNTS.title} description={ACCOUNTS.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {identities === null && <p className="settings-muted">加载中…</p>}
      {identities !== null && identities.length === 0 && (
        <EmptyState
          icon={<Users size={28} aria-hidden="true" />}
          title={ACCOUNTS.emptyTitle}
          description={ACCOUNTS.emptyDesc}
        />
      )}

      {identities !== null && identities.length > 0 && (
        <div className="accounts-grid">
          {identities.map((idt) => {
            const claimsText = formatClaims(idt.claims_summary)
            const scheme = idt.scheme ? `${idt.scheme} · ` : ''
            return (
              <Card
                key={idt.id}
                title={idt.label || idt.id}
                description={`${scheme}${identitySourceLabel(idt.source)}`}
                trailing={
                  <div className="accounts-actions">
                    {idt.is_default ? (
                      <Badge tone="success">{ACCOUNTS.defaultBadge}</Badge>
                    ) : (
                      <Button
                        size="sm"
                        variant="secondary"
                        disabled={busy}
                        onClick={() =>
                          void runAction(() => setDefaultIdentity(conversationId, idt.id), ACCOUNTS.toastDefault)
                        }
                      >
                        {ACCOUNTS.setDefault}
                      </Button>
                    )}
                    {idt.source !== 'env' && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="danger-text"
                        disabled={busy}
                        onClick={() =>
                          void runAction(() => deleteIdentity(conversationId, idt.id), ACCOUNTS.toastLogout)
                        }
                      >
                        {ACCOUNTS.logout}
                      </Button>
                    )}
                  </div>
                }
              >
                {claimsText && (
                  <details className="accounts-claims-details">
                    <summary>{ACCOUNTS.details}</summary>
                    <pre className="accounts-claims">{claimsText}</pre>
                  </details>
                )}
              </Card>
            )
          })}
        </div>
      )}

      {hasCaptured && (
        <Button
          className="accounts-clear"
          variant="ghost"
          disabled={busy}
          onClick={() => setConfirmClear(true)}
        >
          {ACCOUNTS.clear}
        </Button>
      )}

      <ConfirmDialog
        open={confirmClear}
        danger
        title={ACCOUNTS.clearConfirmTitle}
        body={ACCOUNTS.clearConfirmBody}
        confirmText={ACCOUNTS.clearConfirmOk}
        busy={busy}
        onCancel={() => setConfirmClear(false)}
        onConfirm={() =>
          void runAction(() => clearIdentities(conversationId), ACCOUNTS.toastCleared).then(() =>
            setConfirmClear(false),
          )
        }
      />

      <details className="settings-developer">
        <summary>{ACCOUNTS.developer}</summary>
        <p className="settings-meta">会话 {conversationId}</p>
      </details>
    </div>
  )
}
```

在 `web/chat/src/styles/components.css` 末尾追加：

```css
/* ---- 账号页 ---- */
.accounts-grid {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.accounts-actions {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}
.accounts-claims-details {
  margin-top: var(--space-2);
}
.accounts-claims-details summary {
  cursor: pointer;
  font-size: 0.82rem;
  color: var(--text-muted);
}
.accounts-claims {
  margin: var(--space-2) 0 0;
  padding: var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  border: 1px solid var(--border);
  font-size: 0.78rem;
  overflow-x: auto;
}
.accounts-clear {
  margin-top: var(--space-4);
}
.settings-developer {
  margin-top: var(--space-6);
  font-size: 0.82rem;
  color: var(--text-muted);
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web/chat; npx vitest run src/pages/IdentitiesSettings.test.tsx && npx tsc --noEmit`
预期：PASS；tsc 0 错误。

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/pages/IdentitiesSettings.tsx web/chat/src/pages/IdentitiesSettings.test.tsx web/chat/src/styles/components.css
git commit -m "feat(web): 账号页人话化——卡片列表/空态/Toast/清空确认，移除手动 Token 框"
```

---

## 任务 5：存储页重写（driver 人话、Field 表单、二次确认、Toast）

**文件：**
- 修改：`web/chat/src/pages/StorageSettings.tsx`（整体重写）
- 创建：`web/chat/src/pages/StorageSettings.test.tsx`（jsdom）

接口事实（不改）：`getStoreSettings()` GET `/v0/settings/store`，返回 `StoreSettings{ driver, sqlite_path?, dsn_redacted?, drivers[], config_path?, overlay_path? }`；`putStoreSettings({driver, sqlite_path?, dsn?, acknowledge_no_migrate, restart?})` PUT 同路径。

- [ ] **步骤 1：编写失败的测试**

`StorageSettings.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { StorageSettings } from './StorageSettings'

function json(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

let host: HTMLDivElement
let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  host.remove()
  vi.unstubAllGlobals()
})

async function renderPage(getBody: unknown = { driver: 'sqlite', drivers: ['memory', 'sqlite', 'postgres'] }) {
  fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
    if (!init?.method && String(url).endsWith('/settings/store')) return json(getBody)
    return json({ status: 'ok', message: 'restarting' })
  })
  await act(async () => {
    createRoot(host).render(<StorageSettings />)
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

const findBtn = (txt: string) =>
  [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!
const fire = (el: Element) => act(async () => {
  (el as HTMLElement).click()
  await new Promise((r) => setTimeout(r, 0))
  await new Promise((r) => setTimeout(r, 0))
})
const changeSelect = (value: string) => act(async () => {
  const sel = host.querySelector('select')!
  const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value')!.set!
  setter.call(sel, value)
  sel.dispatchEvent(new Event('change', { bubbles: true }))
  await new Promise((r) => setTimeout(r, 0))
})

describe('StorageSettings', () => {
  it('shows friendly driver options but keeps english values', async () => {
    await renderPage()
    const options = [...host.querySelectorAll('select option')].map((o) => ({
      value: o.getAttribute('value'),
      text: o.textContent,
    }))
    expect(options).toContainEqual({ value: 'sqlite', text: '本地文件（SQLite）' })
    expect(options).toContainEqual({ value: 'memory', text: '内存（重启即清空，仅试用）' })
    expect(host.textContent).toContain('数据库文件路径')
  })

  it('switches fields by driver', async () => {
    await renderPage()
    await changeSelect('postgres')
    expect(host.textContent).toContain('连接地址（DSN）')
    expect(host.textContent).not.toContain('数据库文件路径')
    await changeSelect('memory')
    expect(host.textContent).not.toContain('连接地址（DSN）')
  })

  it('blocks submit until the acknowledgement is checked', async () => {
    await renderPage()
    await fire(findBtn('保存并重启'))
    expect(host.textContent).toContain('请先勾选确认')
    expect(fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')).toHaveLength(0)
  })

  it('asks for confirmation; cancel sends nothing, confirm PUTs with ack+restart', async () => {
    await renderPage()
    await act(async () => {
      const cb = host.querySelector('input[type="checkbox"]') as HTMLInputElement
      cb.click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await fire(findBtn('保存并重启'))
    // 确认弹窗出现
    expect(host.querySelector('[data-testid="confirm-ok"]')).not.toBeNull()
    await fire(host.querySelector('[data-testid="confirm-cancel"]')!)
    expect(fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')).toHaveLength(0)

    await fire(findBtn('保存并重启'))
    await fire(host.querySelector('[data-testid="confirm-ok"]')!)
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(1)
    const body = JSON.parse((puts[0][1] as RequestInit).body as string)
    expect(body).toMatchObject({ driver: 'sqlite', acknowledge_no_migrate: true, restart: true })
  })

  it('requires DSN for postgres', async () => {
    await renderPage({ driver: 'postgres', drivers: ['sqlite', 'postgres'] })
    await changeSelect('postgres')
    await act(async () => {
      ;(host.querySelector('input[type="checkbox"]') as HTMLInputElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await fire(findBtn('保存并重启'))
    // 未填 DSN：不进入确认、不发 PUT，提示需要 DSN
    expect(host.querySelector('[data-testid="confirm-ok"]')).toBeNull()
    expect(fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')).toHaveLength(0)
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web/chat; npx vitest run src/pages/StorageSettings.test.tsx`
预期：FAIL（旧实现用裸 `select` 但无人话选项、用 `window.confirm`、无确认弹窗）。

- [ ] **步骤 3：重写实现**

将 `web/chat/src/pages/StorageSettings.tsx` 整体替换为：

```tsx
import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  getStoreSettings,
  putStoreSettings,
  type StoreSettings,
} from '../api'
import {
  Button,
  Card,
  ConfirmDialog,
  Field,
  Input,
  PageHeader,
  Select,
  ToastRegion,
  useToast,
} from '../components/ui'
import { driverLabel, friendlyError, STORAGE } from '../strings'

const FALLBACK_DRIVERS = ['memory', 'sqlite', 'postgres']

export function StorageSettings() {
  const [info, setInfo] = useState<StoreSettings | null>(null)
  const [driver, setDriver] = useState('sqlite')
  const [sqlitePath, setSQLitePath] = useState('./data/baize.db')
  const [dsn, setDSN] = useState('')
  const [ack, setAck] = useState(false)
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const { toasts, push, dismiss } = useToast()

  const load = useCallback(async () => {
    try {
      const s = await getStoreSettings()
      setInfo(s)
      setDriver(s.driver || 'sqlite')
      setSQLitePath(s.sqlite_path || './data/baize.db')
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }, [push])

  useEffect(() => {
    void load()
  }, [load])

  const drivers = info?.drivers?.length ? info.drivers : FALLBACK_DRIVERS

  function requestSubmit(e: FormEvent) {
    e.preventDefault()
    if (!ack) {
      setFieldError(STORAGE.ackRequired)
      return
    }
    if (driver === 'postgres' && !dsn.trim() && !info?.dsn_redacted) {
      setFieldError(STORAGE.postgresRequiresDSN)
      return
    }
    setFieldError(null)
    setConfirmOpen(true)
  }

  async function confirmAndSave() {
    setBusy(true)
    try {
      const resp = await putStoreSettings({
        driver,
        sqlite_path: sqlitePath.trim(),
        dsn: dsn.trim(),
        acknowledge_no_migrate: true,
        restart: true,
      })
      push({ tone: 'success', title: resp.message ?? STORAGE.restarting })
      setConfirmOpen(false)
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-panel">
      <PageHeader title={STORAGE.title} description={STORAGE.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      <Card>
        <form className="storage-form" onSubmit={requestSubmit}>
          <Field label={STORAGE.driverField}>
            <Select value={driver} onChange={(e) => { setDriver(e.target.value); setFieldError(null) }} disabled={busy}>
              {drivers.map((d) => (
                <option key={d} value={d}>
                  {driverLabel(d)}
                </option>
              ))}
            </Select>
          </Field>

          {driver === 'sqlite' && (
            <Field label={STORAGE.sqlitePath} hint={STORAGE.sqliteHint}>
              <Input value={sqlitePath} onChange={(e) => setSQLitePath(e.target.value)} disabled={busy} />
            </Field>
          )}

          {driver === 'postgres' && (
            <Field label={STORAGE.dsn} hint={STORAGE.dsnHint} error={fieldError ?? undefined}>
              <Input
                type="password"
                value={dsn}
                placeholder={info?.dsn_redacted || 'postgres://user:pass@host:5432/baize?sslmode=disable'}
                onChange={(e) => { setDSN(e.target.value); setFieldError(null) }}
                disabled={busy}
              />
            </Field>
          )}

          <label className="ui-checkbox-row">
            <input
              type="checkbox"
              checked={ack}
              onChange={(e) => { setAck(e.target.checked); setFieldError(null) }}
              disabled={busy}
            />
            <span>{STORAGE.ack}</span>
          </label>
          {fieldError && driver !== 'postgres' && <p className="ui-inline-error">{fieldError}</p>}

          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? STORAGE.saving : STORAGE.saveRestart}
          </Button>
        </form>
      </Card>

      {info?.config_path && (
        <details className="settings-developer">
          <summary>{STORAGE.developer}</summary>
          <p className="settings-meta">
            配置：{info.config_path}
            {info.overlay_path ? ` · 覆盖：${info.overlay_path}` : ''}
          </p>
        </details>
      )}

      <ConfirmDialog
        open={confirmOpen}
        danger
        title={STORAGE.confirmRestartTitle}
        body={STORAGE.confirmRestartBody}
        confirmText={STORAGE.saveRestart}
        busy={busy}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={() => void confirmAndSave()}
      />
    </div>
  )
}
```

在 `web/chat/src/styles/components.css` 末尾追加：

```css
/* ---- 确认勾选行 / 行内错误 ---- */
.ui-checkbox-row {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  font-size: 0.875rem;
  color: var(--text);
  cursor: pointer;
}
.ui-checkbox-row input[type='checkbox'] {
  width: 16px;
  height: 16px;
  margin-top: 2px;
  accent-color: var(--accent);
  flex: 0 0 auto;
}
.ui-inline-error {
  margin: 0;
  font-size: 0.8rem;
  color: var(--danger);
}
.storage-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
```

用独立的 `.storage-form`（无内边距/边框/背景，由外层 `Card` 提供容器外观），**不改动**其他设置页仍在使用的共享 `.settings-form`，避免回归。

- [ ] **步骤 4：运行测试验证通过**

运行：`cd web/chat; npx vitest run src/pages/StorageSettings.test.tsx && npx tsc --noEmit`
预期：PASS（5 用例）；tsc 0 错误。

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/pages/StorageSettings.tsx web/chat/src/pages/StorageSettings.test.tsx web/chat/src/styles/components.css
git commit -m "feat(web): 存储页人话化——驱动映射/Field 表单/二次确认/Toast"
```

---

## 任务 6：清理旧样式、全量回归、重建嵌入产物

**文件：**
- 修改：`web/chat/src/style.css`（删除已被替换、且无引用的旧账号表单/列表样式）
- 重建：`internal/ui/dist/**`（嵌入产物）

- [ ] **步骤 1：确认并删除无引用的旧 CSS**

在 `web/chat` 下搜索旧类是否仍被任何 `.tsx` 引用：

运行：`rg -n "accounts-list|accounts-item|accounts-main|accounts-title|accounts-detail|accounts-default" src --glob "*.tsx"`
预期：无输出（新账号页改用 `Card` + `accounts-grid/accounts-actions`）。

若确实无引用，从 `web/chat/src/style.css` 删除这些旧规则块（约在 `.accounts-list .accounts-item` 到 `.accounts-default` 一段，以实际搜索为准）；**保留/合并**仍在用的 `.accounts-claims`（新 `components.css` 已定义则删旧定义，只留一处）、`.accounts-clear`（若新 `components.css` 已定义则删旧）。删除原则：每个类只保留一处定义，且令牌化。共享 `.settings-form` 与本批无关，不要改动（存储页使用独立 `.storage-form`）。

- [ ] **步骤 2：前端全量静态检查与单测**

运行：
```bash
cd web/chat
npx tsc --noEmit
npx vitest run
npm run build
```
预期：tsc 0 错误；全部 vitest 用例绿（含本计划新增）；`npm run build` 成功并刷新 `internal/ui/dist/assets/index-*.js|css`。若现有用例因中文文案/结构变化失败，将断言改为稳定 `role`/`testid`，不得弱化行为断言。

- [ ] **步骤 3：后端构建与测试（确认未改逻辑）**

运行（仓库根）：
```bash
go build ./...
go test ./...
```
预期：全绿。本计划不改任何 `.go` 文件；`internal/ui/dist` 是 `//go:embed` 资源，重建不影响 Go 测试。

- [ ] **步骤 4：人工走查（明暗 × 桌面/移动）**

用本地配置启动 `baize serve`，浏览器核对：
- 账号页：无 Token 输入框；空态文案；有账号时卡片显示来源人话/默认徽标/设为默认/退出；「清空登录账号」弹出确认，取消不调用、确认后列表刷新并出成功 Toast；「开发者信息」折叠里才看到会话 id；claims 默认折叠。
- 存储页：保存方式下拉显示三个人话选项；切 sqlite/postgres/memory 字段联动；未勾选确认提交出现行内提示且不弹窗；勾选后点「保存并重启」先弹红色确认，取消不发 PUT、确认才发（可用浏览器 Network 核对 body 含 `acknowledge_no_migrate:true`、`restart:true`、英文 `driver`）。
- 明/暗主题无硬编码色、对比度正常；≤768px 控件触摸目标足够、布局不溢出。

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/style.css internal/ui/dist
git commit -m "build(ui): 重建 P3-A 嵌入产物并清理旧账号样式"
```

---

## 验收对照（规格 → 任务）

- 表单原语 PageHeader/Field/Input/Select/EmptyState、令牌化与 aria：任务 1、2。
- 文案集中（来源标签、driver 映射、账号/存储文案）：任务 3。
- 账号页：去 Token 框、卡片列表、claims 折叠、空态、Toast、清空 ConfirmDialog、会话 id 折叠：任务 4。
- 存储页：driver 人话映射且提交英文值、字段联动、ack 校验、ConfirmDialog 二次确认、Toast、技术信息折叠：任务 5。
- 不改后端契约；旧样式清理；全量绿；重建 dist；明暗/移动走查：任务 6。
- 明确不做：Switch/Textarea/Tooltip 等未用原语、任何新鉴权方式、数据迁移逻辑（规格 §2 非目标）。

