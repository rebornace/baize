// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { packs } from '../locales'
import { LocaleProvider, useLocale } from './LocaleContext'
import { getPack, setPack } from './pack'
import { LOCALE_STORAGE_KEY } from './types'

function Probe() {
  const { locale, setLocale, strings } = useLocale()
  return (
    <div>
      <span data-testid="locale">{locale}</span>
      <span data-testid="probe">{strings.ACTIONS.copy}</span>
      <button type="button" data-testid="to-en" onClick={() => setLocale('en')}>
        en
      </button>
      <button type="button" data-testid="to-zh" onClick={() => setLocale('zh-CN')}>
        zh
      </button>
    </div>
  )
}

let container: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
    true
  localStorage.clear()
  setPack(packs['zh-CN'])
  document.documentElement.lang = ''
  container = document.createElement('div')
  document.body.appendChild(container)
})
afterEach(() => {
  container.remove()
})

function renderProvider() {
  act(() => {
    createRoot(container).render(
      <LocaleProvider>
        <Probe />
      </LocaleProvider>,
    )
  })
}

describe('LocaleProvider', () => {
  it('sets document.documentElement.lang on mount from resolveLocale', () => {
    localStorage.setItem(LOCALE_STORAGE_KEY, 'en')
    renderProvider()
    expect(document.documentElement.lang).toBe('en')
    expect(container.querySelector('[data-testid="locale"]')?.textContent).toBe('en')
    expect(container.querySelector('[data-testid="probe"]')?.textContent).toBe('Copy')
    expect(getPack()).toBe(packs.en)
  })

  it('setLocale switches lang, pack, and persists baize.locale', () => {
    localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
    renderProvider()
    expect(document.documentElement.lang).toBe('zh-CN')

    act(() => {
      ;(container.querySelector('[data-testid="to-en"]') as HTMLButtonElement).click()
    })

    expect(document.documentElement.lang).toBe('en')
    expect(container.querySelector('[data-testid="locale"]')?.textContent).toBe('en')
    expect(container.querySelector('[data-testid="probe"]')?.textContent).toBe('Copy')
    expect(localStorage.getItem(LOCALE_STORAGE_KEY)).toBe('en')
    expect(getPack()).toBe(packs.en)

    act(() => {
      ;(container.querySelector('[data-testid="to-zh"]') as HTMLButtonElement).click()
    })
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(getPack()).toBe(packs['zh-CN'])
  })
})
