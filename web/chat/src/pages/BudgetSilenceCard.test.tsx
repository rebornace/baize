// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as metricsApi from '../api/metrics'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { BudgetSilenceCard } from './BudgetSilenceCard'

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

describe('BudgetSilenceCard', () => {
  it('shows the active window and clears it', async () => {
    vi.spyOn(metricsApi, 'getBudgetAlertSilence').mockResolvedValue({
      until: '2026-11-26T12:00:00Z',
      reason: '',
      active: true,
    })
    const clear = vi.spyOn(metricsApi, 'setBudgetAlertSilence').mockResolvedValue({
      until: null,
      reason: '',
      active: false,
    })
    await act(async () => {
      createRoot(host).render(<BudgetSilenceCard />)
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(host.textContent).toContain('已静默至')
    const button = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('取消静默'),
    )
    expect(button).not.toBeUndefined()
    await act(async () => {
      button?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(clear).toHaveBeenCalledWith('')
    expect(host.textContent).toContain('临时静默')
  })

  it('starts a one-hour window', async () => {
    vi.spyOn(metricsApi, 'getBudgetAlertSilence').mockResolvedValue({
      until: null,
      reason: '',
      active: false,
    })
    const setSpy = vi.spyOn(metricsApi, 'setBudgetAlertSilence').mockResolvedValue({
      until: '2026-11-26T12:00:00Z',
      reason: '',
      active: true,
    })
    await act(async () => {
      createRoot(host).render(<BudgetSilenceCard />)
      await Promise.resolve()
      await Promise.resolve()
    })
    const button = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('1 小时'),
    )
    await act(async () => {
      button?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(setSpy).toHaveBeenCalledTimes(1)
    expect(host.textContent).toContain('已静默至')
  })
})
