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
      '/settings/plugins',
      '/settings/webhooks',
      '/settings/inbox',
      '/settings/channels/weixin',
      '/settings/storage',
    ])
  })

  it('admin includes 渠道 label', () => {
    expect(settingsNavItems('admin').find((x) => x.to === '/settings/channels/weixin')).toEqual({
      to: '/settings/channels/weixin',
      label: '渠道',
    })
  })
})
