// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { GateContext } from '../gateContext'
import { SettingsHome } from './SettingsHome'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.stubGlobal('fetch', vi.fn())
  vi.clearAllMocks()
})
afterEach(() => { host.remove(); vi.unstubAllGlobals() })

function render(role: 'admin' | 'operator') {
  act(() => {
    createRoot(host).render(
      <MemoryRouter>
        <GateContext.Provider value={{ role, gateEnabled: true, operatorId: 'op' }}>
          <SettingsHome />
        </GateContext.Provider>
      </MemoryRouter>,
    )
  })
}
async function settle() {
  await act(async () => { await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0)) })
}

describe('SettingsHome', () => {
  it('admin renders all 13 cards grouped under four group titles', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/tools') return jsonResponse({ tools: [{ name: 't', connector_id: 'oa1', source: 'spec', enabled: true }] })
      if (u === '/v0/skills') return jsonResponse({ skills: [{ id: 's1' }] })
      if (u === '/v0/settings/channels/weixin') return jsonResponse({ running: true })
      if (u === '/v0/settings/events-webhook') return jsonResponse({ url: 'https://x', headers: {} })
      if (u === '/v0/settings/inbox-channels') return jsonResponse({ channels: [] })
      if (u === '/v0/settings/store') return jsonResponse({ driver: 'sqlite' })
      if (u === '/v0/settings/runtime') return jsonResponse({ effective: {}, overridden: {} })
      if (u === '/v0/settings/mcp-export/identities') return jsonResponse([])
      return jsonResponse({ profiles: [{ id: 'm1' }] })
    })
    render('admin')
    await settle()
    for (const name of ['模型', '助手功能', '技能', '业务系统', '外部工具服务', '插件', '对外提供能力', '微信', '消息回调', '外部来信', '账号', '存储', '运行参数']) {
      expect(host.textContent).toContain(name)
    }
    for (const g of ['助手', '连接', '消息', '系统']) expect(host.textContent).toContain(g)
    expect(host.querySelectorAll('[data-testid="ui-card"]')).toHaveLength(13)
  })

  it('admin with zero models shows the go-add CTA linking to models', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) =>
      String(url) === '/v0/settings/models' ? jsonResponse({ profiles: [] }) : jsonResponse(null))
    render('admin')
    await settle()
    expect(host.textContent).toContain('先添加一个模型')
    const cta = host.querySelector('a[href="/settings/models"]')
    expect(cta).toBeTruthy()
    expect(cta?.textContent).toContain('去添加')
  })

  it('operator with zero models sees contact-admin note but no go-add button', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) =>
      String(url) === '/v0/settings/models' ? jsonResponse({ profiles: [] }) : jsonResponse(null))
    render('operator')
    await settle()
    expect(host.textContent).toContain('联系管理员')
    expect(host.querySelector('a[href="/settings/models"]')).toBeNull()
  })

  it('operator locked cards are not buttons and not focusable; allowed cards are', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/tools') return jsonResponse({ tools: [] })
      if (u === '/v0/skills') return jsonResponse({ skills: [] })
      if (u === '/v0/settings/channels/weixin') return jsonResponse({ enabled: false })
      if (u === '/v0/settings/runtime') return jsonResponse({ effective: {}, overridden: {} })
      return jsonResponse({ profiles: [{ id: 'm1' }] })
    })
    render('operator')
    await settle()
    const cards = Array.from(host.querySelectorAll('[data-testid="ui-card"]'))
    const buttons = cards.filter((c) => c.getAttribute('role') === 'button')
    // 6 reachable cards: 模型/助手功能/技能/微信/账号/运行参数
    expect(buttons).toHaveLength(6)
    const locked = cards.filter((c) => c.getAttribute('role') !== 'button')
    expect(locked.length).toBe(7)
    for (const c of locked) expect(c.getAttribute('tabindex')).toBeNull()
    expect(host.textContent).toContain('仅管理员')
  })

  it('admin hides the onboarding bar and add-model link when the models API fails', async () => {
    vi.mocked(globalThis.fetch).mockRejectedValue(new Error('500'))
    render('admin')
    await settle()
    expect(host.textContent).not.toContain('先添加一个模型')
    expect(host.querySelector('a[href="/settings/models"]')).toBeNull()
  })

  it('clicking the refresh button re-fetches badge data', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async () => jsonResponse({ profiles: [{ id: 'm1' }] }))
    render('admin')
    await settle()
    const callsBefore = fetchMock.mock.calls.length
    expect(callsBefore).toBeGreaterThan(0)
    const btn = host.querySelector('button.settings-refresh-btn') as HTMLButtonElement
    await act(async () => { btn.click() })
    await settle()
    expect(fetchMock.mock.calls.length).toBeGreaterThan(callsBefore)
  })
})
