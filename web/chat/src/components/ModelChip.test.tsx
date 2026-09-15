// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ModelProfile } from '../api'
import { LocaleProvider } from '../locale/LocaleContext'
import { setPack } from '../locale/pack'
import { LOCALE_STORAGE_KEY } from '../locale/types'
import { zhPack } from '../locales/zh'
import { AUTO_MODEL_ID } from '../modelSelect'
import { AUTO_LABEL, VISION_LABEL, tierLabel } from '../strings'
import { ModelChip } from './ModelChip'

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

const profiles = [
  { id: 'mp1', name: '迷你', model: 'mini', auto_tier: 'light', supports_vision: false },
  { id: 'mpv', name: '看图', model: 'v', auto_tier: 'standard', supports_vision: true },
] as ModelProfile[]

function render(value: string, onChange: (id: string) => void = () => {}) {
  act(() => {
    createRoot(host).render(
      <LocaleProvider>
        <ModelChip profiles={profiles} value={value} onChange={onChange} />
      </LocaleProvider>,
    )
  })
}
const open = () =>
  act(() => {
    ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
  })

describe('ModelChip', () => {
  it('shows 智能选择 for auto and lists tier/vision tagged options', () => {
    render(AUTO_MODEL_ID)
    expect(host.textContent).toContain(AUTO_LABEL)
    open()
    const items = host.querySelectorAll('[role="menuitem"]')
    expect(items.length).toBe(3)
    expect(host.textContent).toContain(tierLabel('light'))
    expect(host.textContent).toContain(VISION_LABEL)
  })

  it('shows selected profile name', () => {
    render('mp1')
    expect(host.textContent).toContain('迷你')
  })

  it('emits onChange when an option is picked', () => {
    const onChange = vi.fn()
    render(AUTO_MODEL_ID, onChange)
    open()
    act(() => {
      ;(host.querySelectorAll('[role="menuitem"]')[1] as HTMLElement).click()
    })
    expect(onChange).toHaveBeenCalledWith('mp1')
  })

  it('falls back to auto label for unknown id', () => {
    render('missing')
    expect(host.textContent).toContain(AUTO_LABEL)
  })
})
