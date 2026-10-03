// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { LocaleProvider, useLocale } from '../locale/LocaleContext'
import { setPack } from '../locale/pack'
import { LOCALE_STORAGE_KEY } from '../locale/types'
import { zhPack } from '../locales/zh'
import { CHAT, MODELS } from '../strings'
import { ThinkingChip } from './ThinkingChip'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  localStorage.clear()
  localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  localStorage.clear()
  setPack(zhPack)
})

function SwitchLocale({ to }: { to: 'zh-CN' | 'en' }) {
  const { setLocale } = useLocale()
  return (
    <button type="button" data-testid={`to-${to}`} onClick={() => setLocale(to)}>
      {to}
    </button>
  )
}

function render(
  value: string,
  onChange: (level: string) => void = () => {},
  disabled?: boolean,
) {
  act(() => {
    createRoot(host).render(
      <LocaleProvider>
        <ThinkingChip value={value} onChange={onChange} disabled={disabled} />
        <SwitchLocale to="en" />
        <SwitchLocale to="zh-CN" />
      </LocaleProvider>,
    )
  })
}

const open = () =>
  act(() => {
    ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
  })

describe('ThinkingChip', () => {
  it('shows 默认 for empty value and lists five levels', () => {
    render('')
    expect(host.textContent).toContain(CHAT.thinkingDefault)
    open()
    const items = host.querySelectorAll('[role="menuitem"]')
    expect(items.length).toBe(5)
    expect(host.textContent).toContain(MODELS.thinkingLevelOff)
    expect(host.textContent).toContain(MODELS.thinkingLevelLow)
    expect(host.textContent).toContain(MODELS.thinkingLevelMedium)
    expect(host.textContent).toContain(MODELS.thinkingLevelHigh)
  })

  it('updates labels immediately when locale changes', () => {
    render('medium')
    expect(host.textContent).toContain('中')
    expect(host.textContent).not.toContain('Medium')

    act(() => {
      ;(host.querySelector('[data-testid="to-en"]') as HTMLButtonElement).click()
    })
    expect(host.textContent).toContain('Medium')
    expect(host.textContent).not.toContain('中')

    open()
    expect(host.textContent).toContain('Default')
    expect(host.textContent).toContain('Off')
    expect(host.textContent).toContain('High')
  })

  it('shows the chosen short label and emits onChange', () => {
    const onChange = vi.fn()
    render('medium', onChange)
    expect(host.textContent).toContain(MODELS.thinkingLevelMedium)
    open()
    act(() => {
      ;(host.querySelectorAll('[role="menuitem"]')[4] as HTMLElement).click()
    })
    expect(onChange).toHaveBeenCalledWith('high')
  })

  it('emits empty string when 默认 is picked', () => {
    const onChange = vi.fn()
    render('off', onChange)
    open()
    act(() => {
      ;(host.querySelectorAll('[role="menuitem"]')[0] as HTMLElement).click()
    })
    expect(onChange).toHaveBeenCalledWith('')
  })

  it('does not open the menu when disabled', () => {
    render('low', () => {}, true)
    open()
    expect(host.querySelectorAll('[role="menuitem"]').length).toBe(0)
  })
})
