// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as metricsApi from '../api/metrics'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { BudgetAlertHistoryCard } from './BudgetAlertHistoryCard'

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

async function render(items: metricsApi.BudgetAlertEntry[]) {
  vi.spyOn(metricsApi, 'getBudgetAlertHistory').mockResolvedValue({ items, limit: 50 })
  await act(async () => {
    createRoot(host).render(<BudgetAlertHistoryCard />)
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('BudgetAlertHistoryCard', () => {
  it('renders scoped and global alerts newest first', async () => {
    await render([
      {
        at: '2026-11-25T00:00:00Z',
        target: 'agent',
        target_id: 'a1',
        scope: 'day',
        period: 'day',
        level: 'exceeded',
        total_usd: 1,
        budget_usd: 2,
        ratio: 0.5,
      },
      {
        at: '2026-11-25T00:01:00Z',
        scope: 'month',
        period: 'month',
        level: 'warning',
        total_usd: 0.5,
        budget_usd: 10,
        ratio: 0.05,
      },
    ])
    const card = host.querySelector('[data-testid="budget-alert-history"]')
    expect(card).not.toBeNull()
    expect(card?.textContent).toContain('预算告警历史')
    expect(card?.textContent).toContain('agent:a1')
    expect(card?.textContent).toContain('超限')
    expect(card?.textContent).toContain('全局')
    expect(card?.textContent).toContain('预警')
  })

  it('shows an empty state', async () => {
    await render([])
    expect(host.textContent).toContain('还没有预算告警')
  })
})
