// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ModelChip } from './ModelChip'
import type { ModelProfile } from '../api'
import { AUTO_MODEL_ID } from '../modelSelect'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => { host.remove() })

const profiles = [
  { id: 'mp1', name: '迷你', model: 'mini', auto_tier: 'light', supports_vision: false },
  { id: 'mpv', name: '看图', model: 'v', auto_tier: 'standard', supports_vision: true },
] as ModelProfile[]

function render(value: string, onChange: (id: string) => void = () => {}) {
  act(() => {
    createRoot(host).render(<ModelChip profiles={profiles} value={value} onChange={onChange} />)
  })
}
const open = () =>
  act(() => {
    ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
  })

describe('ModelChip', () => {
  it('shows 智能选择 for auto and lists tier/vision tagged options', () => {
    render(AUTO_MODEL_ID)
    expect(host.textContent).toContain('智能选择')
    open()
    const items = host.querySelectorAll('[role="menuitem"]')
    expect(items.length).toBe(3)
    expect(host.textContent).toContain('快速')
    expect(host.textContent).toContain('视觉')
  })
  it('shows the chosen short name and emits onChange for a concrete pick', () => {
    const onChange = vi.fn()
    render('mp1', onChange)
    expect(host.textContent).toContain('迷你')
    open()
    act(() => { (host.querySelectorAll('[role="menuitem"]')[1] as HTMLElement).click() })
    expect(onChange).toHaveBeenCalledWith('mp1')
  })
  it('emits AUTO when the smart option is picked', () => {
    const onChange = vi.fn()
    render('mp1', onChange)
    open()
    act(() => { (host.querySelectorAll('[role="menuitem"]')[0] as HTMLElement).click() })
    expect(onChange).toHaveBeenCalledWith(AUTO_MODEL_ID)
  })
  it('treats empty value as auto', () => {
    render('')
    expect(host.textContent).toContain('智能选择')
  })
  it('does not open the menu when disabled', () => {
    act(() => {
      createRoot(host).render(
        <ModelChip profiles={profiles} value={AUTO_MODEL_ID} onChange={() => {}} disabled />,
      )
    })
    open()
    expect(host.querySelectorAll('[role="menuitem"]').length).toBe(0)
  })
})
