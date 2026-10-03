// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { LanguageChip } from './LanguageChip'
import { LocaleProvider } from '../locale/LocaleContext'
import { setPack } from '../locale/pack'
import { LOCALE_STORAGE_KEY } from '../locale/types'
import { zhPack } from '../locales/zh'

let container: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
    true
  localStorage.clear()
  setPack(zhPack)
  container = document.createElement('div')
  document.body.appendChild(container)
})
afterEach(() => {
  container.remove()
})

function renderChip() {
  act(() => {
    createRoot(container).render(
      <LocaleProvider>
        <LanguageChip />
      </LocaleProvider>,
    )
  })
}

describe('LanguageChip', () => {
  it('switches to en and persists baize.locale', () => {
    localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
    renderChip()
    const enBtn = [...container.querySelectorAll('button')].find((b) => b.textContent === 'EN')
    expect(enBtn).toBeTruthy()
    act(() => {
      enBtn!.click()
    })
    expect(localStorage.getItem(LOCALE_STORAGE_KEY)).toBe('en')
    expect(document.documentElement.lang).toBe('en')
    expect(enBtn!.getAttribute('aria-pressed')).toBe('true')
  })
})
