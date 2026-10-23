// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
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
  context_projection_enabled: false,
  active_goal_enabled: true,
  active_goal_max_depth: 8,
  active_goal_max_text_runes: 500,
  memory_enabled: true,
  memory_auto_extract: true,
  decide_enabled: true,
  decide_memory_enabled: true,
  decide_profile_id: '',
  decide_systemone_base_url: '',
  decide_systemone_api_key_set: false,
  decide_systemone_model: '',
  decide_tool_routing_enabled: true,
  decide_tool_threshold: 12,
  decide_tool_pre_topk: 16,
  decide_tool_choice_enabled: false,
  decide_tool_prune_enabled: true,
  decide_tool_prune_threshold: 500,
  decide_tool_prune_max_judged: 8,
  decide_route_enabled: false,
  decide_route_min_runes: 400,
  scoped_subrun_enabled: false,
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
    log_format: 'text',
    otel_otlp_endpoint: '',
    otel_service_name: 'baize',
    otel_enabled: false,
  })
  vi.spyOn(api, 'getCredentials').mockResolvedValue({
    source: 'override',
    operator_set: true,
    admin_set: true,
    operators: [],
  })
  vi.spyOn(api, 'listModelProfiles').mockResolvedValue([])
  await act(async () => {
    root.render(
      createElement(
        MemoryRouter,
        null,
        createElement(
          LocaleProvider,
          null,
          createElement(
            GateContext.Provider,
            { value: { role: 'admin', gateEnabled: true, operatorId: 'admin' } },
            createElement(RuntimeSettings),
          ),
        ),
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

async function selectTab(
  host: HTMLElement,
  id: 'basics' | 'speed' | 'memory' | 'security' | 'observability',
) {
  const btn = host.querySelector(`[data-testid="runtime-tab-${id}"]`) as HTMLButtonElement | null
  expect(btn).toBeTruthy()
  await act(async () => {
    btn!.click()
    await new Promise((r) => setTimeout(r, 0))
  })
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
    await selectTab(host, 'security')

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

    await selectTab(host, 'memory')
    expect(host.textContent).toContain(RUNTIME.sectionCompact)
    expect(host.textContent).toContain(RUNTIME.contextProjectionEnabled)
    expect(host.textContent).toContain(RUNTIME.sectionMemory)
    expect(host.textContent).toContain(RUNTIME.memoryEnabled)
    expect(host.textContent).toContain(RUNTIME.memoryAutoExtract)
    // 高级区内字段默认不可见：details 未 open
    const details = host.querySelector('details')
    expect(details).toBeTruthy()
    expect(details!.open).toBe(false)
    const advInputs = details!.querySelectorAll('input')
    expect(advInputs.length).toBeGreaterThanOrEqual(4)

    await selectTab(host, 'speed')
    expect(host.textContent).toContain(RUNTIME.sectionDecide)
    expect(host.textContent).toContain(RUNTIME.decideEnabled)
    expect(host.textContent).toContain(RUNTIME.decideToolRoutingEnabled)
    expect(host.textContent).toContain(RUNTIME.decideToolPruneEnabled)
    expect(host.textContent).toContain(RUNTIME.decideRouteEnabled)

    await selectTab(host, 'security')
    expect(host.textContent).toContain(RUNTIME.sectionCreds)

    await selectTab(host, 'basics')
    expect(host.textContent).toContain(RUNTIME.fieldMaxMessages)
    root.unmount()
    host.remove()
  })

  it('credentials section uses humanized labels and empty named-ops copy', async () => {
    localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
    const { host, root } = await renderRuntime()
    await selectTab(host, 'security')
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

  it('observability tab saves log_format and otel fields via PATCH', async () => {
    localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
    const patch = vi.spyOn(api, 'patchRuntimeSettings').mockResolvedValue({
      effective: baseKnobs,
      overridden,
      public_base_url: '',
      public_base_url_overridden: false,
      log_format: 'json',
      otel_otlp_endpoint: 'http://127.0.0.1:4318',
      otel_service_name: 'baize-hot',
      otel_enabled: true,
      log_format_overridden: true,
      otel_otlp_endpoint_overridden: true,
      otel_service_name_overridden: true,
    })
    const { host, root } = await renderRuntime()
    await selectTab(host, 'observability')
    expect(host.textContent).toContain(RUNTIME.sectionObservability)
    expect(host.textContent).toContain(RUNTIME.obsHotNote)
    const fmt = host.querySelector('[data-testid="runtime-obs-log-format"]') as HTMLSelectElement
    const ep = host.querySelector('[data-testid="runtime-obs-otel-endpoint"]') as HTMLInputElement
    const svc = host.querySelector('[data-testid="runtime-obs-otel-service"]') as HTMLInputElement
    const setSelect = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value')!.set!
    const setInput = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
    await act(async () => {
      setSelect.call(fmt, 'json')
      fmt.dispatchEvent(new Event('change', { bubbles: true }))
      setInput.call(ep, 'http://127.0.0.1:4318')
      ep.dispatchEvent(new Event('input', { bubbles: true }))
      setInput.call(svc, 'baize-hot')
      svc.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => {
      ;(host.querySelector('[data-testid="runtime-obs-save"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(patch).toHaveBeenCalledWith({
      log_format: 'json',
      otel_otlp_endpoint: 'http://127.0.0.1:4318',
      otel_service_name: 'baize-hot',
    })
    expect(host.textContent).toContain(RUNTIME.obsToastSaved)
    root.unmount()
    host.remove()
  })
})
