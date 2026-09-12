// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DropdownMenu, type MenuItem } from './DropdownMenu'

let host: HTMLDivElement
let items: MenuItem[]
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
  items = [
    { id: 'copy', label: '复制', onSelect: vi.fn() },
    { id: 'fork', label: '复制成新对话', onSelect: vi.fn() },
  ]
})
afterEach(() => host.remove())

function render() {
  act(() => {
    createRoot(host).render(
      <DropdownMenu triggerLabel="更多操作" items={items} />,
    )
  })
}
function openMenu() {
  act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
  return host.querySelector('[role="menu"]') as HTMLElement
}
const menuItems = () => host.querySelectorAll('[role="menuitem"]') as NodeListOf<HTMLElement>
const press = (el: HTMLElement, key: string) => act(() => {
  el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
})

describe('DropdownMenu', () => {
  it('toggles on trigger click and exposes aria-expanded/menuitem', () => {
    render()
    const trigger = host.querySelector('[data-testid="dropdown-trigger"]') as HTMLButtonElement
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    act(() => { trigger.click() })
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    expect(menuItems().length).toBe(2)
  })
  it('selects an item, fires onSelect, and closes', () => {
    render()
    openMenu()
    act(() => { menuItems()[1].click() })
    expect(items[1].onSelect).toHaveBeenCalledOnce()
    expect((host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).getAttribute('aria-expanded')).toBe('false')
  })
  it('closes on Escape and on outside click', () => {
    render()
    openMenu()
    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(host.querySelector('[role="menu"]')).toBeNull()
    openMenu()
    act(() => { document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true })) })
    expect(host.querySelector('[role="menu"]')).toBeNull()
  })
  it('moves highlight with ArrowDown and Enter activates the highlighted (non-first) item', () => {
    render()
    const menu = openMenu()
    // 初始高亮在 items[0]：ArrowDown 必须真实地把高亮移到 items[1]
    press(menu, 'ArrowDown')
    expect(menuItems()[0].classList.contains('active')).toBe(false)
    expect(menuItems()[1].classList.contains('active')).toBe(true)
    expect(document.activeElement).toBe(menuItems()[1])
    // Enter 激活的是当前高亮项 items[1]，而不是初始的 items[0]
    press(menu, 'Enter')
    expect(items[1].onSelect).toHaveBeenCalledOnce()
    expect(items[0].onSelect).not.toHaveBeenCalled()
  })
  it('wraps highlight around with ArrowUp/ArrowDown', () => {
    render()
    const menu = openMenu()
    // 从首项 ArrowUp 取模环绕到最后一项（index 1）
    press(menu, 'ArrowUp')
    expect(menuItems()[0].classList.contains('active')).toBe(false)
    expect(menuItems()[1].classList.contains('active')).toBe(true)
    // 再 ArrowDown 从末项环绕回首项（1 -> 0）
    press(menu, 'ArrowDown')
    expect(menuItems()[0].classList.contains('active')).toBe(true)
    expect(menuItems()[1].classList.contains('active')).toBe(false)
  })
})
