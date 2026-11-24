// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '../../api/conversationAttachments'
import { GateContext } from '../../gateContext'
import { setPack } from '../../locale/pack'
import { zhPack } from '../../locales/zh'
import { AttachmentManager } from './AttachmentManager'

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

const items: api.ConversationAttachment[] = [
  {
    object: 'a.pdf',
    filename: '报告.pdf',
    media_type: 'application/pdf',
    size_bytes: 2048,
    created_at: '2026-11-24T04:00:00Z',
    url: '/v0/channels/media/c1/a.pdf',
  },
  {
    object: 'b.png',
    filename: 'shot.png',
    media_type: 'image/png',
    size_bytes: 512,
    created_at: '2026-11-24T04:01:00Z',
    url: '/v0/channels/media/c1/b.png',
  },
]

async function renderManager() {
  const list = vi
    .spyOn(api, 'listConversationAttachments')
    .mockResolvedValueOnce({ items })
    .mockResolvedValueOnce({ items: [items[0]] })
  const remove = vi
    .spyOn(api, 'deleteConversationAttachments')
    .mockResolvedValue({ deleted: 1, failed: 0 })
  vi.spyOn(api, 'getConversationRetention').mockResolvedValue({
    days: -1,
    effective_days: 30,
    global_days: 30,
  })
  vi.spyOn(api, 'getConversationCostBudget').mockResolvedValue({
    budget_usd_per_day: 0,
    budget_usd_per_month: 0,
  })
  const saveBudget = vi.spyOn(api, 'putConversationCostBudget').mockResolvedValue({
    budget_usd_per_day: 1,
    budget_usd_per_month: 20,
  })
  const saveRetention = vi.spyOn(api, 'putConversationRetention').mockResolvedValue({
    days: 7,
    effective_days: 7,
    global_days: 30,
  })
  await act(async () => {
    createRoot(host).render(
      <GateContext.Provider value={{ role: 'admin', gateEnabled: false, operatorId: 'op' }}>
        <AttachmentManager conversationId="c1" open onClose={() => {}} />
      </GateContext.Provider>,
    )
    await Promise.resolve()
    await Promise.resolve()
  })
  return { list, remove, saveRetention, saveBudget }
}

describe('AttachmentManager', () => {
  it('lists indexed attachments', async () => {
    await renderManager()
    const rows = host.querySelectorAll('.attachment-manager-item')
    expect(rows).toHaveLength(2)
    expect(host.textContent).toContain('报告.pdf')
    expect(host.textContent).toContain('shot.png')
  })

  it('deletes selected attachments in bulk', async () => {
    const { remove } = await renderManager()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const checks = host.querySelectorAll<HTMLInputElement>(
      '.attachment-manager-item input[type="checkbox"]',
    )
    expect(checks).toHaveLength(2)
    await act(async () => {
      checks[0].click()
    })
    expect(host.textContent).toContain('已选 1 个')
    const del = host.querySelector('[data-testid="attachment-manager-delete"]') as HTMLButtonElement
    await act(async () => {
      del.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(remove).toHaveBeenCalledWith('c1', ['a.pdf'])
    expect(host.querySelectorAll('.attachment-manager-item')).toHaveLength(1)
  })

  it('saves a conversation retention override', async () => {
    const { saveRetention } = await renderManager()
    const input = host.querySelector<HTMLInputElement>(
      '[data-testid="attachment-retention-days"]',
    )
    expect(input).not.toBeNull()
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(input, '7')
      input!.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const save = host.querySelector(
      '[data-testid="attachment-retention-save"]',
    ) as HTMLButtonElement
    await act(async () => {
      save.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(saveRetention).toHaveBeenCalledWith('c1', 7)
    expect(host.textContent).toContain('本会话：7 天')
  })

  it('saves a conversation budget', async () => {
    const { saveBudget } = await renderManager()
    const day = host.querySelector<HTMLInputElement>('[data-testid="conversation-budget-day"]')
    const month = host.querySelector<HTMLInputElement>('[data-testid="conversation-budget-month"]')
    expect(day).not.toBeNull()
    expect(month).not.toBeNull()
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(day, '1')
      day!.dispatchEvent(new Event('input', { bubbles: true }))
      setter?.call(month, '20')
      month!.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const save = host.querySelector('[data-testid="conversation-budget-save"]') as HTMLButtonElement
    await act(async () => {
      save.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(saveBudget).toHaveBeenCalledWith('c1', 1, 20)
  })

  it('shows an empty state', async () => {
    vi.spyOn(api, 'listConversationAttachments').mockResolvedValue({ items: [] })
    await act(async () => {
      createRoot(host).render(
        <GateContext.Provider value={{ role: 'admin', gateEnabled: false, operatorId: 'op' }}>
          <AttachmentManager conversationId="c1" open onClose={() => {}} />
        </GateContext.Provider>,
      )
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(host.textContent).toContain('这个会话还没有附件')
  })
})
