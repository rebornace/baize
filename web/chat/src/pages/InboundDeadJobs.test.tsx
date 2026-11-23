// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { InboundDeadJobs } from './InboundDeadJobs'

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})

afterEach(() => {
  act(() => {
    root?.unmount()
  })
  host.remove()
  vi.unstubAllGlobals()
})

function render(el: ReactNode) {
  act(() => {
    root = createRoot(host)
    root.render(el)
  })
}

async function flush() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

describe('InboundDeadJobs', () => {
  it('lists dead jobs and retries one', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/retry')) {
        expect(init?.method).toBe('POST')
        return { ok: true, json: async () => ({ status: 'pending' }) } as Response
      }
      return {
        ok: true,
        json: async () => ({
          jobs: [
            {
              id: 'job-1',
              channel: 'feishu',
              status: 'dead',
              attempt: 3,
              max_attempts: 5,
              last_error: 'attachment timeout',
            },
          ],
        }),
      } as Response
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<InboundDeadJobs />)
    await flush()
    expect(host.textContent).toContain('失败的入站消息')
    expect(host.textContent).toContain('feishu · 3/5 次')
    expect(host.textContent).toContain('attachment timeout')

    const retry = Array.from(host.querySelectorAll('button')).find(
      (b) => b.textContent?.trim() === '重试',
    ) as HTMLButtonElement
    expect(retry).toBeTruthy()
    await act(async () => {
      retry.click()
    })
    expect(fetchMock).toHaveBeenCalledWith(
      '/v0/settings/channels/inbound-jobs/job-1/retry',
      expect.objectContaining({ method: 'POST' }),
    )
    await flush()
    expect(host.textContent).toContain('没有失败的入站消息。')
  })

  it('shows the empty state', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: true, json: async () => ({ jobs: [] }) }) as Response),
    )
    render(<InboundDeadJobs />)
    await flush()
    expect(host.textContent).toContain('没有失败的入站消息。')
  })
})
