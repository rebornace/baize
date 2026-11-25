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
  projected_month_usd: 30,
  projection_day: 10,
  days_in_month: 30,
  projected_over_budget: true,
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
  vi.spyOn(modelsApi, 'getCostBreakdown').mockResolvedValue({
    days: 30,
    total_usd: 3,
    by_model: [{ key: 'm1', name: 'power', cost_usd: 2, ratio: 0.6667 }],
    by_agent: [{ key: 'a1', name: 'a1', cost_usd: 3, ratio: 1 }],
    by_conversation: [{ key: 'c1', name: '季度复盘', cost_usd: 3, ratio: 1 }],
    by_workspace: [{ key: 'default', name: '默认', cost_usd: 3, ratio: 1 }],
  })
  vi.spyOn(modelsApi, 'getAgentCostBudgets').mockResolvedValue({ items: agents })
  vi.spyOn(modelsApi, 'getModelCostBudgets').mockResolvedValue({
    items: [
      {
        model_profile_id: 'm1',
        name: 'power',
        model: 'gpt-4o',
        budget_usd_per_day: 0,
        budget_usd_per_month: 0,
        warning_ratio: 0,
        exceeded_ratio: 0,
        spent_usd_per_day: 0,
        spent_usd_per_month: 0,
      },
    ],
  })
  const save = vi.spyOn(modelsApi, 'putCostBudget').mockResolvedValue()
  const saveAgent = vi.spyOn(modelsApi, 'putAgentCostBudget').mockResolvedValue({
    agent_id: 'a1',
    budget_usd_per_day: 1,
    budget_usd_per_month: 20,
    warning_ratio: 0.5,
    exceeded_ratio: 1.2,
    spent_usd_per_day: 0.01,
    spent_usd_per_month: 0.2,
  })
  const saveModel = vi.spyOn(modelsApi, 'putModelCostBudget').mockResolvedValue({
    model_profile_id: 'm1',
    name: 'power',
    model: 'gpt-4o',
    budget_usd_per_day: 1,
    budget_usd_per_month: 20,
    warning_ratio: 0.5,
    exceeded_ratio: 1.2,
    spent_usd_per_day: 0.02,
    spent_usd_per_month: 0.3,
  })
  vi.spyOn(modelsApi, 'getWorkspaceCostBudgets').mockResolvedValue({
    items: [
      {
        workspace_id: 'default',
        name: '默认',
        budget_usd_per_day: 0,
        budget_usd_per_month: 0,
        warning_ratio: 0,
        exceeded_ratio: 0,
        spent_usd_per_day: 0,
        spent_usd_per_month: 0,
      },
    ],
  })
  const saveWorkspace = vi.spyOn(modelsApi, 'putWorkspaceCostBudget').mockResolvedValue({
    workspace_id: 'default',
    name: '默认',
    budget_usd_per_day: 2,
    budget_usd_per_month: 30,
    warning_ratio: 0.5,
    exceeded_ratio: 1.2,
    spent_usd_per_day: 0.03,
    spent_usd_per_month: 0.4,
  })
  const series = vi.spyOn(modelsApi, 'getCostSeries').mockResolvedValue({
    conversation_id: 'c1',
    days: 30,
    total_usd: 3,
    series: [
      { date: '2026-11-23', total_usd: 1 },
      { date: '2026-11-24', total_usd: 2 },
    ],
  })
  const wsSeries = vi.spyOn(modelsApi, 'getWorkspaceCostSeries').mockResolvedValue({
    scope: 'workspace',
    workspace_id: 'default',
    days: 30,
    total_usd: 3,
    series: [{ date: '2026-11-23', total_usd: 3 }],
  })
  await act(async () => {
    createRoot(host).render(<CostSummarySection readOnly={false} />)
    await Promise.resolve()
    await Promise.resolve()
  })
  return { save, saveAgent, saveModel, saveWorkspace, series, wsSeries }
}

describe('CostSummarySection monthly budget', () => {
  it('shows the month total and over-budget state', async () => {
    await renderSection()
    expect(host.textContent).toContain('本月 USD 12.000000')
    expect(host.textContent).toContain('本月费用已超过预算')
    expect(host.textContent).toContain('本月预计')
    expect(host.textContent).toContain('按当前速度预计超出月度预算')
    expect(host.textContent).toContain('按模型')
    expect(host.textContent).toContain('power')
    expect(host.textContent).toContain('$2.0000 · 67%')
    expect(host.textContent).toContain('按会话')
    expect(host.textContent).toContain('季度复盘')
    expect(host.textContent).toContain('按工作区')
    expect(host.textContent).toContain('默认')
    expect(host.textContent).toContain('$3.0000 · 100%')
  })

  it('shows the daily trend and exports CSV', async () => {
    const exportSpy = vi
      .spyOn(modelsApi, 'downloadCostExport')
      .mockResolvedValue(new Blob(['date,runs,cost_usd\n'], { type: 'text/csv' }))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    await renderSection()
    expect(host.querySelector('[data-testid="cost-trend"]')).not.toBeNull()
    expect(host.querySelectorAll('[data-testid="cost-trend-bar"]')).toHaveLength(1)
    expect(host.textContent).toContain('天合计 USD 1.000000')
    const button = host.querySelector('[data-testid="cost-export"]') as HTMLButtonElement
    expect(button).not.toBeNull()
    await act(async () => {
      button.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(exportSpy).toHaveBeenCalledWith(30)
    expect(click).toHaveBeenCalled()
  })

  it('exports the breakdown CSV', async () => {
    const exportSpy = vi
      .spyOn(modelsApi, 'downloadCostExport')
      .mockResolvedValue(new Blob(['group,key\n'], { type: 'text/csv' }))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    await renderSection()
    const button = host.querySelector('[data-testid="cost-export-breakdown"]') as HTMLButtonElement
    expect(button).not.toBeNull()
    await act(async () => {
      button.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(exportSpy).toHaveBeenCalledWith(30, 'breakdown')
    expect(click).toHaveBeenCalled()
  })

  it('saves the monthly budget alongside the daily budget', async () => {
    const { save } = await renderSection()
    const inputs = host.querySelectorAll<HTMLInputElement>(
      '.settings-form-actions input[type="number"]',
    )
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
      {
        agent_id: 'a1',
        budget_usd_per_day: 0,
        budget_usd_per_month: 0,
        warning_ratio: 0,
        exceeded_ratio: 0,
        spent_usd_per_day: 0,
        spent_usd_per_month: 0,
      },
    ])
    const row = host.querySelector('.agent-budget-row') as HTMLDivElement
    expect(row).not.toBeNull()
    const inputs = row.querySelectorAll<HTMLInputElement>('input[type="number"]')
    expect(inputs).toHaveLength(4)
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
      warning_ratio: 0,
      exceeded_ratio: 0,
    })
    expect(host.textContent).toContain('已用：日 $0.0100 · 月 $0.2000')
  })

  it('saves per-model budgets', async () => {
    const { saveModel } = await renderSection()
    const section = host.querySelector('[data-testid="model-budgets"]') as HTMLDivElement
    expect(section).not.toBeNull()
    const row = section.querySelector('.agent-budget-row') as HTMLDivElement
    expect(row).not.toBeNull()
    const inputs = row.querySelectorAll<HTMLInputElement>('input[type="number"]')
    expect(inputs).toHaveLength(4)
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
    expect(saveModel).toHaveBeenCalledWith({
      model_profile_id: 'm1',
      budget_usd_per_day: 1,
      budget_usd_per_month: 20,
      warning_ratio: 0,
      exceeded_ratio: 0,
    })
    expect(row.textContent).toContain('已用：日 $0.0200 · 月 $0.3000')
  })

  it('drills into a conversation cost series', async () => {
    const { series } = await renderSection()
    const row = host.querySelector('[data-testid="cost-conversation-c1"]') as HTMLButtonElement
    expect(row).not.toBeNull()
    await act(async () => {
      row.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(series).toHaveBeenCalledWith('c1', 30)
    expect(host.querySelector('[data-testid="cost-conversation-series"]')).not.toBeNull()
    expect(host.textContent).toContain('会话费用（近 30 天）· 季度复盘')
    expect(host.querySelectorAll('[data-testid="cost-series-bar"]')).toHaveLength(2)
  })

  it('drills into a workspace cost series', async () => {
    const { wsSeries } = await renderSection()
    const row = host.querySelector('[data-testid="cost-workspace-default"]') as HTMLButtonElement
    expect(row).not.toBeNull()
    await act(async () => {
      row.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(wsSeries).toHaveBeenCalledWith('default', 30)
    expect(host.textContent).toContain('工作区费用（近 30 天）· 默认')
    expect(host.querySelectorAll('[data-testid="cost-series-bar"]')).toHaveLength(1)
  })

  it('saves per-workspace budgets', async () => {
    const { saveWorkspace } = await renderSection()
    const section = host.querySelector('[data-testid="workspace-budgets"]') as HTMLDivElement
    expect(section).not.toBeNull()
    const row = section.querySelector('.agent-budget-row') as HTMLDivElement
    expect(row).not.toBeNull()
    const inputs = row.querySelectorAll<HTMLInputElement>('input[type="number"]')
    expect(inputs).toHaveLength(4)
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(inputs[0], '2')
      inputs[0].dispatchEvent(new Event('input', { bubbles: true }))
      setter?.call(inputs[1], '30')
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
    expect(saveWorkspace).toHaveBeenCalledWith({
      workspace_id: 'default',
      budget_usd_per_day: 2,
      budget_usd_per_month: 30,
      warning_ratio: 0,
      exceeded_ratio: 0,
    })
    expect(row.textContent).toContain('已用：日 $0.0300 · 月 $0.4000')
  })
})
