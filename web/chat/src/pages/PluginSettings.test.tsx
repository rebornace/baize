// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PluginSettings } from './PluginSettings'

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

const pluginConnector = {
  id: 'legacy', type: 'http', base_url: 'http://127.0.0.1:19090',
  require_login: ['ping'], require_approval: [],
  tools: [{ name: 'ping' }],
}

async function renderPlugin() {
  fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
    const u = String(url)
    if (!init?.method && u.endsWith('/v0/tools')) return json({ tools: [{ name: 'ping', connector_id: 'legacy', source: 'plugin' }] })
    if (!init?.method && u.includes('/v0/connectors/legacy')) return json(pluginConnector)
    if (init?.method === 'PUT') return json({ ...pluginConnector, tools: [{ name: 'ping' }], require_login: [], require_approval: [] })
    return json({})
  })
  await act(async () => { createRoot(host).render(<MemoryRouter><PluginSettings /></MemoryRouter>); await new Promise((r) => setTimeout(r, 0)) })
  await flush()
}

describe('PluginSettings page', () => {
  it('renders humanized list card with permission summary', async () => {
    await renderPlugin()
    expect(host.textContent).toContain('插件服务')
    expect(host.textContent).toContain('legacy')
    expect(host.textContent).toContain('1 个工具需本人登录')
  })

  it('creates a plugin with id+base url then saves permissions in a second PUT', async () => {
    await renderPlugin()
    await act(async () => { btn('接入插件服务').click(); await new Promise((r) => setTimeout(r, 0)) })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    // 进入第二步
    expect(host.textContent).toContain('工具权限')
    await act(async () => {
      const box = host.querySelector('input[data-flag="login"]') as HTMLInputElement
      box.click(); await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => { btn('完成').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(2)
    const first = JSON.parse((puts[0][1] as RequestInit).body as string)
    const second = JSON.parse((puts[1][1] as RequestInit).body as string)
    expect(first).toMatchObject({ type: 'http', base_url: 'http://127.0.0.1:19090' })
    expect(first.auth).toBeUndefined()
    expect(second.require_login).toEqual(['ping'])
  })

  it('refreshes the list and shows a success toast when skipping after step-1 save', async () => {
    let created = false
    const newPlugin = {
      id: 'p1', type: 'http', base_url: 'http://127.0.0.1:19090',
      require_login: [], require_approval: [], tools: [{ name: 'ping' }],
    }
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (!init?.method && u.endsWith('/v0/tools')) {
        return json({
          tools: created
            ? [{ name: 'ping', connector_id: 'p1', source: 'plugin' }]
            : [{ name: 'ping', connector_id: 'legacy', source: 'plugin' }],
        })
      }
      if (!init?.method && u.includes('/v0/connectors/p1')) return json(newPlugin)
      if (!init?.method && u.includes('/v0/connectors/legacy')) return json(pluginConnector)
      if (init?.method === 'PUT' && u.includes('/v0/connectors/p1')) {
        created = true
        return json(newPlugin)
      }
      return json({})
    })
    await act(async () => { createRoot(host).render(<MemoryRouter><PluginSettings /></MemoryRouter>); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    const toolsGets = () => fetchMock.mock.calls.filter(
      ([u, i]) => !(i as RequestInit | undefined)?.method && String(u).endsWith('/v0/tools'),
    ).length
    expect(toolsGets()).toBe(1)

    await act(async () => { btn('接入插件服务').click(); await new Promise((r) => setTimeout(r, 0)) })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush(4)
    expect(host.textContent).toContain('工具权限')

    // 第一步已保存成功：点「暂不设置」关闭弹窗，也要提示并刷新列表。
    await act(async () => { btn('暂不设置').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush(5)
    expect(host.textContent).toContain('已保存')
    expect(toolsGets()).toBeGreaterThanOrEqual(2)
    expect(host.textContent).toContain('p1')
  })
})
