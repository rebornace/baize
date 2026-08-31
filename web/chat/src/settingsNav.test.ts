import { describe, expect, it } from 'vitest'
import { settingsNavItems } from './settingsNav'

describe('settingsNavItems', () => {
  it('operator only sees identities', () => {
    expect(settingsNavItems('operator')).toEqual([
      { to: '/settings/identities', label: '账号' },
    ])
  })
  it('admin sees all', () => {
    expect(settingsNavItems('admin').map((x) => x.to)).toEqual([
      '/settings/tools',
      '/settings/openapi',
      '/settings/skills',
      '/settings/identities',
      '/settings/mcp',
      '/settings/mcp-export',
      '/settings/plugins',
      '/settings/webhooks',
      '/settings/inbox',
      '/settings/channels/weixin',
      '/settings/models',
      '/settings/storage',
    ])
  })

  it('admin includes 模型 label', () => {
    expect(settingsNavItems('admin').find((x) => x.to === '/settings/models')).toEqual({
      to: '/settings/models',
      label: '模型',
    })
  })

  it('operator does not see 模型', () => {
    expect(settingsNavItems('operator').some((x) => x.to === '/settings/models')).toBe(false)
  })

  it('admin includes 渠道 label', () => {
    expect(settingsNavItems('admin').find((x) => x.to === '/settings/channels/weixin')).toEqual({
      to: '/settings/channels/weixin',
      label: '渠道',
    })
  })

  it('admin includes MCP 导出 separate from MCP', () => {
    const items = settingsNavItems('admin')
    expect(items.find((x) => x.to === '/settings/mcp')).toEqual({
      to: '/settings/mcp',
      label: 'MCP',
    })
    expect(items.find((x) => x.to === '/settings/mcp-export')).toEqual({
      to: '/settings/mcp-export',
      label: 'MCP 导出',
    })
  })
})
