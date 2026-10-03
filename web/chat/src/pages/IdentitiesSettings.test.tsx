// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IdentitiesSettings } from './IdentitiesSettings'

function json(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.stubGlobal('fetch', vi.fn())
})
afterEach(() => {
  host.remove()
  vi.unstubAllGlobals()
})

async function renderPage() {
  await act(async () => {
    createRoot(host).render(<IdentitiesSettings />)
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

const click = (el: Element) => act(async () => { (el as HTMLElement).click(); await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0)) })

const ident = (o: Record<string, unknown>) => ({
  scheme: 'Bearer', source: 'login_capture', is_default: false, label: '张三', id: 'i1', ...o,
})

describe('IdentitiesSettings', () => {
  it('does not render the manual token box and shows empty state', async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(json([]))
    await renderPage()
    expect(host.textContent).toContain('暂无已登录的业务账号')
    expect(host.querySelector('input[type="password"]')).toBeNull()
    expect(host.textContent).not.toContain('保存 Token')
  })

  it('renders cards with friendly source, default badge and role actions', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (!init?.method && u.endsWith('/identities')) {
        return json([
          ident({ id: 'a', label: '张三', source: 'login_capture', is_default: true }),
          ident({ id: 'b', label: '系统', source: 'env' }),
        ])
      }
      return json({ status: 'ok' })
    })
    await renderPage()
    expect(host.textContent).toContain('张三')
    expect(host.textContent).toContain('对话中登录')
    expect(host.textContent).toContain('系统预设')
    expect(host.textContent).toContain('默认中')
    // 默认账号不出现「设为默认」；env 账号不出现「退出」。
    const cards = host.textContent ?? ''
    expect(cards).toContain('设为默认')
    expect(cards).toContain('退出')
  })

  it('requires confirm before clearing; cancel does nothing, ok deletes all', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (!init?.method && u.endsWith('/identities')) return json([ident({ id: 'a', source: 'manual' })])
      return json({ status: 'ok' })
    })
    await renderPage()

    await click([...host.querySelectorAll('button')].find((b) => b.textContent!.includes('清空登录账号'))!)
    expect(host.querySelector('[data-testid="confirm-ok"]')).not.toBeNull()

    await click(host.querySelector('[data-testid="confirm-cancel"]')!)
    const clearCallsDuringCancel = fetchMock.mock.calls.filter(
      ([u, i]) => String(u).endsWith('/identities') && (i as RequestInit)?.method === 'DELETE',
    )
    expect(clearCallsDuringCancel).toHaveLength(0)

    await click([...host.querySelectorAll('button')].find((b) => b.textContent!.includes('清空登录账号'))!)
    await click(host.querySelector('[data-testid="confirm-ok"]')!)
    const clearCalls = fetchMock.mock.calls.filter(
      ([u, i]) => String(u).endsWith('/identities') && (i as RequestInit)?.method === 'DELETE',
    )
    expect(clearCalls).toHaveLength(1)
  })
})
