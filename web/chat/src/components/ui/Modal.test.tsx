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
  it('Tab 在 panel 内环绕并跳过 disabled，Shift+Tab 反向环绕', () => {
    render(
      <Modal
        open
        title="T"
        footer={
          <>
            <button type="button" id="btn-cancel">取消</button>
            <button type="button" id="btn-disabled" disabled>
              禁用
            </button>
            <button type="button" id="btn-ok">确认</button>
          </>
        }
      >
        <p>body</p>
      </Modal>,
    )
    const cancel = host.querySelector('#btn-cancel') as HTMLButtonElement
    const disabled = host.querySelector('#btn-disabled') as HTMLButtonElement
    const ok = host.querySelector('#btn-ok') as HTMLButtonElement
    // 打开时聚焦第一个可聚焦元素。
    expect(document.activeElement).toBe(cancel)

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }))
    })
    // 中间禁用按钮被跳过，焦点从第一个环绕到最后一个。
    expect(document.activeElement).toBe(ok)
    expect(document.activeElement).not.toBe(disabled)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true }),
      )
    })
    // Shift+Tab 从最后一个环绕回第一个。
    expect(document.activeElement).toBe(cancel)
  })
})
