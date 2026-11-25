// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace } from '../../api'
import { MoveConversationDialog } from './MoveConversationDialog'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

const targets: Workspace[] = [
  { id: 'ws_lab', name: 'Lab', created_at: '2026-01-01T00:00:00Z' },
  { id: 'ws_ops', name: 'Ops', created_at: '2026-01-02T00:00:00Z' },
]

async function render(onMove: (id: string) => Promise<void>, onClose = vi.fn()) {
  await act(async () => {
    createRoot(host).render(
      <MoveConversationDialog open targets={targets} onClose={onClose} onMove={onMove} />,
    )
    await Promise.resolve()
  })
  return onClose
}

describe('MoveConversationDialog', () => {
  it('moves to the first target by default and closes', async () => {
    const onMove = vi.fn().mockResolvedValue(undefined)
    const onClose = await render(onMove)
    const select = document.querySelector<HTMLSelectElement>(
      '[data-testid="move-conversation-target"]',
    )
    expect(select?.value).toBe('ws_lab')
    await act(async () => {
      document.querySelector<HTMLButtonElement>('[data-testid="move-conversation-confirm"]')!.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(onMove).toHaveBeenCalledWith('ws_lab')
    expect(onClose).toHaveBeenCalled()
  })

  it('shows an error and stays open when the move fails', async () => {
    const onMove = vi.fn().mockRejectedValue(new Error('conversation_busy'))
    const onClose = await render(onMove)
    await act(async () => {
      document.querySelector<HTMLButtonElement>('[data-testid="move-conversation-confirm"]')!.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(onClose).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('移动失败')
  })
})
