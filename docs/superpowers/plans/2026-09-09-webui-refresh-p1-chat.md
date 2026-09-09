# WebUI ?? P1????? ????

> **?? AI ???????** ???????? superpowers:subagent-driven-development????? superpowers:executing-plans ?????????????????`- [ ]`?????????

**???** ?????"????????????"?????????????Toast/???/???????/HITL/workflow ????????????????????????????

**???** ????????`web/chat`?React 19 + Vite 6 + vitest?????????????????/??? ? ???? ? ???? ? ??????? ? ??????????/JSON ?????"??"??

**????** React 19?react-router-dom 7?Vite 6?TypeScript?vitest?????? node ?????? `// @vitest-environment jsdom` + `react-dom/client` + `act`??? testing-library??`lucide-react`?P0 ????P0 ?????

**???** `docs/superpowers/specs/2026-09-09-webui-refresh-p1-chat.md`

**?????** ??????? `web/chat` ???????????? `internal/ui/dist`?vite ?????

**???????????????**
- `npx tsc --noEmit` ????`npx vitest run` ???
- ??????? `styles/tokens.css` ?????????????
- ??????????? `// @vitest-environment jsdom`??? `src/components/ui/ThemeToggle.test.tsx` ? `createRoot + act` ???
- ??????? conventional commits?`feat(web):` / `fix(web):`??

---

## ??????????????

???
- `web/chat/src/strings.ts` ? ????????????????/??/??/HITL/???/??????????? React?
- `web/chat/src/friendlyTool.ts` ? `friendlyToolName` / `toolPhrase` ????????? + ?? ? ???/??????
- `web/chat/src/historyBlocks.ts` ? `foldToolBlocks(runId, events)` ???????????????/?????? `llm.message`?
- `web/chat/src/components/ui/Toast.tsx` + `Toast.test.tsx` ? `useToast`?`<ToastRegion/>`?
- `web/chat/src/components/ui/Modal.tsx` + `Modal.test.tsx` ? ???????
- `web/chat/src/components/ui/ConfirmDialog.tsx` + `ConfirmDialog.test.tsx` ? ?? Modal ?????? `useConfirm`?
- `web/chat/src/components/ui/DropdownMenu.tsx` + `DropdownMenu.test.tsx` ? `useDropdown` + ?????
- `web/chat/src/components/ModelChip.tsx` + `ModelChip.test.tsx` ? ??????????????????
- `web/chat/src/modelChoice.ts` + `modelChoice.test.ts` ? ????? localStorage ?????????????

???
- `web/chat/src/api.ts` ? ? `parseJSON` ??? `ApiError`?? code?message ?? `HTTP <status>:` ?????????????
- `web/chat/src/modelSelect.ts` ? ??/??/Auto ????????? id ????
- `web/chat/src/components/ToolCard.tsx` ? ????? + ???? + HITL"?????"?????????
- `web/chat/src/components/WorkflowCard.tsx` ? "? n/? m ?" + ??/????
- `web/chat/src/components/Composer.tsx` ? ??????????? children ? props ????????/??????
- `web/chat/src/pages/ChatPage.tsx` ? ? ToastRegion/ConfirmDialog???????historyBlocks ????????????????????????????????? ConfirmDialog???????? Modal?
- `web/chat/src/styles/components.css` ? ?? toast/modal/dropdown ???
- `web/chat/src/style.css` ? ???/???/????/??????????????????? `styles/chat.css` ?? `main.tsx` ????
- `web/chat/src/components/ui/index.ts` ? ??????

---

## ?? 1???/??????`strings.ts`?+ ? `parseJSON` ? `ApiError` + ????

**???**
- ???`web/chat/src/strings.ts`
- ???`web/chat/src/strings.test.ts`
- ???`web/chat/src/api.ts`?? `parseJSON`?? 59-69 ??`ApiError` ????? 882 ??????
- ???`web/chat/src/modelSelect.ts`???/??/Auto ???
- ???????????`web/chat/src/pages/ModelSettings.tsx`????????????
- ?????`web/chat/src/pages/ChatPageModelSelect.test.tsx`?`web/chat/src/pages/ModelSettings.test.tsx`

- [ ] **?? 1.1??? `strings.ts` ?????**

?? `web/chat/src/strings.test.ts`?

```ts
import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import {
  AUTO_LABEL,
  ACTIONS,
  friendlyError,
  HITL,
  tierLabel,
  VISION_LABEL,
  WELCOME,
} from './strings'

describe('tierLabel', () => {
  it('maps internal tier ids to mainstream labels', () => {
    expect(tierLabel('light')).toBe('??')
    expect(tierLabel('standard')).toBe('??')
    expect(tierLabel('power')).toBe('????')
  })
  it('falls back to ?? for unknown/empty tier', () => {
    expect(tierLabel(undefined)).toBe('??')
    expect(tierLabel('nope')).toBe('??')
  })
})

describe('labels', () => {
  it('exposes friendly auto/vision/action/hitl/welcome strings', () => {
    expect(AUTO_LABEL).toBe('????')
    expect(VISION_LABEL).toBe('???')
    expect(ACTIONS.copy).toBe('??')
    expect(ACTIONS.regenerate).toBe('????')
    expect(ACTIONS.editAndReanswer).toBe('???????')
    expect(ACTIONS.forkAsNew).toBe('??????')
    expect(ACTIONS.rollbackHere).toBe('????')
    expect(HITL.title).toBe('?????')
    expect(HITL.approve).toBe('??')
    expect(HITL.reject).toBe('??')
    expect(WELCOME.title).toBe('????????')
  })
})

describe('friendlyError', () => {
  it('maps known ApiError codes to Chinese titles', () => {
    expect(friendlyError(new ApiError(409, 'conversation_busy', 'busy')).title).toContain('??')
    expect(friendlyError(new ApiError(400, 'no_model_configured', 'x')).title).toContain('AI ??')
    expect(friendlyError(new ApiError(400, 'vision_unsupported', 'x')).title).toContain('??')
    expect(friendlyError(new ApiError(401, 'unauthorized', 'x')).title).toContain('??')
    expect(friendlyError(new ApiError(500, 'internal_error', 'x')).title).toContain('??')
    expect(friendlyError(new ApiError(404, 'not_found', 'x')).title).toContain('???')
  })
  it('maps invalid_signature explicitly', () => {
    expect(friendlyError(new ApiError(401, 'invalid_signature', 'bad sig')).title).toContain('??')
  })
  it('falls back to a generic title with detail for unknown codes', () => {
    const r = friendlyError(new ApiError(418, 'weird_teapot', 'boom'))
    expect(r.title).toBeTruthy()
    expect(r.detail).toContain('weird_teapot')
    expect(r.detail).toContain('boom')
  })
  it('treats fetch/network failures as network errors', () => {
    expect(friendlyError(new TypeError('Failed to fetch')).title).toContain('??')
    expect(friendlyError(new Error('NetworkError when attempting to fetch resource')).title).toContain('??')
  })
  it('falls back safely for non-Error values', () => {
    expect(friendlyError('nope').title).toBeTruthy()
  })
})
```

- [ ] **?? 1.2?????????**

???`npx vitest run src/strings.test.ts`
???FAIL?`strings.ts` ??? / ??????

- [ ] **?? 1.3??? `strings.ts`**

?? `web/chat/src/strings.ts`?

```ts
import { ApiError } from './api'

// ---- ???? / ????? id ?????????----
export type TierId = 'light' | 'standard' | 'power'

export const TIER_LABELS: Record<TierId, string> = {
  light: '??',
  standard: '??',
  power: '????',
}

/** ???? id -> ?????????????? */
export function tierLabel(tier?: string): string {
  if (tier === 'light' || tier === 'standard' || tier === 'power') {
    return TIER_LABELS[tier]
  }
  return TIER_LABELS.standard
}

export const AUTO_LABEL = '????'
export const VISION_LABEL = '???'

// ---- ???? ----
export const ACTIONS = {
  copy: '??',
  regenerate: '????',
  editAndReanswer: '???????',
  forkAsNew: '??????',
  rollbackHere: '????',
  more: '????',
} as const

// ---- HITL ----
export const HITL = {
  title: '?????',
  approve: '??',
  reject: '??',
  commentPlaceholder: '??????',
  approved: '???',
  rejected: '???',
} as const

// ---- ??? ----
export const WELCOME = {
  title: '????????',
  subtitle: '??????????????????????????????????',
} as const

// ---- ??? ----
export const ADVANCED = {
  summary: '??',
  tokenLabel: '??????????',
  tokenHint: '???????????????????',
  webhookLabel: '????????????',
} as const

export interface FriendlyError {
  title: string
  /** ?????????????????? */
  detail?: string
}

const CODE_TITLE: Record<string, string> = {
  conversation_busy: '???????????????',
  no_model_configured: '?????? AI ???????? ? AI ????????',
  vision_unsupported: '????????????????????????????????',
  invalid_signature: '?????????????????',
  not_found: '???????????',
  internal_error: '????????????????',
}

function isNetworkish(e: unknown): boolean {
  const msg = e instanceof Error ? e.message : String(e ?? '')
  return /failed to fetch|networkerror|load failed|network request/i.test(msg)
}

/** ?????????????????????????? detail? */
export function friendlyError(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    if (e.status === 401 || e.status === 403) {
      return { title: '??????????????????????' }
    }
    if (e.status >= 500 && !CODE_TITLE[e.code]) {
      return { title: CODE_TITLE.internal_error, detail: `${e.code}: ${e.message}` }
    }
    const title = CODE_TITLE[e.code]
    if (title) return { title }
    return { title: '?????????????', detail: `${e.code}: ${e.message}` }
  }
  if (isNetworkish(e)) {
    return { title: '???????????????????' }
  }
  const detail = e instanceof Error ? e.message : String(e ?? '')
  return { title: '????????', detail: detail || undefined }
}
```

- [ ] **?? 1.4?????????**

???`npx vitest run src/strings.test.ts`
???PASS?

- [ ] **?? 1.5?? `parseJSON` ??? `ApiError`??? `HTTP <status>:` ???**

`web/chat/src/api.ts` ? `parseJSON`?? 59 ??????

```ts
async function parseJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let code = 'unknown'
    let message = res.statusText
    try {
      const body = (await res.json()) as { error?: { code?: string; message?: string } }
      if (body.error?.code) code = body.error.code
      if (body.error?.message) message = body.error.message
    } catch {
      /* ignore */
    }
    // ?? "HTTP <status>:" ????? GateRoot ? startsWith('HTTP 401:') ???
    throw new ApiError(res.status, code, `HTTP ${res.status}: ${message}`)
  }
  return (await res.json()) as T
}
```

??`ApiError` ????????? 882 ??????????`parseConnectorJSON` ???????

- [ ] **?? 1.6?`modelSelect.ts` ??/??/Auto ??????? id ???**

? `modelSelect.ts` ????? `strings` ?????????

```ts
import { AUTO_LABEL, tierLabel, VISION_LABEL } from './strings'
```

`modelOptions` ??
- `const auto = { value: AUTO_MODEL_ID, label: AUTO_LABEL }`
- tags?`p.supports_vision ? VISION_LABEL : ''`
- `tierLabel` ???? `./strings`????????? `tierLabel` ????? `export { tierLabel } from './strings'` ??? `ModelSettings` ?? import ??????

`visionGate` ??? message ????
- ??????`???????????????????? ? AI ????????????????`
- ??????`??????????????????????????????????????`

- [ ] **?? 1.7??? `ModelSettings.tsx` ?????????**

- AutoTier ?????? 15-18 ?????
  - `light`?`??????????????`
  - `standard`?`??????????`
  - `power`?`??????????????`
- ????? label?? 218 ???`???????????`
- ???? `??`?? 292 ???? `{p.supports_vision && <span className="settings-badge">{VISION_LABEL}</span>}`?? `import { VISION_LABEL } from '../strings'`?
- ?????? 258?468-470 ???????/??/????????/??/??????????????????????????Auto??????????????????/???

- [ ] **?? 1.8?????????????**

`ChatPageModelSelect.test.tsx`?
- ? 35?54 ? `label: '?????Auto?'` ? `label: '????'`
- ? 42 ? ? `{ value: 'mp_2', label: '???gpt-4o-mini? ? ??' }`
- ? 50 ? ? `expect(opts[1]).toEqual({ value: 'mp_v', label: '???gpt-4o? ? ??????' })`
- ? 92 ? `expect(r.message).toContain('????')` ? `toContain('???')`
- ? 102 ? `toContain('????')` ? `toContain('????')`
- ? 132 ? `toContain('?????Auto?')` ? `toContain('????')`
- ? 135 ? ? `toContain('???gpt-4o-mini? ? ??')`

`ModelSettings.test.tsx`?
- ? 280 ? `expect(html).toContain('??')` ????????? `toContain('??')`????? profile ?"????"???????????????????? `?????` ?????????????????? profile ????????? `html` ? `????` ???????? `tier` ?? `?????`??
- ? 282 ? `html.match(/??/g)` ? `html.match(/???/g)`?

- [ ] **?? 1.9????????**

???`npx tsc --noEmit`?`npx vitest run`
??????????

```bash
git add web/chat/src/strings.ts web/chat/src/strings.test.ts web/chat/src/api.ts web/chat/src/modelSelect.ts web/chat/src/pages/ModelSettings.tsx web/chat/src/pages/ChatPageModelSelect.test.tsx web/chat/src/pages/ModelSettings.test.tsx
git commit -m "feat(web): P1 ??/???????ApiError ?????????????????/??/????"
```

---

## ?? 2?Toast ????`useToast` + `ToastRegion`?

**???**
- ???`web/chat/src/components/ui/Toast.tsx`
- ???`web/chat/src/components/ui/Toast.test.tsx`
- ???`web/chat/src/styles/components.css`??? toast ???
- ???`web/chat/src/components/ui/index.ts`????

- [ ] **?? 2.1????????jsdom ???**

?? `web/chat/src/components/ui/Toast.test.tsx`?

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastRegion, useToast } from './Toast'

let host: HTMLDivElement
function renderRegion(): { push: ReturnType<typeof useToast>['push'] } {
  let api!: ReturnType<typeof useToast>
  function Harness() {
    api = useToast()
    return <ToastRegion toasts={api.toasts} onDismiss={api.dismiss} />
  }
  act(() => {
    createRoot(host).render(<Harness />)
  })
  return { push: (...a: Parameters<typeof api.push>) => act(() => { api.push(...a) }) }
}

beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  vi.useFakeTimers()
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  vi.useRealTimers()
})

describe('ToastRegion', () => {
  it('renders a toast with polite live region and auto-dismisses', () => {
    const { push } = renderRegion()
    push({ tone: 'success', title: '???' })
    expect(host.textContent).toContain('???')
    const region = host.querySelector('[aria-live="polite"]')
    expect(region).not.toBeNull()
    act(() => { vi.advanceTimersByTime(4100) })
    expect(host.textContent).not.toContain('???')
  })

  it('keeps error toasts longer (6s) and allows manual close', () => {
    const { push } = renderRegion()
    push({ tone: 'error', title: '???', detail: 'boom' })
    act(() => { vi.advanceTimersByTime(4100) })
    expect(host.textContent).toContain('???')
    const close = host.querySelector('[data-testid="toast-close"]') as HTMLButtonElement
    act(() => { close.click() })
    expect(host.textContent).not.toContain('???')
  })

  it('caps the stack at 3 toasts', () => {
    const { push } = renderRegion()
    push({ tone: 'info', title: '?' })
    push({ tone: 'info', title: '?' })
    push({ tone: 'info', title: '?' })
    push({ tone: 'info', title: '?' })
    expect(host.textContent).not.toContain('?')
    expect(host.textContent).toContain('?')
    expect(host.querySelectorAll('[data-testid="toast"]').length).toBe(3)
  })
})
```

- [ ] **?? 2.2???????**

???`npx vitest run src/components/ui/Toast.test.tsx`
???FAIL????????

- [ ] **?? 2.3??? `Toast.tsx`**

```tsx
import { useCallback, useRef, useState } from 'react'

export type ToastTone = 'success' | 'error' | 'info'

export interface ToastInput {
  tone: ToastTone
  title: string
  detail?: string
}

export interface ToastItem extends ToastInput {
  id: number
}

const DURATION_MS: Record<ToastTone, number> = {
  success: 4000,
  info: 4000,
  error: 6000,
}
const MAX_TOASTS = 3

export interface ToastApi {
  toasts: ToastItem[]
  push: (t: ToastInput) => void
  dismiss: (id: number) => void
}

export function useToast(): ToastApi {
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const nextId = useRef(1)

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const push = useCallback(
    (t: ToastInput) => {
      const id = nextId.current++
      setToasts((prev) => [...prev, { ...t, id }].slice(-MAX_TOASTS))
      window.setTimeout(() => dismiss(id), DURATION_MS[t.tone])
    },
    [dismiss],
  )

  return { toasts, push, dismiss }
}

export function ToastRegion({
  toasts,
  onDismiss,
}: {
  toasts: ToastItem[]
  onDismiss: (id: number) => void
}) {
  return (
    <div className="toast-region" aria-live="polite" aria-atomic="false">
      {toasts.map((t) => (
        <div
          key={t.id}
          data-testid="toast"
          className={`toast toast-${t.tone}`}
          role="status"
        >
          <div className="toast-body">
            <p className="toast-title">{t.title}</p>
            {t.detail && <p className="toast-detail">{t.detail}</p>}
          </div>
          <button
            type="button"
            className="toast-close"
            data-testid="toast-close"
            aria-label="????"
            onClick={() => onDismiss(t.id)}
          >
            ?
          </button>
        </div>
      ))}
    </div>
  )
}
```

- [ ] **?? 2.4?????? `components.css`??????**

```css
/* ---- Toast ---- */
.toast-region {
  position: fixed;
  top: var(--space-4);
  right: var(--space-4);
  z-index: 1000;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  max-width: min(360px, calc(100vw - 2 * var(--space-4)));
}
.toast {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  background: var(--surface);
  border: 1px solid var(--border);
  border-left-width: 4px;
  border-radius: var(--radius);
  box-shadow: var(--shadow-2);
  padding: var(--space-3) var(--space-4);
}
.toast-success { border-left-color: var(--success); }
.toast-error { border-left-color: var(--danger); }
.toast-info { border-left-color: var(--info); }
.toast-title { margin: 0; font-weight: 600; color: var(--text); }
.toast-detail { margin: 4px 0 0; font-size: 13px; color: var(--text-muted); word-break: break-word; }
.toast-close {
  margin-left: auto;
  border: 0;
  background: transparent;
  color: var(--text-muted);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
}
.toast-close:focus-visible { outline: 2px solid var(--info); outline-offset: 2px; }
```

- [ ] **?? 2.5?? barrel ?????**

? `components/ui/index.ts` ???

```ts
export { ToastRegion, useToast, type ToastInput, type ToastTone, type ToastApi } from './Toast'
```

???`npx vitest run src/components/ui/Toast.test.tsx`?`npx tsc --noEmit`
???PASS?????

- [ ] **?? 2.6???**

```bash
git add web/chat/src/components/ui/Toast.tsx web/chat/src/components/ui/Toast.test.tsx web/chat/src/styles/components.css web/chat/src/components/ui/index.ts
git commit -m "feat(web): P1 Toast ????????/????/??3?/aria-live?"
```

---

## ?? 3?Modal ?? + ConfirmDialog???????

**???**
- ???`web/chat/src/components/ui/Modal.tsx` + `Modal.test.tsx`
- ???`web/chat/src/components/ui/ConfirmDialog.tsx` + `ConfirmDialog.test.tsx`
- ???`web/chat/src/styles/components.css`
- ???`web/chat/src/components/ui/index.ts`

- [ ] **?? 3.1??? Modal ????**

`Modal.test.tsx`?

```tsx
// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Modal } from './Modal'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => { host.remove() })

function render(el: ReactNode) {
  act(() => { createRoot(host).render(el) })
}

describe('Modal', () => {
  it('renders nothing when closed, renders dialog when open with role/title', () => {
    render(<Modal open={false} title="T"><p>body</p></Modal>)
    expect(host.textContent).not.toContain('body')
    render(<Modal open title="T"><p>body</p></Modal>)
    const dlg = host.querySelector('[role="dialog"]')
    expect(dlg).not.toBeNull()
    expect(host.textContent).toContain('T')
    expect(host.textContent).toContain('body')
  })
  it('closes on Escape and overlay click, not on dialog content click', () => {
    const onClose = vi.fn()
    render(<Modal open title="T" onClose={onClose}><span data-testid="in">x</span></Modal>)
    act(() => {
      host.querySelector('[role="dialog"]')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(onClose).toHaveBeenCalledOnce()
    act(() => { (host.querySelector('[data-testid="modal-overlay"]') as HTMLElement).click() })
    expect(onClose).toHaveBeenCalledTimes(2)
    onClose.mockClear()
    act(() => { (host.querySelector('[data-testid="modal-panel"]') as HTMLElement).click() })
    expect(onClose).not.toHaveBeenCalled()
  })
})
```

- [ ] **?? 3.2??? `Modal.tsx`**

```tsx
import { useEffect, useRef, type ReactNode } from 'react'

export interface ModalProps {
  open: boolean
  title: string
  onClose?: () => void
  children: ReactNode
  footer?: ReactNode
}

export function Modal({ open, title, onClose, children, footer }: ModalProps) {
  const panelRef = useRef<HTMLDivElement>(null)
  const previousActive = useRef<Element | null>(null)

  useEffect(() => {
    if (!open) return
    previousActive.current = document.activeElement
    const close = onClose
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close?.()
    }
    document.addEventListener('keydown', onKey)
    const focusables = panelRef.current?.querySelectorAll<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    )
    ;(focusables?.[0] ?? panelRef.current)?.focus()
    return () => {
      document.removeEventListener('keydown', onKey)
      ;(previousActive.current as HTMLElement | null)?.focus?.()
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="modal-overlay"
      data-testid="modal-overlay"
      onClick={(e) => { if (e.target === e.currentTarget) onClose?.() }}
    >
      <div
        ref={panelRef}
        className="modal-panel"
        data-testid="modal-panel"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
      >
        <h2 className="modal-title">{title}</h2>
        <div className="modal-content">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>
  )
}
```

????? `keydown` ??? document???????? document????

- [ ] **?? 3.3??? Modal ??? `components.css`**

```css
/* ---- Modal ---- */
.modal-overlay {
  position: fixed; inset: 0; z-index: 1100;
  background: rgba(15, 23, 42, 0.45);
  display: flex; align-items: center; justify-content: center;
  padding: var(--space-4);
}
.modal-panel {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-3);
  width: min(440px, 100%);
  padding: var(--space-6);
  outline: none;
}
.modal-title { margin: 0 0 var(--space-2); font-size: 17px; color: var(--text); }
.modal-content { color: var(--text); font-size: 14px; line-height: 1.6; }
.modal-footer { display: flex; justify-content: flex-end; gap: var(--space-2); margin-top: var(--space-4); }
```

- [ ] **?? 3.4??? Modal ????**

???`npx vitest run src/components/ui/Modal.test.tsx`??? PASS?

- [ ] **?? 3.5??? ConfirmDialog ????**

`ConfirmDialog.test.tsx`?

```tsx
// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConfirmDialog } from './ConfirmDialog'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => host.remove())
function render(el: ReactNode) { act(() => { createRoot(host).render(el) }) }

describe('ConfirmDialog', () => {
  it('shows consequence text and calls onConfirm/onCancel', () => {
    const onConfirm = vi.fn(); const onCancel = vi.fn()
    render(<ConfirmDialog open title="?????" body="???????????" confirmText="??" danger onConfirm={onConfirm} onCancel={onCancel} />)
    expect(host.textContent).toContain('????')
    act(() => { (host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click() })
    expect(onConfirm).toHaveBeenCalledOnce()
    act(() => { (host.querySelector('[data-testid="confirm-cancel"]') as HTMLButtonElement).click() })
    expect(onCancel).toHaveBeenCalledOnce()
  })
  it('initial focus lands on the cancel button for dangerous confirms', () => {
    render(<ConfirmDialog open title="x" body="y" danger confirmText="??" onConfirm={() => {}} onCancel={() => {}} />)
    const cancel = host.querySelector('[data-testid="confirm-cancel"]') as HTMLButtonElement
    expect(document.activeElement).toBe(cancel)
  })
})
```


- [ ] **?? 3.6??? `ConfirmDialog.tsx`**

```tsx
import { Button } from './Button'
import { Modal } from './Modal'

export interface ConfirmDialogProps {
  open: boolean
  title: string
  body: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
  busy?: boolean
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmDialog({
  open, title, body, confirmText = '??', cancelText = '??',
  danger, busy, onConfirm, onCancel,
}: ConfirmDialogProps) {
  return (
    <Modal
      open={open}
      title={title}
      onClose={onCancel}
      footer={
        <>
          <Button data-testid="confirm-cancel" variant="secondary" onClick={onCancel} disabled={busy}>
            {cancelText}
          </Button>
          <Button
            data-testid="confirm-ok"
            variant={danger ? 'danger' : 'primary'}
            onClick={onConfirm}
            disabled={busy}
          >
            {confirmText}
          </Button>
        </>
      }
    >
      <p className="confirm-body">{body}</p>
    </Modal>
  )
}
```

??`Button` ?? `data-testid`?? `...rest`??????"???????"? Modal ?????????cancel ??? footer ?????????????????

- [ ] **?? 3.7?????????**

`index.ts` ???

```ts
export { Modal, type ModalProps } from './Modal'
export { ConfirmDialog, type ConfirmDialogProps } from './ConfirmDialog'
```

???`npx vitest run src/components/ui/Modal.test.tsx src/components/ui/ConfirmDialog.test.tsx`?`npx tsc --noEmit`?

```bash
git add web/chat/src/components/ui/Modal.tsx web/chat/src/components/ui/Modal.test.tsx web/chat/src/components/ui/ConfirmDialog.tsx web/chat/src/components/ui/ConfirmDialog.test.tsx web/chat/src/styles/components.css web/chat/src/components/ui/index.ts
git commit -m "feat(web): P1 Modal ??? ConfirmDialog?Esc/???????????????"
```

---

## ?? 4?DropdownMenu?`?` ???????/?????

**???**
- ???`web/chat/src/components/ui/DropdownMenu.tsx` + `DropdownMenu.test.tsx`
- ???`web/chat/src/styles/components.css`?`components/ui/index.ts`

- [ ] **?? 4.1???????**

`DropdownMenu.test.tsx`?

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DropdownMenu, type MenuItem } from './DropdownMenu'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => host.remove())

const items: MenuItem[] = [
  { id: 'copy', label: '??', onSelect: vi.fn() },
  { id: 'fork', label: '??????', onSelect: vi.fn() },
]
function render() {
  act(() => {
    createRoot(host).render(
      <DropdownMenu triggerLabel="????" items={items} />,
    )
  })
}

describe('DropdownMenu', () => {
  it('toggles on trigger click and exposes aria-expanded/menuitem', () => {
    render()
    const trigger = host.querySelector('[data-testid="dropdown-trigger"]') as HTMLButtonElement
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    act(() => { trigger.click() })
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    expect(host.querySelectorAll('[role="menuitem"]').length).toBe(2)
  })
  it('selects an item, fires onSelect, and closes', () => {
    render()
    act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
    act(() => { (host.querySelectorAll('[role="menuitem"]')[1] as HTMLElement).click() })
    expect(items[1].onSelect).toHaveBeenCalledOnce()
    expect((host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).getAttribute('aria-expanded')).toBe('false')
  })
  it('closes on Escape and on outside click', () => {
    render()
    act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(host.querySelector('[role="menu"]')).toBeNull()
    act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
    act(() => { document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true })) })
    expect(host.querySelector('[role="menu"]')).toBeNull()
  })
  it('moves highlight with ArrowDown/Up and activates with Enter', () => {
    render()
    const trigger = host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement
    act(() => { trigger.click() })
    act(() => { trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })) })
    act(() => {
      ;(host.querySelector('[role="menu"]') as HTMLElement).dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }),
      )
    })
    expect(items[0].onSelect).toHaveBeenCalledOnce()
  })
})
```

- [ ] **?? 4.2??? `DropdownMenu.tsx`**

```tsx
import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'

export interface MenuItem {
  id: string
  label: string
  icon?: ReactNode
  destructive?: boolean
  onSelect: () => void
}

export interface DropdownMenuProps {
  triggerLabel: string
  items: MenuItem[]
  triggerClassName?: string
  disabled?: boolean
  children?: ReactNode
}

export function DropdownMenu({ triggerLabel, items, triggerClassName, disabled = false, children }: DropdownMenuProps) {
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const rootRef = useRef<HTMLDivElement>(null)
  const menuId = useId()

  const close = useCallback(() => setOpen(false), [])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) close()
    }
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') close() }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, close])

  const choose = (item: MenuItem) => { item.onSelect(); close() }

  const onMenuKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setActive((a) => (a + 1) % items.length) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActive((a) => (a - 1 + items.length) % items.length) }
    else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); const it = items[active]; if (it) choose(it) }
  }

  const onTriggerKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown' && open) {
      e.preventDefault()
      setActive(0)
    }
  }

  return (
    <div className="dropdown" ref={rootRef}>
      <button
        type="button"
        className={`dropdown-trigger ${triggerClassName ?? ''}`.trim()}
        data-testid="dropdown-trigger"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-label={triggerLabel}
        title={triggerLabel}
        disabled={disabled}
        onClick={() => { if (!disabled) { setOpen((v) => !v); setActive(0) } }}
        onKeyDown={onTriggerKey}
      >
        {children ?? '?'}
      </button>
      {open && (
        <div
          id={menuId}
          className="dropdown-menu"
          role="menu"
          aria-label={triggerLabel}
          onKeyDown={onMenuKey}
        >
          {items.map((item, i) => (
            <button
              key={item.id}
              type="button"
              role="menuitem"
              className={`dropdown-item${item.destructive ? ' danger' : ''}${i === active ? ' active' : ''}`}
              tabIndex={-1}
              onMouseEnter={() => setActive(i)}
              onClick={() => choose(item)}
              ref={(el) => { if (i === active && open) el?.focus() }}
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
```

?????????ref ?? focus active ????? Enter ????/????????? menu ???? Enter ??? active ??

- [ ] **?? 4.3?????**

```css
/* ---- DropdownMenu ---- */
.dropdown { position: relative; display: inline-flex; }
.dropdown-trigger {
  border: 0; background: transparent; color: var(--text-muted);
  min-width: 32px; min-height: 32px; border-radius: var(--radius-sm);
  cursor: pointer; font-size: 18px; line-height: 1; padding: 0 var(--space-1);
}
.dropdown-trigger:hover { background: var(--surface-2); color: var(--text); }
.dropdown-trigger:focus-visible { outline: 2px solid var(--info); outline-offset: 1px; }
.dropdown-menu {
  position: absolute; right: 0; bottom: calc(100% + 6px);
  min-width: 180px; z-index: 1050;
  background: var(--surface); border: 1px solid var(--border);
  border-radius: var(--radius); box-shadow: var(--shadow-2);
  padding: var(--space-1); display: flex; flex-direction: column;
}
.dropdown-item {
  display: flex; align-items: center; gap: var(--space-2);
  border: 0; background: transparent; width: 100%;
  text-align: left; padding: 10px var(--space-3);
  border-radius: var(--radius-sm); color: var(--text);
  font-size: 14px; cursor: pointer; min-height: 40px;
}
.dropdown-item:hover, .dropdown-item.active { background: var(--surface-2); }
.dropdown-item.danger { color: var(--danger); }
.dropdown-item:focus-visible { outline: 2px solid var(--info); outline-offset: -2px; }
```

- [ ] **?? 4.4?????????**

`index.ts` ???

```ts
export { DropdownMenu, type MenuItem, type DropdownMenuProps } from './DropdownMenu'
```

???`npx vitest run src/components/ui/DropdownMenu.test.tsx`?`npx tsc --noEmit`?

```bash
git add web/chat/src/components/ui/DropdownMenu.tsx web/chat/src/components/ui/DropdownMenu.test.tsx web/chat/src/styles/components.css web/chat/src/components/ui/index.ts
git commit -m "feat(web): P1 DropdownMenu ???????/Esc ???????Enter??????40?"
```

---

## ?? 5???????????????????

**???**
- ???`web/chat/src/friendlyTool.ts` + `friendlyTool.test.ts`
- ???`web/chat/src/historyBlocks.ts` + `historyBlocks.test.ts`

- [ ] **?? 5.1??? `friendlyTool` ????**

`friendlyTool.test.ts`?

```ts
import { describe, expect, it } from 'vitest'
import { friendlyToolName, toolPhrase, type ToolCatalog } from './friendlyTool'

const catalog: ToolCatalog = [
  { name: 'query_order', title: '????', description: '??????' },
  { name: 'no_title', title: '', description: '' },
]

describe('friendlyToolName', () => {
  it('prefers catalog title, falls back to technical name', () => {
    expect(friendlyToolName('query_order', catalog)).toBe('????')
    expect(friendlyToolName('no_title', catalog)).toBe('no_title')
    expect(friendlyToolName('totally_new', catalog)).toBe('totally_new')
    expect(friendlyToolName('query_order', [])).toBe('query_order')
  })
})

describe('toolPhrase', () => {
  it('builds human phrases per status without fabricating verbs', () => {
    expect(toolPhrase('query_order', 'running', catalog)).toBe('??????????')
    expect(toolPhrase('query_order', 'waiting_human', catalog)).toBe('????????')
    expect(toolPhrase('query_order', 'succeeded', catalog)).toBe('????????')
    expect(toolPhrase('query_order', 'failed', catalog)).toBe('?????????')
    expect(toolPhrase('query_order', 'approved', catalog)).toBe('????????')
    expect(toolPhrase('query_order', 'rejected', catalog)).toBe('????????')
  })
  it('uses technical name in phrase when no title', () => {
    expect(toolPhrase('totally_new', 'running', catalog)).toBe('?????totally_new?')
  })
})
```

- [ ] **?? 5.2??? `friendlyTool.ts`**

```ts
import type { ToolInfo } from './api'

export type ToolCatalog = Array<Pick<ToolInfo, 'name' | 'title' | 'description'>>

type ToolStatus =
  | 'running'
  | 'waiting_human'
  | 'succeeded'
  | 'failed'
  | 'approved'
  | 'rejected'

const PHRASE: Record<ToolStatus, (label: string) => string> = {
  running: (l) => `?????${l}?`,
  waiting_human: (l) => `????${l}`,
  succeeded: (l) => `????${l}`,
  failed: (l) => `?????${l}`,
  approved: (l) => `????${l}`,
  rejected: (l) => `????${l}`,
}

/** ????? -> ????? title ? title????????? */
export function friendlyToolName(name: string, catalog: ToolCatalog): string {
  const hit = catalog.find((t) => t.name === name)
  const title = hit?.title?.trim()
  return title || name
}

/** ?????????????????????? */
export function toolPhrase(name: string, status: ToolStatus, catalog: ToolCatalog): string {
  return PHRASE[status](friendlyToolName(name, catalog))
}
```

- [ ] **?? 5.3??? `historyBlocks` ????**

`historyBlocks.test.ts`?

```ts
import { describe, expect, it } from 'vitest'
import type { Event } from './api'
import { foldToolBlocks } from './historyBlocks'

function ev(type: string, data: Record<string, unknown>): Event {
  return { type, timestamp: '', data }
}

describe('foldToolBlocks', () => {
  it('keeps tool/workflow blocks in event order and drops assistant text', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'a' }),
      ev('tool.result', { name: 'a', content: 'ok' }),
      ev('llm.message', { content: 'assistant words' }),
      ev('workflow.started', { skill: 's', steps: ['x'] }),
    ]
    const blocks = foldToolBlocks('run-1', events)
    expect(blocks.map((b) => b.kind)).toEqual(['tool', 'workflow'])
    const tool = blocks[0]
    expect(tool.kind === 'tool' && tool.name).toBe('a')
    expect(tool.kind === 'tool' && tool.status).toBe('succeeded')
    expect(blocks.every((b) => b.runId === 'run-1')).toBe(true)
  })
  it('returns [] when there are no tool/workflow events', () => {
    expect(foldToolBlocks('r', [ev('llm.message', { content: 'hi' })])).toEqual([])
  })
  it('folds HITL waiting into a tool block for read-only rendering', () => {
    const blocks = foldToolBlocks('r', [
      ev('llm.tool_call', { name: 'a' }),
      ev('hitl.waiting', { tool_name: 'a', arguments: { k: 1 } }),
    ])
    expect(blocks).toHaveLength(1)
    expect(blocks[0].kind === 'tool' && blocks[0].status).toBe('waiting_human')
  })
})
```

- [ ] **?? 5.4??? `historyBlocks.ts`??? `foldEvents`?DRY?**

```ts
import type { Event } from './api'
import { foldEvents, type ChatBlock } from './foldEvents'

export type ToolOrWorkflowBlock = Extract<ChatBlock, { kind: 'tool' | 'workflow' }>

/**
 * ???? run ????????/?????? llm.message ??????
 * ???????????????????????????????
 */
export function foldToolBlocks(runId: string, events: Event[]): ToolOrWorkflowBlock[] {
  return foldEvents(runId, events).filter(
    (b): b is ToolOrWorkflowBlock => b.kind === 'tool' || b.kind === 'workflow',
  )
}
```

- [ ] **?? 5.5??????**

???`npx vitest run src/friendlyTool.test.ts src/historyBlocks.test.ts`?`npx tsc --noEmit`?

```bash
git add web/chat/src/friendlyTool.ts web/chat/src/friendlyTool.test.ts web/chat/src/historyBlocks.ts web/chat/src/historyBlocks.test.ts
git commit -m "feat(web): P1 ?????/??????? + ???????????????"
```

---

## ?? 6?ToolCard / WorkflowCard ????? HITL????????

**???**
- ???`web/chat/src/components/ToolCard.tsx`??? `ToolCard.test.tsx`
- ???`web/chat/src/components/WorkflowCard.tsx`??? `WorkflowCard.test.tsx`
- ???`web/chat/src/style.css` ?????????????????????/?????

- [ ] **?? 6.1??? ToolCard ??????? node ???**

`ToolCard.test.tsx`?

```tsx
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../foldEvents'
import { ToolCard } from './ToolCard'

const catalog = [
  { name: 'query_order', title: '????', description: '?????' },
]
const tool = (over: Partial<Extract<ChatBlock, { kind: 'tool' }>> = {}) =>
  ({
    kind: 'tool', name: 'query_order', status: 'running', runId: 'r1', ...over,
  }) as Extract<ChatBlock, { kind: 'tool' }>

describe('ToolCard', () => {
  it('shows a friendly phrase and hides technical name when collapsed', () => {
    const html = renderToStaticMarkup(<ToolCard block={tool()} catalog={catalog} />)
    expect(html).toContain('?????????')
    expect(html).not.toContain('query_order')
  })
  it('falls back to technical name when not in catalog', () => {
    const html = renderToStaticMarkup(<ToolCard block={tool({ name: 'zzz' })} catalog={[]} />)
    expect(html).toContain('zzz')
  })
  it('renders ????? with ??/?? and a comment box driven by ??', () => {
    const html = renderToStaticMarkup(
      <ToolCard block={tool({ status: 'waiting_human' })} catalog={catalog} />,
    )
    expect(html).toContain('?????')
    expect(html).toContain('??')
    expect(html).toContain('??')
  })
  it('readOnly renders no action buttons for historical HITL', () => {
    const html = renderToStaticMarkup(
      <ToolCard block={tool({ status: 'waiting_human' })} catalog={catalog} readOnly />,
    )
    expect(html).not.toContain('??')
    expect(html).toContain('???')
  })
})
```

- [ ] **?? 6.2??? `ToolCard.tsx`**

```tsx
import { useEffect, useState } from 'react'
import { Check, ChevronDown, Loader2, X } from 'lucide-react'
import { parseAnalysisPageResult } from '../analysisPage'
import { resumeRun } from '../api'
import { friendlyToolName, toolPhrase, type ToolCatalog } from '../friendlyTool'
import type { ChatBlock } from '../foldEvents'
import { HITL } from '../strings'
import { AnalysisPagePreview } from './AnalysisPagePreview'
import { Button } from './ui'

type ToolBlock = Extract<ChatBlock, { kind: 'tool' }>

export interface ToolCardProps {
  block: ToolBlock
  catalog?: ToolCatalog
  /** ???????????????????? */
  readOnly?: boolean
  onResumed?: () => void
}

export function ToolCard({ block, catalog = [], readOnly = false, onResumed }: ToolCardProps) {
  const waiting = block.status === 'waiting_human' && !readOnly
  const [expanded, setExpanded] = useState(waiting)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showComment, setShowComment] = useState(false)
  const [comment, setComment] = useState('')

  const analysisPage = block.result !== undefined ? parseAnalysisPageResult(block.result) : null
  const label = friendlyToolName(block.name, catalog)
  const description = catalog.find((t) => t.name === block.name)?.description?.trim() || ''

  useEffect(() => {
    if (block.status === 'waiting_human' && !readOnly) setExpanded(true)
  }, [block.status, readOnly])

  const decide = async (decision: 'approve' | 'reject') => {
    setBusy(true); setError(null)
    try {
      await resumeRun(block.runId, decision, decision === 'reject' ? comment.trim() : '')
      onResumed?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const icon =
    block.status === 'running' || block.status === 'approved' ? (
      <Loader2 size={15} className="icon-spin" aria-hidden />
    ) : block.status === 'failed' || block.status === 'rejected' ? (
      <X size={15} aria-hidden />
    ) : (
      <Check size={15} aria-hidden />
    )

  return (
    <div className={`tool-card${waiting ? ' tool-card-waiting' : ''}`}>
      {waiting ? (
        <div className="hitl-head">
          <p className="hitl-title">{HITL.title}</p>
          <p className="hitl-desc">{label}{description ? ` ? ${description}` : ''}</p>
        </div>
      ) : (
        <button
          type="button"
          className="tool-card-header"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
        >
          <span className="tool-card-phrase">
            {icon}
            {toolPhrase(block.name, block.status, catalog)}
          </span>
          <ChevronDown size={14} className={`tool-card-chevron${expanded ? ' expanded' : ''}`} aria-hidden />
        </button>
      )}

      {analysisPage && (
        <div className="tool-card-preview"><AnalysisPagePreview artifactUrl={analysisPage.artifactUrl} /></div>
      )}

      {expanded && (
        <div className="tool-card-body">
          <p className="tool-card-techname">???{block.name}</p>
          {block.arguments !== undefined && <pre className="tool-card-json">{formatJSON(block.arguments)}</pre>}
          {block.result !== undefined &&
            (analysisPage ? (
              <details className="tool-card-details"><summary>??</summary>
                <pre className="tool-card-json">{formatJSON(block.result)}</pre>
              </details>
            ) : (
              <pre className="tool-card-json">{formatJSON(block.result)}</pre>
            ))}
        </div>
      )}

      {waiting && (
        <div className="tool-card-actions">
          {showComment && (
            <input
              className="hitl-comment"
              type="text"
              value={comment}
              disabled={busy}
              placeholder={HITL.commentPlaceholder}
              onChange={(e) => setComment(e.target.value)}
            />
          )}
          <Button size="sm" variant="primary" disabled={busy} onClick={() => void decide('approve')}>
            {HITL.approve}
          </Button>
          {!showComment ? (
            <Button size="sm" variant="danger" disabled={busy} onClick={() => setShowComment(true)}>
              {HITL.reject}
            </Button>
          ) : (
            <Button size="sm" variant="danger" disabled={busy} onClick={() => void decide('reject')}>
              ????
            </Button>
          )}
          <button type="button" className="tool-card-detailbtn" onClick={() => setExpanded((v) => !v)}>
            ???
          </button>
        </div>
      )}

      {error && <p className="tool-card-error">{error}</p>}
    </div>
  )
}

function formatJSON(value: unknown): string {
  try { return JSON.stringify(value, null, 2) } catch { return String(value) }
}
```

- [ ] **?? 6.3???/????????????? `style.css`**

?? `.tool-card-header/.tool-card-name/.tool-card-status` ????????????????

```css
.tool-card-phrase { display: inline-flex; align-items: center; gap: 6px; }
.icon-spin { animation: baize-spin 1s linear infinite; }
@keyframes baize-spin { to { transform: rotate(360deg); } }
.tool-card-chevron { transition: transform .15s ease; }
.tool-card-chevron.expanded { transform: rotate(180deg); }
.hitl-head { margin-bottom: 6px; }
.hitl-title { margin: 0; font-weight: 600; color: var(--text); }
.hitl-desc { margin: 2px 0 0; font-size: 13px; color: var(--text-muted); }
.tool-card-techname { margin: 0 0 4px; font-size: 12px; color: var(--text-faint); }
.tool-card-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); align-items: center; margin-top: 8px; }
.hitl-comment { flex: 1 1 160px; min-width: 0; padding: 6px 10px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); }
.tool-card-detailbtn { border: 0; background: none; color: var(--text-muted); cursor: pointer; font-size: 12px; }
```

- [ ] **?? 6.4??? WorkflowCard ???????**

`WorkflowCard.test.tsx`?

```tsx
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../foldEvents'
import { WorkflowCard } from './WorkflowCard'

const wf = (steps: Array<{ id: string; status: 'pending' | 'running' | 'done' | 'failed' }>) =>
  ({ kind: 'workflow', skill: 's', runId: 'r', steps }) as Extract<ChatBlock, { kind: 'workflow' }>

describe('WorkflowCard', () => {
  it('shows ? n/? m ? and numbered steps without raw ids', () => {
    const html = renderToStaticMarkup(<WorkflowCard block={wf([
      { id: 'a', status: 'done' }, { id: 'b', status: 'running' }, { id: 'c', status: 'pending' },
    ])} />)
    expect(html).toContain('? 2 / ? 3 ?')
    expect(html).toContain('?? 1')
    expect(html).not.toContain('>a<')
  })
})
```

?? `WorkflowCard.tsx`?

```tsx
import { Check, Loader2, X } from 'lucide-react'
import type { ChatBlock, WorkflowStepStatus } from '../foldEvents'

type WorkflowBlock = Extract<ChatBlock, { kind: 'workflow' }>

export function WorkflowCard({ block }: { block: WorkflowBlock }) {
  const total = block.steps.length
  const current = block.steps.filter((s) => s.status === 'done' || s.status === 'running').length
  return (
    <div className="tool-card workflow-card">
      <div className="tool-card-header workflow-card-header">
        <span className="workflow-progress">? {Math.max(current, 1)} / ? {total} ?</span>
      </div>
      <ol className="workflow-steps">
        {block.steps.map((step, i) => (
          <li key={step.id} className={`workflow-step workflow-step-${step.status}`} data-status={step.status}>
            <StepIcon status={step.status} />
            <span>?? {i + 1}</span>
          </li>
        ))}
      </ol>
    </div>
  )
}

function StepIcon({ status }: { status: WorkflowStepStatus }) {
  if (status === 'running') return <Loader2 size={13} className="icon-spin" aria-hidden />
  if (status === 'failed') return <X size={13} aria-hidden />
  if (status === 'done') return <Check size={13} aria-hidden />
  return <span className="workflow-step-dot" aria-hidden />
}
```

- [ ] **?? 6.5??????**

???`npx vitest run src/components/ToolCard.test.tsx src/components/WorkflowCard.test.tsx`?`npx tsc --noEmit`?
??????? ToolCard ???? `catalog` ????? `catalog` ?????? `[]`???????????

```bash
git add web/chat/src/components/ToolCard.tsx web/chat/src/components/ToolCard.test.tsx web/chat/src/components/WorkflowCard.tsx web/chat/src/components/WorkflowCard.test.tsx web/chat/src/style.css
git commit -m "feat(web): P1 ??/HITL/workflow ????????????????(?????)??n/m?"
```

---

## ?? 7?????????`modelChoice.ts`?+ ????????`ModelChip`?

**???**
- ???`web/chat/src/modelChoice.ts` + `modelChoice.test.ts`
- ???`web/chat/src/components/ModelChip.tsx` + `ModelChip.test.tsx`
- ???`web/chat/src/components/Composer.tsx`??? `toolbar` ??????

- [ ] **?? 7.1??? `modelChoice` ????**

`modelChoice.test.ts`??? `// @vitest-environment jsdom`?beforeEach `localStorage.clear()`??

```ts
import { describe, expect, it, beforeEach } from 'vitest'
import { loadModelChoice, saveModelChoice, resolveModelChoice, MODEL_CHOICE_KEY } from './modelChoice'
import { AUTO_MODEL_ID } from './modelSelect'
import type { ModelProfile } from './api'

beforeEach(() => localStorage.clear())
const prof = (id: string): ModelProfile =>
  ({ id, name: id, model: 'm', auto_tier: 'standard', supports_vision: false }) as ModelProfile

describe('modelChoice persistence', () => {
  it('defaults to auto and round-trips a manual choice', () => {
    expect(loadModelChoice()).toBe(AUTO_MODEL_ID)
    saveModelChoice('mp_1')
    expect(localStorage.getItem(MODEL_CHOICE_KEY)).toBe('mp_1')
    expect(loadModelChoice()).toBe('mp_1')
  })
  it('saving auto stores the auto sentinel', () => {
    saveModelChoice('mp_1'); saveModelChoice(AUTO_MODEL_ID)
    expect(loadModelChoice()).toBe(AUTO_MODEL_ID)
  })
  it('falls back to auto (and flags stale) when the chosen profile is gone', () => {
    saveModelChoice('mp_gone')
    const r = resolveModelChoice(['mp_keep'].map(prof))
    expect(r.choice).toBe(AUTO_MODEL_ID)
    expect(r.stale).toBe(true)
  })
  it('keeps a still-existing manual choice', () => {
    saveModelChoice('mp_keep')
    const r = resolveModelChoice(['mp_keep'].map(prof))
    expect(r.choice).toBe('mp_keep'); expect(r.stale).toBe(false)
  })
})
```

- [ ] **?? 7.2??? `modelChoice.ts`**

```ts
import type { ModelProfile } from './api'
import { AUTO_MODEL_ID } from './modelSelect'

export const MODEL_CHOICE_KEY = 'baize.model_choice'

/** ?????????????/???? Auto? */
export function loadModelChoice(): string {
  try {
    return localStorage.getItem(MODEL_CHOICE_KEY)?.trim() || AUTO_MODEL_ID
  } catch {
    return AUTO_MODEL_ID
  }
}

export function saveModelChoice(id: string): void {
  try {
    localStorage.setItem(MODEL_CHOICE_KEY, id.trim() || AUTO_MODEL_ID)
  } catch {
    /* ?????????????????????? */
  }
}

export interface ResolvedChoice {
  choice: string
  /** ?????????????????????????? Auto? */
  stale: boolean
}

/** ???????????Auto ??????????????? */
export function resolveModelChoice(profiles: Pick<ModelProfile, 'id'>[]): ResolvedChoice {
  const raw = loadModelChoice()
  if (raw === AUTO_MODEL_ID) return { choice: AUTO_MODEL_ID, stale: false }
  if (profiles.some((p) => p.id === raw)) return { choice: raw, stale: false }
  return { choice: AUTO_MODEL_ID, stale: true }
}
```

- [ ] **?? 7.3??? `ModelChip` ?????jsdom?**

`ModelChip.test.tsx`?

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { ModelChip } from './ModelChip'
import type { ModelProfile } from '../api'
import { AUTO_MODEL_ID } from '../modelSelect'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => host.remove())
const profiles = [
  { id: 'mp1', name: '??', model: 'mini', auto_tier: 'light', supports_vision: false },
  { id: 'mpv', name: '??', model: 'v', auto_tier: 'standard', supports_vision: true },
] as ModelProfile[]
function render(value: string, onChange = () => {}) {
  act(() => { createRoot(host).render(<ModelChip profiles={profiles} value={value} onChange={onChange} />) })
}

describe('ModelChip', () => {
  it('shows ???? for auto and opens a menu of tier/vision tagged options', () => {
    render(AUTO_MODEL_ID)
    expect(host.textContent).toContain('????')
    act(() => { (host.querySelector('[data-testid="model-chip"]') as HTMLElement).click() })
    const items = host.querySelectorAll('[role="menuitem"]')
    expect(items.length).toBe(3)
    expect(host.textContent).toContain('??')
    expect(host.textContent).toContain('???')
  })
  it('shows the chosen short name and emits onChange', () => {
    const onChange = (id: string) => { expect(id).toBe('mp1') }
    render('mp1', onChange)
    expect(host.textContent).toContain('??')
    act(() => { (host.querySelector('[data-testid="model-chip"]') as HTMLElement).click() })
    act(() => { (host.querySelectorAll('[role="menuitem"]')[1] as HTMLElement).click() })
  })
})
```

- [ ] **?? 7.4??? `ModelChip.tsx`??? DropdownMenu?**

```tsx
import { useMemo } from 'react'
import { Cpu } from 'lucide-react'
import type { ModelProfile } from '../api'
import { AUTO_MODEL_ID } from '../modelSelect'
import { AUTO_LABEL, tierLabel, VISION_LABEL } from '../strings'
import { DropdownMenu, type MenuItem } from './ui'

export interface ModelChipProps {
  profiles: ModelProfile[]
  value: string
  onChange: (id: string) => void
  disabled?: boolean
}

export function ModelChip({ profiles, value, onChange, disabled }: ModelChipProps) {
  const selected = value.trim() === '' ? AUTO_MODEL_ID : value
  const chosen = profiles.find((p) => p.id === selected)
  const label = chosen ? chosen.name : AUTO_LABEL

  const items: MenuItem[] = useMemo(() => {
    const autoItem: MenuItem = { id: AUTO_MODEL_ID, label: AUTO_LABEL, onSelect: () => onChange(AUTO_MODEL_ID) }
    const rest: MenuItem[] = profiles.map((p) => ({
      id: p.id,
      label: `${p.name} ? ${tierLabel(p.auto_tier)}${p.supports_vision ? ` ? ${VISION_LABEL}` : ''}`,
      onSelect: () => onChange(p.id),
    }))
    return [autoItem, ...rest]
  }, [profiles, onChange])

  return (
    <DropdownMenu
      triggerLabel="????"
      triggerClassName="model-chip"
      items={items}
      disabled={disabled}
    >
      <span data-testid="model-chip" className="model-chip-inner">
        <Cpu size={14} aria-hidden /> {label}
      </span>
    </DropdownMenu>
  )
}
```

??`DropdownMenu` ? trigger ?? children?? trigger ? `disabled` ????? `DropdownMenu.tsx` ? trigger `<button>` ????? `disabled`??? props?`disabled?: boolean` ??????????????????????????

- [ ] **?? 7.5?? `Composer.tsx` ??????**

`ComposerProps` ?? `toolbar?: React.ReactNode`?? `.composer-box` ??????? textarea ???? box ????????? `{toolbar}`??? box ????

```tsx
export interface ComposerProps {
  disabled?: boolean
  onSend: (text: string, files: File[]) => void
  draft?: string
  skills?: SkillSummary[]
  toolbar?: React.ReactNode
}
```

? `<div className="composer-box">` ???? `<button className="composer-attach">` ?????

```tsx
{toolbar && <span className="composer-toolbar">{toolbar}</span>}
```

- [ ] **?? 7.6?????????**

`style.css` ????????

```css
.composer-toolbar { display: inline-flex; align-items: center; margin-right: var(--space-1); }
.model-chip, .model-chip-inner { display: inline-flex; align-items: center; gap: 4px; }
.model-chip-inner { font-size: 13px; color: var(--text-muted); }
.model-chip { border: 1px solid var(--border); border-radius: var(--radius-pill); padding: 2px 10px; background: var(--surface); min-height: 32px; }
.model-chip:hover { background: var(--surface-2); color: var(--text); }
```

???`npx vitest run src/modelChoice.test.ts src/components/ModelChip.test.tsx`?`npx tsc --noEmit`?

```bash
git add web/chat/src/modelChoice.ts web/chat/src/modelChoice.test.ts web/chat/src/components/ModelChip.tsx web/chat/src/components/ModelChip.test.tsx web/chat/src/components/Composer.tsx web/chat/src/components/ui/DropdownMenu.tsx web/chat/src/style.css
git commit -m "feat(web): P1 ???? + ?? localStorage ????????/??/?????????"
```

---

## ?? 8?ChatPage ?????????????????????????????

**???**
- ???`web/chat/src/pages/ChatPage.tsx`
- ???`web/chat/src/style.css`????????????
- ??????????????? `ChatPage*` ?? + ???????????????????????????/`data-testid`?

- [ ] **?? 8.1?????????**

? `ChatPage.tsx` ?? import ???

```tsx
import { MoreHorizontal, GitBranch, CornerUpLeft } from 'lucide-react'
import { listTools, type ToolInfo } from '../api'
import { useToast, ToastRegion, ConfirmDialog, Modal, DropdownMenu, type MenuItem, Button } from '../components/ui'
import { ModelChip } from '../components/ModelChip'
import { ToolCard } from '../components/ToolCard'
import { WorkflowCard } from '../components/WorkflowCard'
import type { ToolCatalog } from '../friendlyTool'
import { foldToolBlocks } from '../historyBlocks'
import { loadModelChoice, saveModelChoice, resolveModelChoice } from '../modelChoice'
import { AUTO_MODEL_ID, buildRunOptions, visionGate } from '../modelSelect'
import { ACTIONS, ADVANCED, WELCOME, friendlyError } from '../strings'
```

???`ApiError`?`foldEvents`?`type ChatBlock`?`ToolCard`/`WorkflowCard` ????????????????????????? import ??`buildRunOptions, visionGate` ??????? `AUTO_MODEL_ID`??????????????????

???????????? `selectedModelId` ???

```tsx
const toast = useToast()
const [toolCatalog, setToolCatalog] = useState<ToolCatalog>([])
const [historyBlocks, setHistoryBlocks] = useState<Record<string, ChatBlock[]>>({})
const [confirmDelete, setConfirmDelete] = useState<string | null>(null)
const [visionWarning, setVisionWarning] = useState<string | null>(null)
```

- [ ] **?? 8.2???????????? + ???????????**

? `const [selectedModelId, setSelectedModelId] = useState('')` ?? `useState(loadModelChoice)`?

?????? `modelProfiles` ? effect ???? profiles ?????

```tsx
const resolved = resolveModelChoice(profiles)
if (resolved.stale) {
  setSelectedModelId(AUTO_MODEL_ID)
  saveModelChoice(AUTO_MODEL_ID)
  toast.push({ tone: 'info', title: '????????????????????' })
}
```

???? effect ???????????????????????

```tsx
useEffect(() => {
  let cancelled = false
  void listTools().then((tools: ToolInfo[]) => {
    if (!cancelled) setToolCatalog(tools.map((t) => ({ name: t.name, title: t.title, description: t.description })))
  }).catch(() => { /* ????????? */ })
  return () => { cancelled = true }
}, [])
```

- [ ] **?? 8.3?????????????????**

?? handler ????????? `setSelectedModelId`?

```tsx
const onChooseModel = (id: string) => {
  setSelectedModelId(id)
  saveModelChoice(id)
}
```

?? `onSend` ?? `setSelectedModelId('')`??? 545 ???

- [ ] **?? 8.4??? run ??????/???**

???? `run_id` ? `listEvents` ????? 394-408 ?????? `events` ????????????

```tsx
const toolBlocks = foldToolBlocks(runId, events)
if (toolBlocks.length > 0) {
  setHistoryBlocks((prev) => (prev[runId] ? prev : { ...prev, [runId]: toolBlocks }))
}
```

`onNewChat` ???????????? `setHistoryBlocks({})`?

- [ ] **?? 8.5?????? Toast?friendlyError?**

???

```tsx
const reportError = (e: unknown) => {
  const f = friendlyError(e)
  toast.push({ tone: 'error', title: f.title, detail: f.detail })
}
```

???/HITL/??? catch ? `setError(????)` ???????? `reportError(err)`?`conversation_busy`?`vision_unsupported` ??? `friendlyError` ???????????????????????????????`status`???????? `error`?

- [ ] **?? 8.6????????? Modal**

????????? 495-505 ????? `built`?`supportsVision`?? `setError(...)` ?????

```tsx
if (!gate.allowed) {
  setBusy(false)
  setVisionWarning(gate.message ?? '???????????')
  return
}
```

?? `try` ???????? `catch`??? 509-512 ???? `reportError(err); setBusy(false); return`?

??? `catch` ???? `vision_unsupported` ???? 555-559 ???????? `reportError(err)`?`friendlyError` ??? `vision_unsupported`??

? JSX ??? `<ToastRegion>` ????

```tsx
<Modal open={visionWarning !== null} title="????????" onClose={() => setVisionWarning(null)}
  footer={<Button variant="primary" onClick={() => setVisionWarning(null)}>???</Button>}>
  <p>{visionWarning}</p>
</Modal>
```

- [ ] **?? 8.7??????? ConfirmDialog**

`onDeleteConversation` ?? `window.confirm`??????????

```tsx
const onDeleteConversation = (id: string) => setConfirmDelete(id)
const performDelete = async () => {
  const id = confirmDelete
  if (!id) return
  setConfirmDelete(null)
  try {
    await deleteConversation(id)
    toast.push({ tone: 'success', title: '?????' })
  } catch (e) {
    reportError(e)
    return
  }
  if (id === conversationId) { /* ????????? setHistoryBlocks({}) */ }
  await refreshConversations()
}
```

?????

```tsx
<ConfirmDialog
  open={confirmDelete !== null}
  danger
  title="???????"
  body="???????????????????????"
  confirmText="??"
  onConfirm={() => void performDelete()}
  onCancel={() => setConfirmDelete(null)}
/>
```

- [ ] **?? 8.8??????????????????**

????? `.msg-actions`?? 824-845 ??????????
- assistant?`??`??? `m.content`??`????`?`onRegenerate`??
- user?`???????`?`onRollbackUser`??`??`??? `m.content`??

???????????????HTTP ??? IP ? `navigator.clipboard`?? helper?? `ChatPage.tsx` ??? `strings` ???????

```tsx
async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) { await navigator.clipboard.writeText(text); return true }
  } catch { /* fall through to legacy path */ }
  try {
    const ta = document.createElement('textarea')
    ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0'
    document.body.appendChild(ta); ta.select()
    const ok = document.execCommand('copy'); ta.remove(); return ok
  } catch { return false }
}
const onCopy = (m: ChatMessage) => {
  void copyText(m.content).then((ok) =>
    toast.push(ok ? { tone: 'success', title: '???' } : { tone: 'error', title: '????????????' }),
  )
}
```

???? `DropdownMenu`?

```tsx
const moreItems: MenuItem[] = []
if (m.role === 'user' || m.role === 'assistant') {
  moreItems.push({ id: 'fork', label: ACTIONS.forkAsNew, icon: <GitBranch size={15} aria-hidden />, onSelect: () => void onFork(m) })
}
if (m.role === 'system_note') {
  moreItems.unshift({ id: 'rb', label: ACTIONS.rollbackHere, icon: <CornerUpLeft size={15} aria-hidden />, onSelect: () => void onRollbackTo(m) })
}
```

???`{moreItems.length > 0 && <DropdownMenu triggerLabel={ACTIONS.more} items={moreItems} />}`??????? `Fork / ????` ?????????????????? `ACTIONS.*`??

- [ ] **?? 8.9??????????/???????**

????? map ???? `run_id` ??????????????? run ???

```tsx
{m.role === 'assistant' && m.run_id && historyBlocks[m.run_id] && isFirstMessageOfRun(m, messages) && (
  <div className="msg-history-blocks">
    {historyBlocks[m.run_id].map((b, i) =>
      b.kind === 'tool' ? (
        <ToolCard key={`h-${i}`} block={b} catalog={toolCatalog} readOnly />
      ) : (
        <WorkflowCard key={`h-${i}`} block={b} />
      ),
    )}
  </div>
)}
```

?? `isFirstMessageOfRun` ?????????? run ???????

```tsx
function isFirstMessageOfRun(index: number, msgs: ChatMessage[]): boolean {
  const runId = msgs[index].run_id
  if (!runId) return false
  return msgs.findIndex((mm) => mm.run_id === runId) === index
}
```

?????? map ????`isFirstMessageOfRun(msgIndex, messages)`?? `messages.map((m, msgIndex) => ...)` ?????????`liveBlocks`?? `<ToolCard>` ?? `catalog={toolCatalog}`??? readOnly??

- [ ] **?? 8.10??????????**

??????

```tsx
<div className="welcome">
  <p className="welcome-title">{WELCOME.title}</p>
  <p className="welcome-sub">{WELCOME.subtitle}</p>
</div>
```

??????`<summary>` ??? `ADVANCED.summary`?Token ??? `ADVANCED.tokenLabel` ??????? `ADVANCED.tokenHint`?Webhook ??? `ADVANCED.webhookLabel`??? placeholder ?? `Bearer eyJ?` ????????????

- [ ] **?? 8.11?? ModelChip ????? ModelSelect**

?? footer ???? `<ModelSelect .../>`?? 938-945 ????????? `Composer` ? `toolbar` ???

```tsx
<Composer
  disabled={composerDisabled}
  draft={composerDraft}
  skills={skills}
  onSend={(t, f) => void onSend(t, f)}
  toolbar={
    modelProfiles.length > 0 ? (
      <ModelChip profiles={modelProfiles} value={selectedModelId} onChange={onChooseModel} disabled={composerDisabled} />
    ) : (
      <Link className="model-chip model-chip-empty" to="/settings/models">????</Link>
    )
  }
/>
```

? `react-router-dom` ? `Link` ??? `ChatPage.tsx` ???? `import { Link } from 'react-router-dom'`?

- [ ] **?? 8.12?? ToastRegion ???**

????????? `<div>` ???? `<ToastRegion toasts={toast.toasts} onDismiss={toast.dismiss} />`??? Modal ????

???`npx tsc --noEmit`?`npx vitest run`??????/?????????????? `data-testid`/??/????????????

```bash
git add web/chat/src/pages/ChatPage.tsx web/chat/src/style.css web/chat/src/components/Composer.tsx
git commit -m "feat(web): P1 ??????Toast/???/???????????????????????????????????"
```

---

## ?? 9????? + ?????? + ????

**???** ?? `internal/ui/dist/**`?Go `//go:embed` ????

- [ ] **?? 9.1????????**

? `web/chat` ???

```bash
npx tsc --noEmit
npx vitest run
npm run build
```

???tsc ???????????build ???????? `internal/ui/dist`?? `index-*.js/css` ????

- [ ] **?? 9.2???????**

??????`go build ./... && go test ./internal/api/... ./internal/channel/...`
????????? Go ??????????????

- [ ] **?? 9.3?gofmt ??????? Go ????**

? `internal/ui/dist` ?? Go ??????????? `gofmt -l .` ?????

- [ ] **?? 9.4?????????/? ? ??/?????**

- ???????????????????? + ???????/JSON??HITL???????????workflow?? n/m ???????????????????????????
- ?????????????/?????????????????? run ???????? HITL ??????
- ????????????????? Toast????? Toast?????????????????? Toast + ??????
- ???????????????????????????????????????????????
- ???????????????/???????/Esc/??????
- ???????????+????????/Token ?????????????????
- ???????Tab ??????? Esc????????/????????

- [ ] **?? 9.5???????**

```bash
git add internal/ui/dist
git commit -m "build(web): ???????P1 ?????"
```

---


## ????????????

| ?????? | ??????? |
|---|---|
| 4.1 ?????????? parseJSON ?? ApiError?????? HTTP ???? | ???? 1??1.5?? |
| 4.1 ????????? friendlyError + ?????? + ??????? | ???? 1??1.1/1.3?? |
| 4.1 ??? ????/???/??????????????????????? ModelSettings ???? | ???? 1??1.6/1.7/1.8?? |
| 4.2 Toast/useToast/aria-live | ???? 2 |
| 4.2 Modal ???? | ???? 3??3.1-3.4?? |
| 4.2 ConfirmDialog??????????????? | ???? 3??3.5-3.7?? |
| 4.2 DropdownMenu??????/Esc/?????/Enter?? | ???? 4 |
| 4.3 friendlyToolName/toolPhrase ???? | ???? 5 |
| 4.3 foldToolBlocks ?????????/????????? | ???? 5 |
| 4.3 ToolCard ??????+????+HITL ???????????????????+?????? | ???? 6 |
| 4.3 WorkflowCard ?? n/m ??????????????? id | ???? 6 |
| 4.4 ModelChip?????/??????????????? | ???? 7 + 8.11 |
| 4.4 ?????? localStorage ????/?????? | ???? 7 + 8.2/8.3 |
| 4.4 ??????????? + ?????????????????????? | 8.8 |
| 4.4 ?????? ConfirmDialog + Toast | 8.7 |
| 4.4 ???????????????????? | 8.10 |
| 4.4 ?????????? Modal ???????????????? | 8.6 |
| ???? Toast ??????????????? | 8.5 |
| ????????????? run ????????? | 8.4/8.9 |
| ?? 5 ?? ????? | ??? + ??????????? |
| ?? 6 ?? ????/???? | ?????? TDD + ???? 9 |
| ?????? dist / ?????? | ???? 9 |
