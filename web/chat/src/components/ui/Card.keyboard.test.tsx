// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Card } from './Card'

describe('Card keyboard activation (a11y)', () => {
  let container: HTMLDivElement | undefined

  afterEach(() => {
    container?.remove()
    container = undefined
  })

  function renderCard(element: ReactNode): HTMLElement {
    const div = document.createElement('div')
    container = div
    document.body.appendChild(div)
    act(() => {
      createRoot(div).render(element)
    })
    return div.querySelector('[data-testid="ui-card"]') as HTMLElement
  }

  it('triggers onClick on Enter and Space for a clickable card', () => {
    const spy = vi.fn()
    const card = renderCard(<Card onClick={spy} title="设置" />)

    act(() => {
      card.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    })
    act(() => {
      card.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true }))
    })

    expect(spy).toHaveBeenCalledTimes(2)
  })

  it('ignores unrelated keys', () => {
    const spy = vi.fn()
    const card = renderCard(<Card onClick={spy} />)

    act(() => {
      card.dispatchEvent(new KeyboardEvent('keydown', { key: 'a', bubbles: true }))
    })

    expect(spy).not.toHaveBeenCalled()
  })

  it('does not mark a non-clickable card as focusable', () => {
    const card = renderCard(<Card title="普通卡片" />)
    expect(card.getAttribute('tabindex')).toBeNull()
    expect(card.getAttribute('role')).toBeNull()
  })
})
