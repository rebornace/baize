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
  await act(async () => {
    createRoot(host).render(
      <GateContext.Provider value={{ role: 'admin', gateEnabled: false, operatorId: 'op' }}>
        <AttachmentManager conversationId="c1" open onClose={() => {}} />
      </GateContext.Provider>,
    )
    await Promise.resolve()
    await Promise.resolve()
  })
  return { list, remove }
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
