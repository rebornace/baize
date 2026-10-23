// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { MetricsSettings } from './MetricsSettings'
import * as metricsApi from '../api/metrics'
import { LocaleProvider } from '../locale/LocaleContext'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  localStorage.setItem('baize.locale', 'zh-CN')
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})

afterEach(() => {
  act(() => {
    root.unmount()
  })
  host.remove()
  vi.restoreAllMocks()
})

async function renderPage() {
  await act(async () => {
    root.render(
      <LocaleProvider>
        <MemoryRouter>
          <MetricsSettings />
        </MemoryRouter>
      </LocaleProvider>,
    )
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('MetricsSettings', () => {
  it('loads and shows human overview instead of raw dump', async () => {
    vi.spyOn(metricsApi, 'getMetricsText').mockResolvedValue(
      [
        'baize_runs_total{status="succeeded"} 3',
        'baize_runs_total{status="failed"} 1',
        'baize_run_duration_seconds_sum 8',
        'baize_run_duration_seconds_count 4',
        'baize_tool_invocations_total{connector_id="x",result="ok"} 5',
        'baize_tool_invocations_total{connector_id="x",result="error"} 1',
      ].join('\n'),
    )
    vi.spyOn(metricsApi, 'getRecentLogs').mockResolvedValue({ lines: [], limit: 200 })
    await renderPage()
    const body = host.querySelector('[data-testid="metrics-body"]')
    expect(body?.textContent).toContain('成功')
    expect(body?.textContent).toContain('3')
    expect(body?.textContent).toContain('失败')
    expect(host.querySelector('[data-testid="metrics-raw-body"]')?.textContent).toContain(
      'baize_runs_total',
    )
  })

  it('surfaces load errors', async () => {
    const { ApiError } = await import('../api/http')
    vi.spyOn(metricsApi, 'getMetricsText').mockRejectedValue(
      new ApiError(403, 'forbidden', 'HTTP 403: forbidden'),
    )
    await renderPage()
    expect(host.textContent).toMatch(/没有访问权限|登录已失效|No access|unauthorized|session expired/i)
  })

  it('switches to logs tab and loads recent lines', async () => {
    vi.spyOn(metricsApi, 'getMetricsText').mockResolvedValue('baize_runs_total{status="ok"} 1\n')
    const logsSpy = vi.spyOn(metricsApi, 'getRecentLogs').mockResolvedValue({
      lines: ['time=1 msg=hello', 'time=2 msg=world'],
      limit: 200,
    })
    await renderPage()
    const logsTab = host.querySelector('[data-testid="metrics-tab-logs"]') as HTMLButtonElement
    await act(async () => {
      logsTab.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(logsSpy).toHaveBeenCalled()
    const body = host.querySelector('[data-testid="logs-body"]')
    expect(body?.textContent).toContain('msg=hello')
    expect(body?.textContent).toContain('msg=world')
  })
})
