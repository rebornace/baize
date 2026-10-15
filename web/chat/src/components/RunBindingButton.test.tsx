// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../api/http'
import { RunBindingButton } from './RunBindingButton'

vi.mock('../api', () => ({
  getRunBinding: vi.fn(),
}))

import { getRunBinding } from '../api'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.mocked(getRunBinding).mockReset()
})
afterEach(() => {
  host.remove()
})

function render(el: ReactNode) {
  act(() => {
    createRoot(host).render(el)
  })
}

describe('RunBindingButton', () => {
  it('opens modal with pretty JSON on success', async () => {
    vi.mocked(getRunBinding).mockResolvedValue({
      run_id: 'r1',
      binding: { model_profile_id: 'mp1', knobs: { thinking: 'high' } },
    })
    render(<RunBindingButton runId="r1" />)
    const btn = host.querySelector('[data-testid="run-binding-open"]') as HTMLButtonElement
    expect(btn).not.toBeNull()
    await act(async () => {
      btn.click()
    })
    expect(getRunBinding).toHaveBeenCalledWith('r1')
    const body = host.querySelector('[data-testid="run-binding-body"]')
    expect(body?.textContent).toContain('model_profile_id')
    expect(body?.textContent).toContain('mp1')
  })

  it('hides itself on 404 binding_not_found', async () => {
    vi.mocked(getRunBinding).mockRejectedValue(
      new ApiError(404, 'binding_not_found', 'missing'),
    )
    render(<RunBindingButton runId="r-missing" />)
    const btn = host.querySelector('[data-testid="run-binding-open"]') as HTMLButtonElement
    await act(async () => {
      btn.click()
    })
    expect(host.querySelector('[data-testid="run-binding-open"]')).toBeNull()
    expect(host.querySelector('[role="dialog"]')).toBeNull()
  })

  it('renders nothing for empty runId', () => {
    render(<RunBindingButton runId="  " />)
    expect(host.querySelector('[data-testid="run-binding-open"]')).toBeNull()
  })
})
