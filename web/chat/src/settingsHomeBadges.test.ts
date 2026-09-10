import { describe, expect, it } from 'vitest'
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
} from './settingsHomeBadges'

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
