// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ThemeToggle } from './ThemeToggle'

function installMatchMedia(initialDark: boolean) {
  const listeners = new Set<() => void>()
  const mql = {
    matches: initialDark,
    addEventListener: (_e: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_e: string, fn: () => void) => listeners.delete(fn),
  }
  window.matchMedia = (() => mql) as unknown as typeof window.matchMedia
}

let container: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
    true
  localStorage.clear()
  installMatchMedia(false)
  container = document.createElement('div')
  document.body.appendChild(container)
})
afterEach(() => {
  container.remove()
})

function render() {
  act(() => {
    createRoot(container).render(<ThemeToggle />)
  })
}

describe('ThemeToggle', () => {
  it('applies and persists dark when the dark button is pressed', () => {
    render()
    const btn = container.querySelector('[aria-label="深色"]') as HTMLButtonElement
    act(() => btn.click())
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(localStorage.getItem('baize.theme')).toBe('dark')
    expect(btn.getAttribute('aria-pressed')).toBe('true')
  })

  it('applies light and keeps the other options unpressed', () => {
    localStorage.setItem('baize.theme', 'dark')
    render()
    const light = container.querySelector('[aria-label="浅色"]') as HTMLButtonElement
    act(() => light.click())
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(light.getAttribute('aria-pressed')).toBe('true')
    expect(
      (container.querySelector('[aria-label="跟随系统"]') as HTMLButtonElement).getAttribute(
        'aria-pressed',
      ),
    ).toBe('false')
  })

  it('storage 抛错时显式选择不被系统偏好覆盖', () => {
    // 隐私模式：setItem 抛错但 getItem 正常（返回 null/空）；系统偏好为浅色。
    const setItemSpy = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new Error('denied')
      })
    try {
      render()
      const btn = container.querySelector('[aria-label="深色"]') as HTMLButtonElement
      act(() => btn.click())
      // choice 状态是事实源：即便持久化失败，显式 dark 也不得被回退成系统 light。
      expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    } finally {
      setItemSpy.mockRestore()
    }
  })
})
