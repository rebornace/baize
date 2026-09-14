// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ThinkingChip } from './ThinkingChip'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
})

function render(value: string, onChange: (level: string) => void = () => {}, disabled?: boolean) {
  act(() => {
    createRoot(host).render(
      <ThinkingChip value={value} onChange={onChange} disabled={disabled} />,
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
    expect(host.textContent).toContain('默认')
    open()
    const items = host.querySelectorAll('[role="menuitem"]')
    expect(items.length).toBe(5)
    expect(host.textContent).toContain('关')
    expect(host.textContent).toContain('低')
    expect(host.textContent).toContain('中')
    expect(host.textContent).toContain('高')
  })

  it('shows the chosen short label and emits onChange', () => {
    const onChange = vi.fn()
    render('medium', onChange)
    expect(host.textContent).toContain('中')
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
