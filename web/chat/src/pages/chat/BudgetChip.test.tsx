// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as modelsApi from '../../api/settings/models'
import { setPack } from '../../locale/pack'
import { zhPack } from '../../locales/zh'
import { BudgetChip } from './BudgetChip'

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

const baseStatus: modelsApi.CostStatus = {
  currency: 'USD',
  today_total_usd: 0.9,
  month_total_usd: 10,
  budget_usd_per_day: 1,
  budget_usd_per_month: 20,
  day_ratio: 0.9,
  month_ratio: 0.5,
  day_alert: 'warning',
  month_alert: '',
  alert_level: 'warning',
  alert_scope: 'day',
}

async function renderChip(status: modelsApi.CostStatus | Error, enabled = true) {
  const spy = vi.spyOn(modelsApi, 'getCostStatus')
  if (status instanceof Error) spy.mockRejectedValue(status)
  else spy.mockResolvedValue(status)
  await act(async () => {
    createRoot(host).render(
      <MemoryRouter>
        <BudgetChip enabled={enabled} />
      </MemoryRouter>,
    )
    await Promise.resolve()
    await Promise.resolve()
  })
  return spy
}

describe('BudgetChip', () => {
  it('shows the day warning at 80% or above', async () => {
    await renderChip(baseStatus)
    const chip = host.querySelector('[data-testid="budget-chip"]')
    expect(chip?.textContent).toContain('今日费用已达 90%')
    expect(chip?.className).toContain('warning')
  })

  it('shows an exceeded budget with amounts', async () => {
    await renderChip({
      ...baseStatus,
      today_total_usd: 1.2,
      day_ratio: 1.2,
      day_alert: 'exceeded',
      alert_level: 'exceeded',
    })
    const chip = host.querySelector('[data-testid="budget-chip"]')
    expect(chip?.textContent).toContain('今日费用已超预算（$1.20 / $1.00）')
    expect(chip?.className).toContain('exceeded')
  })

  it('stays hidden when no budget is configured', async () => {
    await renderChip({ ...baseStatus, day_alert: '', alert_level: '', alert_scope: '' })
    expect(host.querySelector('[data-testid="budget-chip"]')).toBeNull()
  })

  it('stays hidden for non-admin users', async () => {
    const spy = await renderChip(baseStatus, false)
    expect(spy).not.toHaveBeenCalled()
    expect(host.querySelector('[data-testid="budget-chip"]')).toBeNull()
  })

  it('hides itself when the API fails', async () => {
    await renderChip(new Error('forbidden'))
    expect(host.querySelector('[data-testid="budget-chip"]')).toBeNull()
  })
})
