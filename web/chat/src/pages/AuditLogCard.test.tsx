// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as metricsApi from '../api/metrics'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { AuditLogCard } from './AuditLogCard'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.spyOn(metricsApi, 'getAuditRetention').mockResolvedValue({ rows: 5000, min: 100, max: 5000 })
})
afterEach(() => {
  host.remove()
  vi.restoreAllMocks()
  setPack(zhPack)
})

async function flush() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
  })
}

async function render(items: metricsApi.AuditEvent[]) {
  vi.spyOn(metricsApi, 'getAuditEvents').mockResolvedValue({ items, limit: 50 })
  await act(async () => {
    createRoot(host).render(<AuditLogCard />)
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('AuditLogCard', () => {
  it('renders operator, action and status', async () => {
    await render([
      {
        id: 'e1',
        operator_id: 'alice',
        role: 'admin',
        method: 'DELETE',
        action: 'DELETE /v0/conversations/{id}',
        target: '/v0/conversations/c1',
        status: 200,
        created_at: '2026-11-25T00:00:00Z',
      },
      {
        id: 'e2',
        operator_id: '',
        role: 'admin',
        method: 'PUT',
        action: 'PUT /v0/settings/runtime',
        target: '/v0/settings/runtime',
        status: 400,
        created_at: '2026-11-25T00:01:00Z',
      },
    ])
    const card = host.querySelector('[data-testid="audit-log"]')
    expect(card).not.toBeNull()
    expect(card?.textContent).toContain('操作审计')
    expect(card?.textContent).toContain('alice')
    expect(card?.textContent).toContain('DELETE /v0/conversations/{id}')
    expect(card?.textContent).toContain('HTTP 200')
    expect(card?.textContent).toContain('未命名操作员')
    expect(card?.textContent).toContain('HTTP 400')
  })

  it('shows an empty state', async () => {
    await render([])
    expect(host.textContent).toContain('还没有操作记录')
  })

  it('applies the search filter and exports CSV', async () => {
    const getSpy = vi.spyOn(metricsApi, 'getAuditEvents').mockResolvedValue({ items: [], limit: 50 })
    const exportSpy = vi.spyOn(metricsApi, 'exportAuditCSV').mockResolvedValue()
    await act(async () => {
      createRoot(host).render(<AuditLogCard />)
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    await flush()

    const input = host.querySelectorAll('input[type="search"]')[0] as HTMLInputElement
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(input, 'delete')
      input.dispatchEvent(new Event('input', { bubbles: true }))
      await Promise.resolve()
    })
    const apply = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('筛选'),
    ) as HTMLButtonElement | undefined
    expect(apply?.disabled).toBe(false)
    await act(async () => {
      apply?.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    const last = getSpy.mock.calls[getSpy.mock.calls.length - 1]
    expect(last?.[1]).toMatchObject({ q: 'delete' })

    const exportButton = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('导出 CSV'),
    )
    await act(async () => {
      exportButton?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(exportSpy.mock.calls.some((c) => (c[0] as { q?: string })?.q === 'delete')).toBe(true)
  })
})
