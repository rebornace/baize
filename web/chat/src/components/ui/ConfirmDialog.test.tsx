// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConfirmDialog } from './ConfirmDialog'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => host.remove())
function render(el: ReactNode) { act(() => { createRoot(host).render(el) }) }

describe('ConfirmDialog', () => {
  it('shows consequence text and calls onConfirm/onCancel', () => {
    const onConfirm = vi.fn(); const onCancel = vi.fn()
    render(<ConfirmDialog open title="删除对话？" body="将永久删除，不可恢复。" confirmText="删除" danger onConfirm={onConfirm} onCancel={onCancel} />)
    expect(host.textContent).toContain('不可恢复')
    act(() => { (host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click() })
    expect(onConfirm).toHaveBeenCalledOnce()
    act(() => { (host.querySelector('[data-testid="confirm-cancel"]') as HTMLButtonElement).click() })
    expect(onCancel).toHaveBeenCalledOnce()
  })
  it('initial focus lands on the cancel button for dangerous confirms', () => {
    render(<ConfirmDialog open title="x" body="y" danger confirmText="删除" onConfirm={() => {}} onCancel={() => {}} />)
    const cancel = host.querySelector('[data-testid="confirm-cancel"]') as HTMLButtonElement
    expect(document.activeElement).toBe(cancel)
  })

  it('does not call onCancel via Escape while busy', () => {
    const onCancel = vi.fn()
    render(<ConfirmDialog open busy title="x" body="y" onConfirm={() => {}} onCancel={onCancel} />)
    act(() => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })) })
    expect(onCancel).not.toHaveBeenCalled()
  })

  it('does not call onCancel via overlay click while busy', () => {
    const onCancel = vi.fn()
    render(<ConfirmDialog open busy title="x" body="y" onConfirm={() => {}} onCancel={onCancel} />)
    act(() => {
      ;(host.querySelector('.modal-overlay') as HTMLElement).dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    expect(onCancel).not.toHaveBeenCalled()
  })

  it('still calls onCancel via Escape when not busy', () => {
    const onCancel = vi.fn()
    render(<ConfirmDialog open title="x" body="y" onConfirm={() => {}} onCancel={onCancel} />)
    act(() => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })) })
    expect(onCancel).toHaveBeenCalledOnce()
  })

  it('renders an error line inside the dialog when error is provided', () => {
    render(<ConfirmDialog open title="x" body="y" error="操作未能完成，请稍后重试。" onConfirm={() => {}} onCancel={() => {}} />)
    expect(host.textContent).toContain('操作未能完成，请稍后重试。')
    expect(host.querySelector('[role="alert"]')).toBeTruthy()
  })
})
