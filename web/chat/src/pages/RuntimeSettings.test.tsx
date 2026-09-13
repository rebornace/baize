// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import type { RuntimeKnobs } from '../api'
import { GateContext } from '../gateContext'
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
        GateContext.Provider,
        { value: { role: 'admin', gateEnabled: true, operatorId: 'admin' } },
        createElement(RuntimeSettings),
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

    const btn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('重置为基线口令'),
    )
    expect(btn).toBeTruthy()
    await act(async () => { btn!.click() })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(RUNTIME.confirmResetTitle)
    expect(patch).not.toHaveBeenCalled()

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(patch).toHaveBeenCalledWith({ reset: true })
    root.unmount()
    host.remove()
  })
})
