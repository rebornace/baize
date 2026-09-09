// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastRegion, useToast } from './Toast'

let host: HTMLDivElement
function renderRegion(): { push: ReturnType<typeof useToast>['push'] } {
  let api!: ReturnType<typeof useToast>
  function Harness() {
    api = useToast()
    return <ToastRegion toasts={api.toasts} onDismiss={api.dismiss} />
  }
  act(() => {
    createRoot(host).render(<Harness />)
  })
  return { push: (...a: Parameters<typeof api.push>) => act(() => { api.push(...a) }) }
}

beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  vi.useFakeTimers()
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  vi.useRealTimers()
})

describe('ToastRegion', () => {
  it('renders a toast with polite live region and auto-dismisses', () => {
    const { push } = renderRegion()
    push({ tone: 'success', title: '已复制' })
    expect(host.textContent).toContain('已复制')
    const region = host.querySelector('[aria-live="polite"]')
    expect(region).not.toBeNull()
    act(() => { vi.advanceTimersByTime(4100) })
    expect(host.textContent).not.toContain('已复制')
  })

  it('keeps error toasts longer (6s) and allows manual close', () => {
    const { push } = renderRegion()
    push({ tone: 'error', title: '出错了', detail: 'boom' })
    act(() => { vi.advanceTimersByTime(4100) })
    expect(host.textContent).toContain('出错了')
    const close = host.querySelector('[data-testid="toast-close"]') as HTMLButtonElement
    act(() => { close.click() })
    expect(host.textContent).not.toContain('出错了')
  })

  it('caps the stack at 3 toasts', () => {
    const { push } = renderRegion()
    push({ tone: 'info', title: '一' })
    push({ tone: 'info', title: '二' })
    push({ tone: 'info', title: '三' })
    push({ tone: 'info', title: '四' })
    expect(host.textContent).not.toContain('一')
    expect(host.textContent).toContain('四')
    expect(host.querySelectorAll('[data-testid="toast"]').length).toBe(3)
  })
})
