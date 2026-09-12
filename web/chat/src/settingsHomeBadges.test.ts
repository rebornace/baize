// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  EventsWebhookConfig,
  InboxChannel,
  ModelProfile,
  RuntimeKnobsView,
  SkillSummary,
  StoreSettings,
  ToolInfo,
  WeixinChannelSettings,
} from './api'
import type { MCPExportIdentity } from './api'
import {
  countConnectorsBySource,
  enabledToolCount,
  resolveBadge,
  useSettingsBadges,
} from './settingsHomeBadges'
import { settingsNavItems } from './settingsNav'

const model = (over: Partial<ModelProfile> = {}): ModelProfile => ({
  id: 'm1', name: 'm', provider: 'openai_compatible', base_url: 'u', model: 'gpt',
  disable_thinking: false, supports_vision: false, context_tokens: 1, auto_tier: 'standard', ...over,
})
const tool = (source: string, connector = 'c', enabled = true): ToolInfo => ({
  name: source + connector, connector_id: connector, source, enabled,
})
const skill = (id: string): SkillSummary => ({ id, name: id, description: '', tools: [], source: 'builtin' })

describe('enabledToolCount', () => {
  it('counts only enabled tools', () => {
    expect(enabledToolCount([tool('spec', 'a'), tool('mcp', 'b', false)])).toBe(1)
    expect(enabledToolCount([])).toBe(0)
  })
})

describe('countConnectorsBySource', () => {
  it('dedupes connectors and counts only matching sources', () => {
    const tools = [
      tool('spec', 'oa1'), tool('extra', 'oa1'), tool('spec', 'oa2'),
      tool('mcp', 'm1'), tool('plugin', 'p1'),
    ]
    expect(countConnectorsBySource(tools, ['spec', 'extra'])).toBe(2)
    expect(countConnectorsBySource(tools, ['mcp'])).toBe(1)
    expect(countConnectorsBySource(tools, ['plugin'])).toBe(1)
  })
})

describe('resolveBadge', () => {
  it('models: 0 warns, >0 ok', () => {
    expect(resolveBadge('models', [])).toEqual({ tone: 'warning', text: '未配置' })
    expect(resolveBadge('models', [model(), model()])).toEqual({ tone: 'success', text: '已配置 2 个' })
  })

  it('tools: none hidden, some enabled ok, all exist-but-disabled warns', () => {
    expect(resolveBadge('tools', [])).toBeNull()
    expect(resolveBadge('tools', [tool('spec', 'a'), tool('mcp', 'b', false)])).toEqual({ tone: 'success', text: '1 项可用' })
    expect(resolveBadge('tools', [tool('spec', 'a', false)])).toEqual({ tone: 'warning', text: '未启用' })
  })

  it('skills / mcpExport / inbox: count shown neutral, zero hidden', () => {
    expect(resolveBadge('skills', { skills: [skill('a'), skill('b')] })).toEqual({ tone: 'neutral', text: '2 个技能' })
    expect(resolveBadge('skills', { skills: [] })).toBeNull()
    const id = (i: string): MCPExportIdentity => ({ id: i, name: i, scheme: '' }) as MCPExportIdentity
    expect(resolveBadge('mcpExport', [id('x')])).toEqual({ tone: 'neutral', text: '1 个出口' })
    expect(resolveBadge('mcpExport', [])).toBeNull()
    const ch = (i: string): InboxChannel => ({ id: i, agent_id: 'a', enabled: true })
    expect(resolveBadge('inbox', [ch('c1')])).toEqual({ tone: 'neutral', text: '1 个收件地址' })
    expect(resolveBadge('inbox', [])).toBeNull()
  })

  it('openapi/mcp/plugins connectors: n ok, zero neutral go-connect for openapi, hidden otherwise', () => {
    expect(resolveBadge('openapi', [tool('mcp', 'm1')])).toEqual({ tone: 'neutral', text: '去接入' })
    expect(resolveBadge('openapi', [tool('spec', 'oa1')])).toEqual({ tone: 'success', text: '已接 1 个' })
    expect(resolveBadge('mcp', [tool('mcp', 'm1')])).toEqual({ tone: 'success', text: '已接 1 个' })
    expect(resolveBadge('mcp', [tool('spec', 'oa1')])).toBeNull()
    expect(resolveBadge('plugins', [tool('plugin', 'p1')])).toEqual({ tone: 'success', text: '已接 1 个' })
    expect(resolveBadge('plugins', [])).toBeNull()
  })

  it('weixin: running / enabled-with-reason / disabled / missing', () => {
    expect(resolveBadge('weixin', { running: true } as WeixinChannelSettings)).toEqual({ tone: 'success', text: '运行中' })
    expect(resolveBadge('weixin', { enabled: true, running: false, reason: 'login_required' })).toEqual({ tone: 'warning', text: '待登录' })
    expect(resolveBadge('weixin', { enabled: true, running: false, reason: 'start_failed' })).toEqual({ tone: 'warning', text: '启动异常' })
    expect(resolveBadge('weixin', { enabled: true, running: false, reason: 'stopped' })).toEqual({ tone: 'warning', text: '已停用' })
    expect(resolveBadge('weixin', { enabled: false, running: false })).toEqual({ tone: 'neutral', text: '未接入' })
    expect(resolveBadge('weixin', {} as WeixinChannelSettings)).toEqual({ tone: 'neutral', text: '未接入' })
  })

  it('webhook: url set ok, empty neutral', () => {
    const set: EventsWebhookConfig = { url: 'https://x', headers: {} }
    const empty: EventsWebhookConfig = { url: '', headers: {} }
    expect(resolveBadge('webhook', set)).toEqual({ tone: 'success', text: '已设置' })
    expect(resolveBadge('webhook', empty)).toEqual({ tone: 'neutral', text: '未设置' })
  })

  it('store: sqlite maps to 本地文件, other drivers shown lowercase as-is', () => {
    expect(resolveBadge('store', { driver: 'sqlite' } as StoreSettings)).toEqual({ tone: 'neutral', text: '本地文件' })
    expect(resolveBadge('store', { driver: 'postgres' } as StoreSettings)).toEqual({ tone: 'neutral', text: 'postgres' })
  })

  it('runtime: any override customised, else default', () => {
    const view = (overridden: Record<string, boolean>): RuntimeKnobsView =>
      ({ effective: {}, overridden }) as unknown as RuntimeKnobsView
    expect(resolveBadge('runtime', view({}))).toEqual({ tone: 'neutral', text: '默认' })
    expect(resolveBadge('runtime', view({ max_messages: true }))).toEqual({ tone: 'neutral', text: '已自定义' })
  })
})

// ---- useSettingsBadges ----

const hookHosts: HTMLElement[] = []

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
function runHook<T>(useHook: (refreshKey: number) => T): {
  value: () => T
  rerender: (p: { key: number }) => void
  unmount: () => void
} {
  let current: T
  const host = document.createElement('div')
  document.body.appendChild(host)
  hookHosts.push(host)
  const root = createRoot(host)
  const Harness = ({ keyStep }: { keyStep: number }) => {
    current = useHook(keyStep)
    return null
  }
  act(() => { root.render(createElement(Harness, { keyStep: 0 })) })
  return {
    value: () => current,
    rerender: (p) => act(() => { root.render(createElement(Harness, { keyStep: p.key })) }),
    unmount: () => act(() => { root.unmount() }),
  }
}

describe('useSettingsBadges', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
    ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    hookHosts.splice(0).forEach((h) => h.remove())
  })

  const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)) })

  it('admin: fetches shared tool list once and resolves all badges', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/tools') return jsonResponse({ tools: [{ name: 't', connector_id: 'oa1', source: 'spec', enabled: true }] })
      if (u === '/v0/settings/models') return jsonResponse({ profiles: [{ id: 'm1' }] })
      if (u === '/v0/skills') return jsonResponse({ skills: [{ id: 's1' }] })
      if (u === '/v0/settings/channels/weixin') return jsonResponse({ running: true })
      if (u === '/v0/settings/events-webhook') return jsonResponse({ url: 'https://x', headers: {} })
      if (u === '/v0/settings/inbox-channels') return jsonResponse({ channels: [] })
      if (u === '/v0/settings/store') return jsonResponse({ driver: 'sqlite' })
      if (u === '/v0/settings/runtime') return jsonResponse({ effective: {}, overridden: {} })
      if (u === '/v0/settings/mcp-export/identities') return jsonResponse([])
      return jsonResponse(null)
    })
    const h = runHook(() => useSettingsBadges(settingsNavItems('admin'), 'admin', 0))
    await flush()
    const toolsCalls = fetchMock.mock.calls.filter(([u]) => String(u) === '/v0/tools').length
    expect(toolsCalls).toBe(1)
    const badges = h.value()
    expect(badges.models).toEqual({ tone: 'success', text: '已配置 1 个' })
    expect(badges.weixin).toEqual({ tone: 'success', text: '运行中' })
    expect(badges.store).toEqual({ tone: 'neutral', text: '本地文件' })
  })

  it('operator: never requests locked kinds (no webhook/inbox/store/mcp-export)', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async () => jsonResponse(null))
    runHook(() => useSettingsBadges(settingsNavItems('admin'), 'operator', 0))
    await flush()
    const urls = fetchMock.mock.calls.map(([u]) => String(u))
    expect(urls).not.toContain('/v0/settings/events-webhook')
    expect(urls).not.toContain('/v0/settings/inbox-channels')
    expect(urls).not.toContain('/v0/settings/store')
    expect(urls).not.toContain('/v0/settings/mcp-export/identities')
    // operator-allowed kinds ARE fetched
    expect(urls).toContain('/v0/tools')
    expect(urls).toContain('/v0/settings/channels/weixin')
  })

  it('a failing request yields null badge without affecting other kinds', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async (url: unknown) =>
      String(url) === '/v0/settings/models'
        ? new Response('nope', { status: 500 })
        : jsonResponse({ profiles: [{ id: 'm1' }] }))
    const h = runHook(() => useSettingsBadges(
      settingsNavItems('admin').filter((i) => i.badge === 'models' || i.badge === 'weixin'),
      'admin', 0))
    await flush()
    const badges = h.value()
    expect(badges.models).toBeNull()
  })

  it('re-fetches when refreshKey changes', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async () => jsonResponse({ profiles: [] }))
    const h = runHook((k) => useSettingsBadges(settingsNavItems('admin'), 'admin', k))
    await flush()
    h.rerender({ key: 1 })
    await flush()
    expect(fetchMock.mock.calls.length).toBeGreaterThan(1)
  })

  it('tools request failure: tools/openapi/mcp/plugins badges are all null (no 去接入 CTA)', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async (url: unknown) =>
      String(url) === '/v0/tools'
        ? new Response('boom', { status: 500 })
        : jsonResponse(null))
    const h = runHook(() => useSettingsBadges(
      settingsNavItems('admin').filter((i) =>
        i.badge === 'tools' || i.badge === 'openapi' || i.badge === 'mcp' || i.badge === 'plugins'),
      'admin', 0))
    await flush()
    const badges = h.value()
    expect(badges.tools).toBeNull()
    expect(badges.openapi).toBeNull()
    expect(badges.mcp).toBeNull()
    expect(badges.plugins).toBeNull()
  })

  it('tools 200 with zero connectors: openapi still shows 去接入 (distinct from request failure)', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async (url: unknown) =>
      String(url) === '/v0/tools'
        ? jsonResponse({ tools: [] })
        : jsonResponse(null))
    const h = runHook(() => useSettingsBadges(
      settingsNavItems('admin').filter((i) =>
        i.badge === 'tools' || i.badge === 'openapi' || i.badge === 'mcp' || i.badge === 'plugins'),
      'admin', 0))
    await flush()
    const badges = h.value()
    expect(badges.openapi).toEqual({ tone: 'neutral', text: '去接入' })
    expect(badges.tools).toBeNull()
    expect(badges.mcp).toBeNull()
    expect(badges.plugins).toBeNull()
  })

  it('unmount before fetch settles: no setState after unmount', async () => {
    const fetchMock = vi.mocked(globalThis.fetch)
    fetchMock.mockImplementation(async () => jsonResponse({ profiles: [{ id: 'm1' }] }))
    const h = runHook(() => useSettingsBadges(
      settingsNavItems('admin').filter((i) => i.badge === 'models'),
      'admin', 0))
    expect(h.value().models).toBeUndefined()
    h.unmount()
    // Let the pending fetch resolve; a post-unmount setState would surface as an act warning.
    await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
    expect(fetchMock.mock.calls.length).toBe(1)
  })
})
