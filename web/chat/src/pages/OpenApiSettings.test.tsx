// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { OpenApiSettings, openApiConnectorIds } from './OpenApiSettings'

function json(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' }, ...init })
}
let host: HTMLDivElement
let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); fetchMock = vi.fn(); vi.stubGlobal('fetch', fetchMock) })
afterEach(() => { host.remove(); vi.unstubAllGlobals() })
const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const btn = (txt: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!
const setValue = (el: Element, v: string) => act(async () => {
  const s = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  s.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})

const conn = {
  id: 'ticket-api', type: 'openapi', base_url: 'https://api.example.com',
  require_login: ['me'], require_approval: ['create_ticket'],
  tools: [{ name: 'me' }, { name: 'create_ticket' }],
}

async function renderOpenApi() {
  fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
    const u = String(url)
    if (!init?.method && u.endsWith('/v0/tools'))
      return json({ tools: [{ name: 'me', connector_id: 'ticket-api', source: 'spec' }] })
    if (!init?.method && u.includes('/v0/connectors/ticket-api')) return json(conn)
    if (init?.method === 'PUT') return json({ ...conn, require_login: [], require_approval: [] })
    return json({})
  })
  await act(async () => { createRoot(host).render(<MemoryRouter><OpenApiSettings /></MemoryRouter>); await new Promise((r) => setTimeout(r, 0)) })
  await flush()
}

describe('openApiConnectorIds', () => {
  it('includes spec/extra sources only', () => {
    expect(openApiConnectorIds([
      { name: 'a', connector_id: 'o1', source: 'spec' },
      { name: 'b', connector_id: 'o2', source: 'extra' },
      { name: 'c', connector_id: 'p1', source: 'plugin' },
    ] as any)).toEqual(['o1', 'o2'])
  })
})

describe('OpenApiSettings page', () => {
  it('renders humanized card with counts', async () => {
    await renderOpenApi()
    expect(host.textContent).toContain('业务系统')
    expect(host.textContent).toContain('ticket-api')
    expect(host.textContent).toContain('1 个工具需本人登录 · 1 个需审批')
  })

  it('create without a spec is blocked; with url sends spec_url then permissions', async () => {
    await renderOpenApi()
    await act(async () => { btn('接入业务系统').click(); await new Promise((r) => setTimeout(r, 0)) })
    const textInputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(textInputs[0], 'o1')
    await setValue(textInputs[1], 'https://api.example.com')
    // 不提供文档：被拦截
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect(host.textContent).toContain('请上传接口文档或填写文档链接')
    // 文档链接输入是第三个文本框
    await setValue(textInputs[2], 'https://api.example.com/openapi.json')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(host.textContent).toContain('工具权限')
    await act(async () => { btn('完成').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts.length).toBeGreaterThanOrEqual(2)
    const first = JSON.parse((puts[0][1] as RequestInit).body as string)
    expect(first).toMatchObject({ type: 'openapi', spec_url: 'https://api.example.com/openapi.json' })
    expect(first.auth).toBeUndefined()
  })
})
