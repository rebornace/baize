// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ToolInfo } from '../api'
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
    const tools: ToolInfo[] = [
      { name: 'a', connector_id: 'o1', source: 'spec' },
      { name: 'b', connector_id: 'o2', source: 'extra' },
      { name: 'c', connector_id: 'p1', source: 'plugin' },
    ]
    expect(openApiConnectorIds(tools)).toEqual(['o1', 'o2'])
  })

  it('dedupes connector ids, preserves first-seen order and excludes plugin/mcp', () => {
    // connector_id 为空串：覆盖 !t.connector_id 分支（缺字段场景用窄类型而非 as any）
    const tools: ToolInfo[] = [
      { name: 'a', connector_id: 'o1', source: 'spec' },
      { name: 'b', connector_id: 'o1', source: 'extra' },
      { name: 'c', connector_id: 'o2', source: 'extra' },
      { name: 'd', connector_id: 'o1', source: 'spec' },
      { name: 'e', connector_id: 'p1', source: 'plugin' },
      { name: 'f', connector_id: 'm1', source: 'mcp' },
      { name: 'g', connector_id: 'o3', source: 'spec' },
      { name: 'h', connector_id: '', source: 'spec' },
    ]
    expect(openApiConnectorIds(tools)).toEqual(['o1', 'o2', 'o3'])
  })
})

describe('OpenApiSettings page', () => {
  it('renders humanized card with counts', async () => {
    await renderOpenApi()
    expect(host.textContent).toContain('业务系统')
    expect(host.textContent).toContain('ticket-api')
    expect(host.textContent).toContain('1 个工具需本人登录 · 1 个需审批')
  })

  it('create without a spec is blocked; with url sends one saveInfo PUT then closes', async () => {
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
    await flush(5)
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(1)
    const first = JSON.parse((puts[0][1] as RequestInit).body as string)
    expect(first).toMatchObject({ type: 'openapi', spec_url: 'https://api.example.com/openapi.json' })
    // 高级区默认回传空 capture（mode=static），便于后端按显式字段落库/清空
    expect(first.auth).toEqual({ mode: 'static', capture: {} })
    expect(host.textContent).not.toContain('工具权限')
    expect(host.textContent).toContain('已保存')
  })

  it('create sends single PUT with connection info and closes modal', async () => {
    await renderOpenApi()
    await act(async () => { btn('接入业务系统').click(); await new Promise((r) => setTimeout(r, 0)) })
    const textInputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(textInputs[0], 'o1')
    await setValue(textInputs[1], 'https://api.example.com')
    await setValue(textInputs[2], 'https://api.example.com/openapi.json')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush(5)

    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(1)
    const first = JSON.parse((puts[0][1] as RequestInit).body as string)

    expect(first).toMatchObject({
      type: 'openapi',
      base_url: 'https://api.example.com',
      spec_url: 'https://api.example.com/openapi.json',
    })
    expect(first.auth).toEqual({ mode: 'static', capture: {} })
    expect(first.spec_content).toBeUndefined()
    expect(host.textContent).not.toContain('工具权限')
  })

  it('refreshes the list and shows a success toast after save closes the modal', async () => {
    let created = false
    const newConn = {
      id: 'o1', type: 'openapi', base_url: 'https://api.example.com',
      require_login: [], require_approval: [], tools: [{ name: 'ping' }],
    }
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (!init?.method && u.endsWith('/v0/tools')) {
        return json({
          tools: created
            ? [{ name: 'ping', connector_id: 'o1', source: 'spec' }]
            : [{ name: 'me', connector_id: 'ticket-api', source: 'spec' }],
        })
      }
      if (!init?.method && u.includes('/v0/connectors/o1')) return json(newConn)
      if (!init?.method && u.includes('/v0/connectors/ticket-api')) return json(conn)
      if (init?.method === 'PUT' && u.includes('/v0/connectors/o1')) {
        created = true
        return json(newConn)
      }
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><OpenApiSettings /></MemoryRouter>); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    const toolsGets = () => fetchMock.mock.calls.filter(
      ([u, i]) => !(i as RequestInit | undefined)?.method && String(u).endsWith('/v0/tools'),
    ).length
    expect(toolsGets()).toBe(1)

    await act(async () => { btn('接入业务系统').click(); await new Promise((r) => setTimeout(r, 0)) })
    const textInputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(textInputs[0], 'o1')
    await setValue(textInputs[1], 'https://api.example.com')
    await setValue(textInputs[2], 'https://api.example.com/openapi.json')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush(5)
    expect(host.textContent).toContain('已保存')
    expect(toolsGets()).toBeGreaterThanOrEqual(2)
    expect(host.textContent).toContain('o1')
  })

  it('edit without touching advanced keeps existing auth.capture on saveInfo PUT', async () => {
    const customCapture = {
      tool_name_glob: '*login*',
      token_json_paths: ['$.token', '$.access_token'],
      header_template: 'Bearer {{token}}',
    }
    const connWithCapture = {
      ...conn,
      auth: { mode: 'static' as const, capture: customCapture },
    }
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (!init?.method && u.endsWith('/v0/tools'))
        return json({ tools: [{ name: 'me', connector_id: 'ticket-api', source: 'spec' }] })
      if (!init?.method && u.includes('/v0/connectors/ticket-api')) return json(connWithCapture)
      if (init?.method === 'PUT') return json(connWithCapture)
      return json({})
    })
    await act(async () => {
      createRoot(host).render(<MemoryRouter><OpenApiSettings /></MemoryRouter>)
      await new Promise((r) => setTimeout(r, 0))
    })
    await flush()

    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')]
        .find((i) => i.textContent!.includes('编辑'))!
        .dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await new Promise((r) => setTimeout(r, 0))
    })
    await flush()
    expect(host.textContent).toContain('编辑业务系统')

    // 不改高级区：点「保存连接」→ saveInfo PUT 须回传原 capture（openEdit 水合）
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush(5)

    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(1)
    const body = JSON.parse((puts[0][1] as RequestInit).body as string)
    expect(body.auth).toMatchObject({
      mode: 'static',
      capture: customCapture,
    })
    expect(body.auth.capture).not.toEqual({})
  })
})
