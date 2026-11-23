// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { GateContext } from '../gateContext'
import { LocaleProvider } from '../locale/LocaleContext'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { FeatureSearchButton, FeatureSearchProvider } from './FeatureSearch'

let host: HTMLDivElement
let root: ReturnType<typeof createRoot>

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  localStorage.clear()
  localStorage.setItem('baize.locale', 'zh-CN')
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})

afterEach(() => {
  act(() => {
    root.unmount()
  })
  host.remove()
  localStorage.clear()
})

function renderHarness(role: 'operator' | 'admin' = 'operator') {
  act(() => {
    root.render(
      <LocaleProvider>
        <GateContext.Provider value={{ role, gateEnabled: false, operatorId: 'local' }}>
          <MemoryRouter initialEntries={['/']}>
            <FeatureSearchProvider>
              <FeatureSearchButton />
              <Routes>
                <Route path="/" element={<div>chat-home</div>} />
                <Route path="/settings/memory" element={<div>memory-page</div>} />
              </Routes>
            </FeatureSearchProvider>
          </MemoryRouter>
        </GateContext.Provider>
      </LocaleProvider>,
    )
  })
}

function input(): HTMLInputElement {
  return host.querySelector('[data-testid="feature-search-input"]') as HTMLInputElement
}

function setInputValue(el: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  act(() => {
    setter.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

function openWithShortcut() {
  act(() => {
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }))
  })
}

describe('FeatureSearch', () => {
  it('opens with Ctrl+K and closes with Esc', () => {
    renderHarness()
    openWithShortcut()
    expect(input()).toBeTruthy()
    act(() => {
      input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(host.querySelector('[data-testid="feature-search-input"]')).toBeNull()
  })

  it('navigates to a settings page', async () => {
    renderHarness()
    openWithShortcut()
    setInputValue(input(), '记忆')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0))
    })
    act(() => {
      input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    })
    expect(host.textContent).toContain('memory-page')
  })

  it('shows a location hint for an action', () => {
    renderHarness()
    openWithShortcut()
    setInputValue(input(), '运行绑定')
    act(() => {
      input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    })
    expect(host.textContent).toContain('聊天页 → 助手回复下方')
  })

  it('opens with / when focus is not editable', () => {
    renderHarness()
    act(() => {
      document.body.tabIndex = -1
      document.body.focus()
      window.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true }))
    })
    expect(input()).toBeTruthy()
  })

  it('returns focus to the trigger after Esc', async () => {
    renderHarness()
    const btn = host.querySelector('[data-testid="feature-search-trigger"]') as HTMLButtonElement
    act(() => {
      btn.focus()
      btn.click()
    })
    expect(input()).toBeTruthy()
    act(() => {
      input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })
    expect(document.activeElement).toBe(btn)
  })

  it('hides admin-only pages from operators', () => {
    renderHarness('operator')
    openWithShortcut()
    setInputValue(input(), 'openapi')
    expect(host.textContent).toContain('没有找到匹配的功能')
  })
})
