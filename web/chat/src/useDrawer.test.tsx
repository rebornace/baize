// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { useDrawer } from './useDrawer'

let container: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
    true
  container = document.createElement('div')
  document.body.appendChild(container)
})
afterEach(() => container.remove())

function Harness() {
  const d = useDrawer()
  return (
    <div>
      <span data-testid="open">{String(d.isOpen)}</span>
      <button data-testid="open-btn" onClick={d.open}>open</button>
      <button data-testid="close-btn" onClick={d.close}>close</button>
      <button data-testid="toggle-btn" onClick={d.toggle}>toggle</button>
    </div>
  )
}

function render() {
  act(() => createRoot(container).render(<Harness />))
}
const text = () =>
  (container.querySelector('[data-testid="open"]') as HTMLElement).textContent

describe('useDrawer', () => {
  it('starts closed and opens/closes/toggles', () => {
    render()
    expect(text()).toBe('false')
    act(() => (container.querySelector('[data-testid="open-btn"]') as HTMLButtonElement).click())
    expect(text()).toBe('true')
    act(() => (container.querySelector('[data-testid="close-btn"]') as HTMLButtonElement).click())
    expect(text()).toBe('false')
    act(() => (container.querySelector('[data-testid="toggle-btn"]') as HTMLButtonElement).click())
    expect(text()).toBe('true')
  })
})
