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
      if (u.endsWith('/v0/tools')) return json({ tools: [{ name: 'write', connector_id: 'legacy', source: 'mcp' }] })
      if (u.includes('/v0/connectors/legacy')) return json(existing)
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><McpSettings /></MemoryRouter>); await Promise.resolve() })
    await flush()
    expect(host.textContent).toContain('外部工具服务')
    expect(host.textContent).toContain('本地程序 · npx')
    expect(host.textContent).toContain('1 个需审批')
  })

  it('creates an mcp connector then saves approval in a second PUT that resends mcp and omits require_login', async () => {
    const created = { ...existing, id: 'a1', mcp: { transport: 'stdio', command: 'npx', args: [] }, require_approval: [], tools: [{ name: 'query' }] }
    let made = false
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (u.endsWith('/v0/tools')) return json({ tools: made ? [{ name: 'query', connector_id: 'a1', source: 'mcp' }] : [] })
      if (init?.method === 'PUT' && u.includes('/v0/connectors/a1')) { made = true; return json(created) }
      if (!init?.method && u.includes('/v0/connectors/a1')) return json(created)
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
    expect(host.textContent).toContain('工具权限')
    await act(async () => {
      (host.querySelector('input[data-tool="query"][data-flag="approval"]') as HTMLInputElement).click()
      await Promise.resolve()
    })
    await act(async () => { btn('完成').click(); await Promise.resolve() }); await flush(4)
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(2)
    const first = JSON.parse((puts[0][1] as RequestInit).body as string)
    const second = JSON.parse((puts[1][1] as RequestInit).body as string)
    expect(first).toMatchObject({ type: 'mcp', mcp: { transport: 'stdio', command: 'npx' } })
    expect(first.require_login).toBeUndefined()
    expect(second.mcp).toEqual({ transport: 'stdio', command: 'npx', args: [] })
    expect(second.require_approval).toEqual(['query'])
    expect(second.require_login).toBeUndefined()
  })
})
