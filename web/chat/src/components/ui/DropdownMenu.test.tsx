// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DropdownMenu, type MenuItem } from './DropdownMenu'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => host.remove())

const items: MenuItem[] = [
  { id: 'copy', label: '复制', onSelect: vi.fn() },
  { id: 'fork', label: '复制成新对话', onSelect: vi.fn() },
]
function render() {
  act(() => {
    createRoot(host).render(
      <DropdownMenu triggerLabel="更多操作" items={items} />,
    )
  })
}

describe('DropdownMenu', () => {
  it('toggles on trigger click and exposes aria-expanded/menuitem', () => {
    render()
    const trigger = host.querySelector('[data-testid="dropdown-trigger"]') as HTMLButtonElement
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    act(() => { trigger.click() })
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    expect(host.querySelectorAll('[role="menuitem"]').length).toBe(2)
  })
  it('selects an item, fires onSelect, and closes', () => {
    render()
    act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
    act(() => { (host.querySelectorAll('[role="menuitem"]')[1] as HTMLElement).click() })
    expect(items[1].onSelect).toHaveBeenCalledOnce()
    expect((host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).getAttribute('aria-expanded')).toBe('false')
  })
  it('closes on Escape and on outside click', () => {
    render()
    act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(host.querySelector('[role="menu"]')).toBeNull()
    act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
    act(() => { document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true })) })
    expect(host.querySelector('[role="menu"]')).toBeNull()
  })
  it('moves highlight with ArrowDown/Up and activates with Enter', () => {
    render()
    const trigger = host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement
    act(() => { trigger.click() })
    act(() => { trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })) })
    act(() => {
      ;(host.querySelector('[role="menu"]') as HTMLElement).dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }),
      )
    })
    expect(items[0].onSelect).toHaveBeenCalledOnce()
  })
})
