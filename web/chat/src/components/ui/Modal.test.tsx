// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Modal } from './Modal'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => { host.remove() })

function render(el: ReactNode) {
  act(() => { createRoot(host).render(el) })
}

describe('Modal', () => {
  it('renders nothing when closed, renders dialog when open with role/title', () => {
    render(<Modal open={false} title="T"><p>body</p></Modal>)
    expect(host.textContent).not.toContain('body')
    render(<Modal open title="T"><p>body</p></Modal>)
    const dlg = host.querySelector('[role="dialog"]')
    expect(dlg).not.toBeNull()
    expect(host.textContent).toContain('T')
    expect(host.textContent).toContain('body')
  })
  it('closes on Escape and overlay click, not on dialog content click', () => {
    const onClose = vi.fn()
    render(<Modal open title="T" onClose={onClose}><span data-testid="in">x</span></Modal>)
    act(() => {
      host.querySelector('[role="dialog"]')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(onClose).toHaveBeenCalledOnce()
    act(() => { (host.querySelector('[data-testid="modal-overlay"]') as HTMLElement).click() })
    expect(onClose).toHaveBeenCalledTimes(2)
    onClose.mockClear()
    act(() => { (host.querySelector('[data-testid="modal-panel"]') as HTMLElement).click() })
    expect(onClose).not.toHaveBeenCalled()
  })
})
