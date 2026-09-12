// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Composer } from './Composer'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => host.remove())
function render(el: ReactNode) {
  act(() => {
    createRoot(host).render(el)
  })
}

function textarea(): HTMLTextAreaElement {
  return host.querySelector('textarea') as HTMLTextAreaElement
}
function sendButton(): HTMLButtonElement {
  return host.querySelector('.composer-send') as HTMLButtonElement
}
function fileInput(): HTMLInputElement {
  return host.querySelector('.composer-file-input') as HTMLInputElement
}

function typeText(value: string) {
  const ta = textarea()
  const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')!.set!
  act(() => {
    setter.call(ta, value)
    ta.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

function attachFile(name: string) {
  const file = new File(['hello'], name, { type: 'text/plain' })
  const list: FileList = {
    0: file,
    length: 1,
    item: () => file,
  } as unknown as FileList
  act(() => {
    Object.defineProperty(fileInput(), 'files', { value: list, configurable: true })
    fileInput().dispatchEvent(new Event('change', { bubbles: true }))
  })
}

function clickSend() {
  return act(async () => {
    sendButton().click()
  })
}

describe('Composer submit acceptance contract', () => {
  it('keeps text and attachments when onSend resolves to false (rejected)', async () => {
    const onSend = vi.fn().mockResolvedValue(false)
    render(<Composer onSend={onSend} />)
    typeText('被门控拒绝的消息')
    attachFile('note.txt')
    expect(host.querySelector('.composer-chip')).toBeTruthy()

    await clickSend()

    expect(onSend).toHaveBeenCalledWith('被门控拒绝的消息', expect.any(Array))
    // Rejected: both draft text and the attachment chip stay in place.
    expect(textarea().value).toBe('被门控拒绝的消息')
    expect(host.querySelector('.composer-chip')).toBeTruthy()
  })

  it('clears text and attachments when onSend resolves to true (accepted)', async () => {
    const onSend = vi.fn().mockResolvedValue(true)
    render(<Composer onSend={onSend} />)
    typeText('正常发送的消息')
    attachFile('note.txt')

    await clickSend()

    expect(onSend).toHaveBeenCalledOnce()
    expect(textarea().value).toBe('')
    expect(host.querySelector('.composer-chip')).toBeNull()
  })

  it('clears when onSend returns undefined (legacy void contract)', async () => {
    const onSend = vi.fn().mockReturnValue(undefined)
    render(<Composer onSend={onSend} />)
    typeText('旧契约消息')

    await clickSend()

    expect(onSend).toHaveBeenCalledOnce()
    expect(textarea().value).toBe('')
  })
})
