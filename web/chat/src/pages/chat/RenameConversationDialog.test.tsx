// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RenameConversationDialog } from './RenameConversationDialog'

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

async function render(initialTitle: string, onSave: (title: string) => Promise<void>) {
  const onClose = vi.fn()
  await act(async () => {
    createRoot(host).render(
      <RenameConversationDialog
        open
        initialTitle={initialTitle}
        onClose={onClose}
        onSave={onSave}
      />,
    )
    await Promise.resolve()
  })
  return onClose
}

async function setTitle(value: string) {
  const input = document.querySelector<HTMLInputElement>(
    '[data-testid="rename-conversation-title"]',
  )!
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
  await act(async () => {
    setter?.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

describe('RenameConversationDialog', () => {
  it('saves a trimmed title and closes', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const onClose = await render('旧标题', onSave)
    await setTitle('  新标题  ')
    await act(async () => {
      document.querySelector<HTMLButtonElement>('[data-testid="rename-conversation-confirm"]')!.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(onSave).toHaveBeenCalledWith('新标题')
    expect(onClose).toHaveBeenCalled()
  })

  it('allows clearing the title', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    await render('旧标题', onSave)
    await setTitle('')
    await act(async () => {
      document.querySelector<HTMLButtonElement>('[data-testid="rename-conversation-confirm"]')!.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(onSave).toHaveBeenCalledWith('')
  })

  it('shows an error and stays open when saving fails', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('boom'))
    const onClose = await render('旧标题', onSave)
    await act(async () => {
      document.querySelector<HTMLButtonElement>('[data-testid="rename-conversation-confirm"]')!.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(onClose).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('重命名失败')
  })
})
