// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { McpSettings } from './McpSettings'
import { mcpConnectorIds } from './connectorForms/mcp'
import type { ToolInfo } from '../api'

function json(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
let host: HTMLDivElement
let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); fetchMock = vi.fn(); vi.stubGlobal('fetch', fetchMock) })
afterEach(() => { host.remove(); vi.unstubAllGlobals() })
const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const btn = (t: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(t))!
const setValue = (el: Element, v: string) => act(async () => {
  const s = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  s.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})
const isListMcp = (u: string) => /\/v0\/connectors\?type=mcp/.test(u)

const existing = {
  id: 'legacy', type: 'mcp',
  mcp: { transport: 'stdio', command: 'npx', args: ['srv'] },
  require_login: [], require_approval: ['write'], tools: [{ name: 'write' }],
}

describe('mcpConnectorIds', () => {
  it('filters/dedupes/orders', () => {
    const tools = [
      { name: 'a', source: 'mcp', connector_id: 'c1' },
      { name: 'b', source: 'mcp', connector_id: 'c1' },
      { name: 'c', source: 'http', connector_id: 'x' },
      { name: 'd', source: 'mcp', connector_id: 'c2' },
    ] as unknown as ToolInfo[]
    expect(mcpConnectorIds(tools)).toEqual(['c1', 'c2'])
  })
})

describe('McpSettings page', () => {
  it('lists an mcp connector with transport summary and approval count', async () => {
    fetchMock.mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (isListMcp(u)) return json({ connectors: [existing] })
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    expect(host.textContent).toContain('外部工具服务')
    expect(host.textContent).toContain('本地程序 · npx')
    expect(host.textContent).toContain('1 个需审批')
  })

  it('lists HTTP MCP with zero tools (pre-OAuth)', async () => {
    const empty = {
      id: 'gugu', type: 'mcp',
      mcp: { transport: 'http', url: 'https://mcp.gugudata.com/mcp' },
      require_approval: [], tools: [],
    }
    fetchMock.mockImplementation(async (url: unknown) => {
      if (isListMcp(String(url))) return json({ connectors: [empty] })
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    expect(host.textContent).toContain('gugu')
    expect(host.textContent).toContain('mcp.gugudata.com')
    expect(host.querySelector('[data-testid="dropdown-trigger"]')).toBeTruthy()
  })

  it('creates an mcp connector in a single saveInfo PUT then closes', async () => {
    const created = { ...existing, id: 'a1', mcp: { transport: 'stdio', command: 'npx', args: [] }, require_approval: [], tools: [{ name: 'query' }] }
    let made = false
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (isListMcp(u)) return json({ connectors: made ? [created] : [] })
      if (init?.method === 'PUT' && u.includes('/v0/connectors/a1')) { made = true; return json(created) }
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    await act(async () => { btn('接入外部工具').click(); await Promise.resolve() })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'a1')
    const cmd = [...host.querySelectorAll('input')].find((i) => i.placeholder === 'npx')!
    await setValue(cmd, 'npx')
    await act(async () => { btn('保存连接').click(); await Promise.resolve() }); await flush(4)
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(1)
    const first = JSON.parse((puts[0][1] as RequestInit).body as string)
    expect(first).toMatchObject({ type: 'mcp', mcp: { transport: 'stdio', command: 'npx' } })
    expect(first.require_login).toBeUndefined()
    expect(host.textContent).not.toContain('工具权限')
    expect(host.textContent).toContain('已保存')
  })

  it('shows authorized / needs_reauth badges for HTTP MCP OAuth status', async () => {
    const authorized = {
      id: 'authz', type: 'mcp',
      mcp: { transport: 'http', url: 'https://mcp.example/a', oauth: { status: 'authorized', client_id: 'c1' } },
      require_approval: [], tools: [],
    }
    const reauth = {
      id: 'reauth', type: 'mcp',
      mcp: { transport: 'http', url: 'https://mcp.example/b', oauth: { status: 'needs_reauth', client_id: 'c2' } },
      require_approval: [], tools: [],
    }
    fetchMock.mockImplementation(async (url: unknown) => {
      if (isListMcp(String(url))) return json({ connectors: [authorized, reauth] })
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    expect(host.textContent).toContain('已授权')
    expect(host.textContent).toContain('需重新登录')
  })

  it('clicks 去授权 then POSTs start and opens authorization_url', async () => {
    const httpConn = {
      id: 'remote', type: 'mcp',
      mcp: { transport: 'http', url: 'https://mcp.example/mcp' },
      require_approval: [], tools: [],
    }
    const openSpy = vi.fn()
    vi.stubGlobal('open', openSpy)
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (isListMcp(u)) return json({ connectors: [httpConn] })
      if (u.includes('/mcp/oauth/start') && init?.method === 'POST') {
        return json({ authorization_url: 'https://auth.example/authorize?x=1' })
      }
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await Promise.resolve()
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')]
        .find((i) => i.textContent!.includes('去授权'))!
        .dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
    })
    await flush(4)
    const startCalls = fetchMock.mock.calls.filter(
      ([u, i]) => String(u).includes('/v0/connectors/remote/mcp/oauth/start') && (i as RequestInit)?.method === 'POST',
    )
    expect(startCalls).toHaveLength(1)
    expect(openSpy).toHaveBeenCalledWith('https://auth.example/authorize?x=1', '_blank', 'noopener,noreferrer')
  })

  it('public_base_required opens dialog; save patches runtime then retries start', async () => {
    const httpConn = {
      id: 'remote', type: 'mcp',
      mcp: { transport: 'http', url: 'https://mcp.example/mcp' },
      require_approval: [], tools: [],
    }
    let startCount = 0
    const openSpy = vi.fn()
    vi.stubGlobal('open', openSpy)
    Object.defineProperty(window, 'location', {
      value: { origin: 'http://127.0.0.1:8080' },
      writable: true,
    })
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (isListMcp(u)) return json({ connectors: [httpConn] })
      if (u.includes('/mcp/oauth/start') && init?.method === 'POST') {
        startCount += 1
        if (startCount === 1) {
          return new Response(JSON.stringify({
            error: { code: 'public_base_required', message: 'need public base' },
          }), { status: 400, headers: { 'Content-Type': 'application/json' } })
        }
        return json({ authorization_url: 'https://auth.example/authorize?x=1' })
      }
      if (u === '/v0/settings/runtime' && init?.method === 'PATCH') {
        return json({
          effective: {},
          overridden: {},
          public_base_url: 'http://127.0.0.1:8080',
          public_base_url_overridden: true,
        })
      }
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await Promise.resolve()
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')]
        .find((i) => i.textContent!.includes('去授权'))!
        .dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
    })
    await flush(4)
    expect(host.textContent).toContain('设置公开基址')
    expect(startCount).toBe(1)

    await act(async () => {
      ;[...host.querySelectorAll('button')]
        .find((b) => b.textContent!.includes('保存并继续授权'))!
        .click()
      await Promise.resolve()
    })
    await flush(6)

    const patchCalls = fetchMock.mock.calls.filter(
      ([u, i]) => String(u) === '/v0/settings/runtime' && (i as RequestInit)?.method === 'PATCH',
    )
    expect(patchCalls).toHaveLength(1)
    expect(JSON.parse(String((patchCalls[0][1] as RequestInit).body))).toEqual({
      public_base_url: 'http://127.0.0.1:8080',
    })
    expect(startCount).toBe(2)
    expect(openSpy).toHaveBeenCalledWith('https://auth.example/authorize?x=1', '_blank', 'noopener,noreferrer')
  })

  it('after 去授权, focus/visibilitychange reloads OAuth status badge', async () => {
    let status = ''
    const base = {
      id: 'remote', type: 'mcp',
      mcp: { transport: 'http', url: 'https://mcp.example/mcp', oauth: { status: '', client_id: 'c1' } },
      require_approval: [], tools: [],
    }
    const openSpy = vi.fn()
    vi.stubGlobal('open', openSpy)
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (isListMcp(u)) {
        return json({
          connectors: [{ ...base, mcp: { ...base.mcp, oauth: { status, client_id: 'c1' } } }],
        })
      }
      if (u.includes('/mcp/oauth/start') && init?.method === 'POST') {
        return json({ authorization_url: 'https://auth.example/authorize?x=1' })
      }
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    expect(host.textContent).not.toContain('已授权')

    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await Promise.resolve()
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')]
        .find((i) => i.textContent!.includes('去授权'))!
        .dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
    })
    await flush(4)

    status = 'authorized'
    const listsBefore = fetchMock.mock.calls.filter(([u]) => isListMcp(String(u))).length
    await act(async () => {
      window.dispatchEvent(new Event('focus'))
      await Promise.resolve()
    })
    await flush(4)
    const listsAfter = fetchMock.mock.calls.filter(([u]) => isListMcp(String(u))).length
    expect(listsAfter).toBeGreaterThan(listsBefore)
    expect(host.textContent).toContain('已授权')
  })

  it('clicks 断开授权 then POSTs disconnect, toasts, and reloads', async () => {
    let disconnected = false
    const authorized = {
      id: 'authz', type: 'mcp',
      mcp: { transport: 'http', url: 'https://mcp.example/mcp', oauth: { status: 'authorized', client_id: 'c1' } },
      require_approval: [], tools: [],
    }
    const after = {
      ...authorized,
      mcp: { transport: 'http', url: 'https://mcp.example/mcp', oauth: { status: '', client_id: 'c1' } },
    }
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (isListMcp(u)) return json({ connectors: [disconnected ? after : authorized] })
      if (u.includes('/mcp/oauth/disconnect') && init?.method === 'POST') {
        disconnected = true
        return json({ id: 'authz', status: '' })
      }
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    expect(host.textContent).toContain('已授权')
    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await Promise.resolve()
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')]
        .find((i) => i.textContent!.includes('断开授权'))!
        .dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
    })
    await flush(6)
    const disconnectCalls = fetchMock.mock.calls.filter(
      ([u, i]) => String(u).includes('/v0/connectors/authz/mcp/oauth/disconnect') && (i as RequestInit)?.method === 'POST',
    )
    expect(disconnectCalls).toHaveLength(1)
    expect(host.textContent).toContain('已断开授权')
    expect(host.textContent).not.toContain('已授权')
  })
})
