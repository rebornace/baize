// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as metricsApi from '../api/metrics'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { CostAnomalyCard } from './CostAnomalyCard'

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

async function render(items: metricsApi.CostAnomalyEntry[]) {
  vi.spyOn(metricsApi, 'getCostAnomalies').mockResolvedValue({ items, limit: 50 })
  await act(async () => {
    createRoot(host).render(<CostAnomalyCard />)
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('CostAnomalyCard', () => {
  it('renders global and conversation anomalies', async () => {
    await render([
      {
        at: '2026-11-26T00:00:00Z',
        scope: 'global',
        today_usd: 5,
        baseline_usd: 1,
        ratio: 5,
      },
      {
        at: '2026-11-26T00:01:00Z',
        scope: 'conversation',
        conversation_id: 'c1',
        today_usd: 3,
        baseline_usd: 1,
        ratio: 3,
      },
    ])
    const card = host.querySelector('[data-testid="cost-anomaly-history"]')
    expect(card).not.toBeNull()
    expect(card?.textContent).toContain('费用异常')
    expect(card?.textContent).toContain('全局')
    expect(card?.textContent).toContain('会话:c1')
    expect(card?.textContent).toContain('5.0 倍')
  })

  it('shows an empty state', async () => {
    await render([])
    expect(host.textContent).toContain('还没有费用异常')
  })
})
