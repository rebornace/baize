// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as metricsApi from '../api/metrics'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { RunOverviewCard } from './RunOverviewCard'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  vi.restoreAllMocks()
  setPack(zhPack)
})

async function renderCard(overview: metricsApi.RunOverview) {
  vi.spyOn(metricsApi, 'getRunOverview').mockResolvedValue(overview)
  await act(async () => {
    createRoot(host).render(<RunOverviewCard />)
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('RunOverviewCard', () => {
  it('renders success rate, totals, waiting and cost', async () => {
    await renderCard({
      days: 7,
      total: 4,
      by_status: { succeeded: 3, failed: 1 },
      terminal: 4,
      success_rate: 0.75,
      active: 0,
      waiting_human: 1,
      range_cost_usd: 0.42,
      today_cost_usd: 0.1,
      top_errors: [{ message: 'boom', count: 2 }],
      truncated: false,
    })
    const card = host.querySelector('[data-testid="run-overview"]')
    expect(card).not.toBeNull()
    expect(card?.textContent).toContain('75%')
    expect(card?.textContent).toContain('最近 7 天')
    expect(card?.textContent).toContain('等待审批')
    expect(card?.textContent).toContain('0.4200')
    expect(card?.textContent).toContain('boom')
  })

  it('shows an empty state when there are no runs', async () => {
    await renderCard({
      days: 7,
      total: 0,
      by_status: {},
      terminal: 0,
      success_rate: 0,
      active: 0,
      waiting_human: 0,
      range_cost_usd: 0,
      today_cost_usd: 0,
      top_errors: [],
      truncated: false,
    })
    expect(host.textContent).toContain('这段时间还没有运行')
  })
})
