// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { GateContext } from '../gateContext'
import { LocaleProvider, useLocale } from '../locale/LocaleContext'
import { setPack } from '../locale/pack'
import { LOCALE_STORAGE_KEY } from '../locale/types'
import { zhPack } from '../locales/zh'
import { SettingsLayout } from './SettingsLayout'

function installMatchMedia(initialDark: boolean) {
  const listeners = new Set<() => void>()
  const mql = {
    matches: initialDark,
    addEventListener: (_e: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_e: string, fn: () => void) => listeners.delete(fn),
  }
  window.matchMedia = (() => mql) as unknown as typeof window.matchMedia
}

let host: HTMLDivElement

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  localStorage.clear()
  localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
  setPack(zhPack)
  installMatchMedia(false)
  host = document.createElement('div')
  document.body.appendChild(host)
})

afterEach(() => {
  host.remove()
  localStorage.clear()
  setPack(zhPack)
})

function SwitchToEn() {
  const { setLocale } = useLocale()
  return (
    <button type="button" data-testid="switch-en" onClick={() => setLocale('en')}>
      en
    </button>
  )
}

function render(_ui?: ReactNode) {
  act(() => {
    createRoot(host).render(
      <LocaleProvider>
        <GateContext.Provider value={{ role: 'admin', gateEnabled: true, operatorId: 'op' }}>
          <MemoryRouter initialEntries={['/settings/runtime']}>
            <Routes>
              <Route path="/settings" element={<SettingsLayout />}>
                <Route path="runtime" element={<SwitchToEn />} />
              </Route>
            </Routes>
          </MemoryRouter>
        </GateContext.Provider>
      </LocaleProvider>,
    )
  })
}

describe('SettingsLayout locale', () => {
  it('updates sidebar labels immediately when locale changes', () => {
    render()
    expect(host.textContent).toContain('运行参数')
    expect(host.textContent).toContain('返回聊天')
    expect(host.textContent).not.toContain('Runtime settings')

    act(() => {
      ;(host.querySelector('[data-testid="switch-en"]') as HTMLButtonElement).click()
    })

    expect(host.textContent).toContain('Runtime settings')
    expect(host.textContent).toContain('Back to chat')
    expect(host.textContent).not.toContain('运行参数')
  })
})
