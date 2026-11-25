// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { LocaleProvider } from '../locale/LocaleContext'
import { RouteErrorBoundary, RouteLoadFailed } from './RouteBoundary'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  vi.restoreAllMocks()
})

function Boom(): ReactNode {
  throw new Error('chunk failed')
}

describe('RouteErrorBoundary', () => {
  it('renders the fallback when a child throws', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await act(async () => {
      createRoot(host).render(
        <RouteErrorBoundary fallback={<div data-testid="fallback">failed</div>}>
          <Boom />
        </RouteErrorBoundary>,
      )
    })
    expect(host.querySelector('[data-testid="fallback"]')).not.toBeNull()
  })

  it('offers a reload action', async () => {
    const onReload = vi.fn()
    await act(async () => {
      createRoot(host).render(
        <LocaleProvider>
          <RouteLoadFailed onReload={onReload} />
        </LocaleProvider>,
      )
    })
    const alert = host.querySelector('[data-testid="route-load-failed"]')
    expect(alert).not.toBeNull()
    const button = alert?.querySelector('button')
    expect(button).not.toBeNull()
    await act(async () => {
      button?.click()
    })
    expect(onReload).toHaveBeenCalledTimes(1)
  })
})
