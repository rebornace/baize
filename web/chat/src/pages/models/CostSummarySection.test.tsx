// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as modelsApi from '../../api/settings/models'
import { setPack } from '../../locale/pack'
import { zhPack } from '../../locales/zh'
import { CostSummarySection } from './CostSummarySection'

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

const summary: modelsApi.CostSummary = {
  currency: 'USD',
  days: [{ date: '2026-11-24', total_usd: 1, runs: 2 }],
  today_total_usd: 1,
  month_total_usd: 12,
  budget_usd_per_day: 1,
  budget_usd_per_month: 10,
  warning_ratio: 0.8,
  exceeded_ratio: 1,
  budget_enforce: false,
  over_budget: false,
  month_over_budget: true,
}

async function renderSection(agents: modelsApi.AgentCostBudget[] = []) {
  vi.spyOn(modelsApi, 'getCostSummary').mockResolvedValue(summary)
  vi.spyOn(modelsApi, 'getAgentCostBudgets').mockResolvedValue({ items: agents })
  const save = vi.spyOn(modelsApi, 'putCostBudget').mockResolvedValue()
  const saveAgent = vi.spyOn(modelsApi, 'putAgentCostBudget').mockResolvedValue({
    agent_id: 'a1',
    budget_usd_per_day: 1,
    budget_usd_per_month: 20,
  })
  await act(async () => {
    createRoot(host).render(<CostSummarySection readOnly={false} />)
    await Promise.resolve()
    await Promise.resolve()
  })
  return { save, saveAgent }
}

describe('CostSummarySection monthly budget', () => {
  it('shows the month total and over-budget state', async () => {
    await renderSection()
    expect(host.textContent).toContain('本月 USD 12.000000')
    expect(host.textContent).toContain('本月费用已超过预算')
  })

  it('saves the monthly budget alongside the daily budget', async () => {
    const { save } = await renderSection()
    const inputs = host.querySelectorAll<HTMLInputElement>('input[type="number"]')
    expect(inputs).toHaveLength(4)
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(inputs[0], '2')
      inputs[0].dispatchEvent(new Event('input', { bubbles: true }))
      setter?.call(inputs[1], '30')
      inputs[1].dispatchEvent(new Event('input', { bubbles: true }))
    })
    const button = [...host.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === '保存预算',
    ) as HTMLButtonElement
    await act(async () => {
      button.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(save).toHaveBeenCalledWith(2, 30, false, 0.8, 1)
  })

  it('saves per-agent budgets', async () => {
    const { saveAgent } = await renderSection([
      { agent_id: 'a1', budget_usd_per_day: 0, budget_usd_per_month: 0 },
    ])
    const row = host.querySelector('.agent-budget-row') as HTMLDivElement
    expect(row).not.toBeNull()
    const inputs = row.querySelectorAll<HTMLInputElement>('input[type="number"]')
    expect(inputs).toHaveLength(2)
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(inputs[0], '1')
      inputs[0].dispatchEvent(new Event('input', { bubbles: true }))
      setter?.call(inputs[1], '20')
      inputs[1].dispatchEvent(new Event('input', { bubbles: true }))
    })
    const saveBtn = [...row.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === '保存',
    ) as HTMLButtonElement
    await act(async () => {
      saveBtn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(saveAgent).toHaveBeenCalledWith({
      agent_id: 'a1',
      budget_usd_per_day: 1,
      budget_usd_per_month: 20,
    })
  })
})
