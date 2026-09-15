// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import type { RuntimeKnobs } from '../api'
import { GateContext } from '../gateContext'
import { LocaleProvider } from '../locale/LocaleContext'
import { LOCALE_STORAGE_KEY } from '../locale/types'
import { RUNTIME } from '../strings'
import { RuntimeSettings } from './RuntimeSettings'

const baseKnobs: RuntimeKnobs = {
  max_messages: 40,
  max_steps: 16,
  tool_timeout_seconds: 60,
  compaction_enabled: true,
  compact_threshold: 0.8,
  compact_reserve_tokens: 8000,
  compact_keep_recent: 8,
  compact_summary_timeout_seconds: 60,
  memory_enabled: true,
  memory_auto_extract: true,
}

const overridden = Object.fromEntries(
  Object.keys(baseKnobs).map((k) => [k, false]),
) as api.RuntimeKnobsOverrides

async function renderRuntime() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  vi.spyOn(api, 'getRuntimeSettings').mockResolvedValue({
    effective: baseKnobs,
    overridden,
    public_base_url: '',
    public_base_url_overridden: false,
  })
  vi.spyOn(api, 'getCredentials').mockResolvedValue({
    source: 'override',
    operator_set: true,
    admin_set: true,
    operators: [],
  })
  await act(async () => {
    root.render(
      createElement(
        LocaleProvider,
        null,
        createElement(
          GateContext.Provider,
          { value: { role: 'admin', gateEnabled: true, operatorId: 'admin' } },
          createElement(RuntimeSettings),
        ),
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

describe('RuntimeSettings reset credentials ConfirmDialog', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('opens ConfirmDialog instead of window.confirm; confirms then patches reset', async () => {
    const patch = vi.spyOn(api, 'patchCredentials').mockResolvedValue({
      source: 'config',
      operator_set: true,
      admin_set: true,
      operators: [],
    })
    const confirmSpy = vi.spyOn(window, 'confirm')
    const { host, root } = await renderRuntime()

    expect(host.textContent).toContain(RUNTIME.credsSourceOverride)

    const btn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(RUNTIME.resetButton),
    )
    expect(btn).toBeTruthy()
    await act(async () => { btn!.click() })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(RUNTIME.confirmResetTitle)
    expect(host.textContent).toContain(RUNTIME.confirmResetOk)
    expect(patch).not.toHaveBeenCalled()

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(patch).toHaveBeenCalledWith({ reset: true })
    expect(host.textContent).toContain(RUNTIME.toastReset)
    root.unmount()
    host.remove()
  })
})

describe('RuntimeSettings humanize shell', () => {
  afterEach(() => { vi.restoreAllMocks() })

  it('shows PageHeader title and section headings; compact adv collapsed', async () => {
    localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
    const { host, root } = await renderRuntime()
    expect(host.textContent).toContain(RUNTIME.title)
    expect(host.textContent).not.toContain('运行时设置')
    expect(host.textContent).toContain(RUNTIME.sectionPublicBase)
    expect(host.textContent).toContain(RUNTIME.sectionBehavior)
    expect(host.textContent).toContain(RUNTIME.sectionCompact)
    expect(host.textContent).toContain(RUNTIME.sectionMemory)
    expect(host.textContent).toContain(RUNTIME.memoryEnabled)
    expect(host.textContent).toContain(RUNTIME.memoryAutoExtract)
    expect(host.textContent).toContain(RUNTIME.sectionCreds)
    // 高级区内字段默认不可见：details 未 open，或不在 DOM 可见区
    const details = host.querySelector('details')
    expect(details).toBeTruthy()
    expect(details!.open).toBe(false)
    expect(host.textContent).toContain(RUNTIME.fieldMaxMessages)
    // 折叠未开时，高级 label 仍可能在 summary 旁；字段 input 应在 details 内
    const advInputs = details!.querySelectorAll('input')
    expect(advInputs.length).toBeGreaterThanOrEqual(4)
    root.unmount()
    host.remove()
  })

  it('credentials section uses humanized labels and empty named-ops copy', async () => {
    localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
    const { host, root } = await renderRuntime()
    expect(host.textContent).toContain(RUNTIME.fieldOperatorToken)
    expect(host.textContent).toContain(RUNTIME.fieldAdminToken)
    expect(host.textContent).toContain(RUNTIME.namedOpsEmpty)
    expect(host.textContent).toContain(RUNTIME.namedOpsTitle)
    expect(host.textContent).toContain(RUNTIME.rotateSubmit)
    expect(host.textContent).toContain(RUNTIME.addOperator)
    root.unmount()
    host.remove()
  })

  it('locale choice persists baize.locale when switching to English', async () => {
    localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
    const { host, root } = await renderRuntime()
    const enBtn = host.querySelector('[data-testid="locale-en"]') as HTMLButtonElement | null
    expect(enBtn).toBeTruthy()
    await act(async () => {
      enBtn!.click()
    })
    expect(localStorage.getItem(LOCALE_STORAGE_KEY)).toBe('en')
    expect(document.documentElement.lang).toBe('en')
    root.unmount()
    host.remove()
  })
})
