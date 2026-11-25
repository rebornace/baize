// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as metricsApi from '../api/metrics'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { OpsStatusCard } from './OpsStatusCard'

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

describe('OpsStatusCard', () => {
  it('renders active runs, webhook, gate and retention', async () => {
    vi.spyOn(metricsApi, 'getOpsStatus').mockResolvedValue({
      runs: {
        queued: 2,
        running: 1,
        waiting_human: 1,
        active: 4,
        succeeded_24h: 10,
        failed_24h: 3,
      },
      webhook_configured: true,
      budget: { day_alert: 'warning', month_alert: '', scoped_alerts: 2 },
      eval_gate: { last_pass: true, last_ran_at: '2026-11-25T00:00:00Z', total: 5, failed: 0 },
      retention: { attachment_days: 30, run_days: 0, message_days: 90 },
    })
    await act(async () => {
      createRoot(host).render(<OpsStatusCard />)
      await Promise.resolve()
      await Promise.resolve()
    })
    const card = host.querySelector('[data-testid="ops-status"]')
    expect(card).not.toBeNull()
    expect(card?.textContent).toContain('活动运行')
    expect(card?.textContent).toContain('4')
    expect(card?.textContent).toContain('已配置')
    expect(card?.textContent).toContain('通过')
    expect(card?.textContent).toContain('附件 30d')
    expect(card?.textContent).toContain('消息 90d')
    expect(card?.textContent).toContain('Agent/会话告警 2')
  })
})
