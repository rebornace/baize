// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { MetricsSettings } from './MetricsSettings'
import * as metricsApi from '../api/metrics'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'

let host: HTMLDivElement
let root: Root

beforeEach(() => {
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
      <MemoryRouter>
        <MetricsSettings />
      </MemoryRouter>,
    )
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('MetricsSettings', () => {
  it('loads and shows Prometheus text', async () => {
    vi.spyOn(metricsApi, 'getMetricsText').mockResolvedValue(
      '# HELP baize_runs_total\nbaize_runs_total{status="ok"} 1\n',
    )
    vi.spyOn(metricsApi, 'getRecentLogs').mockResolvedValue({ lines: [], limit: 200 })
    await renderPage()
    const body = host.querySelector('[data-testid="metrics-body"]')
    expect(body?.textContent).toContain('baize_runs_total')
  })

  it('surfaces load errors', async () => {
    const { ApiError } = await import('../api/http')
    vi.spyOn(metricsApi, 'getMetricsText').mockRejectedValue(
      new ApiError(403, 'forbidden', 'HTTP 403: forbidden'),
    )
    await renderPage()
    expect(host.textContent).toMatch(/没有访问权限|登录已失效|unauthorized/i)
  })

  it('switches to logs tab and loads recent lines', async () => {
    vi.spyOn(metricsApi, 'getMetricsText').mockResolvedValue('m=1\n')
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
