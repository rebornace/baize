import { afterEach, describe, expect, it } from 'vitest'
import { setPack } from './locale/pack'
import { enPack } from './locales/en'
import { zhPack } from './locales/zh'
import { SETTINGS_GROUPS, settingsNavItems, visibleNavItems } from './settingsNav'

afterEach(() => {
  setPack(zhPack)
})

describe('SETTINGS_GROUPS', () => {
  it('declares the four groups in display order', () => {
    setPack(zhPack)
    expect(SETTINGS_GROUPS.map((g) => g.id)).toEqual([
      'assistant',
      'connect',
      'messaging',
      'system',
    ])
    expect(SETTINGS_GROUPS.map((g) => g.label)).toEqual(['助手', '连接', '消息', '系统'])
  })

  it('switches group labels to English', () => {
    setPack(enPack)
    expect(SETTINGS_GROUPS.map((g) => g.label)).toEqual([
      'Assistant',
      'Connect',
      'Messaging',
      'System',
    ])
  })
})

describe('settingsNavItems(admin)', () => {
  it('returns 14 items with unique routes and complete metadata', () => {
    const adminItems = settingsNavItems('admin')
    expect(adminItems).toHaveLength(14)
    const tos = adminItems.map((i) => i.to)
    expect(new Set(tos).size).toBe(14)
    for (const item of adminItems) {
      expect(item.label).toBeTruthy()
      expect(item.desc).toBeTruthy()
      expect(
        typeof item.icon === 'function' ||
          (typeof item.icon === 'object' &&
            typeof (item.icon as { render?: unknown }).render === 'function'),
      ).toBe(true)
      expect(SETTINGS_GROUPS.some((g) => g.id === item.group)).toBe(true)
    }
  })

  it('uses the friendly names', () => {
    setPack(zhPack)
    const byTo = Object.fromEntries(settingsNavItems('admin').map((i) => [i.to, i.label]))
    expect(byTo['/settings/models']).toBe('模型')
    expect(byTo['/settings/tools']).toBe('助手功能')
    expect(byTo['/settings/memory']).toBe('账号记忆')
    expect(byTo['/settings/openapi']).toBe('业务系统')
    expect(byTo['/settings/plugins']).toBe('插件服务')
    expect(byTo['/settings/mcp']).toBe('外部工具服务')
    expect(byTo['/settings/mcp-export']).toBe('对外提供能力')
    expect(byTo['/settings/channels/weixin']).toBe('微信')
    expect(byTo['/settings/runtime']).toBe('运行参数')
  })

  it('uses English labels when en pack is active', () => {
    setPack(enPack)
    const byTo = Object.fromEntries(settingsNavItems('admin').map((i) => [i.to, i.label]))
    expect(byTo['/settings/models']).toBe('Models')
    expect(byTo['/settings/tools']).toBe('Assistant capabilities')
    expect(byTo['/settings/runtime']).toBe('Runtime settings')
  })

  it('groups items correctly', () => {
    const adminItems = settingsNavItems('admin')
    const inGroup = (g: string) => adminItems.filter((i) => i.group === g).map((i) => i.to)
    expect(inGroup('assistant')).toEqual([
      '/settings/models',
      '/settings/tools',
      '/settings/skills',
      '/settings/memory',
    ])
    expect(inGroup('connect')).toEqual([
      '/settings/openapi',
      '/settings/mcp',
      '/settings/plugins',
      '/settings/mcp-export',
    ])
    expect(inGroup('messaging')).toEqual([
      '/settings/channels/weixin',
      '/settings/webhooks',
      '/settings/inbox',
    ])
    expect(inGroup('system')).toEqual(['/settings/identities', '/settings/storage', '/settings/runtime'])
  })

  it('declares badge kinds for the cards that have live status', () => {
    const adminItems = settingsNavItems('admin')
    const badgeByTo = Object.fromEntries(adminItems.map((i) => [i.to, i.badge]))
    expect(badgeByTo['/settings/models']).toBe('models')
    expect(badgeByTo['/settings/identities']).toBeUndefined()
    expect(badgeByTo['/settings/memory']).toBeUndefined()
  })
})

describe('settingsNavItems(operator) access levels', () => {
  it('marks read/login/full/locked correctly', () => {
    const operatorItems = settingsNavItems('operator')
    const access = (to: string) => operatorItems.find((i) => i.to === to)?.operator
    expect(access('/settings/models')).toBe('read')
    expect(access('/settings/tools')).toBe('read')
    expect(access('/settings/skills')).toBe('read')
    expect(access('/settings/memory')).toBe('full')
    expect(access('/settings/runtime')).toBe('read')
    expect(access('/settings/channels/weixin')).toBe('login')
    expect(access('/settings/identities')).toBe('full')
    expect(access('/settings/openapi')).toBe('locked')
    expect(access('/settings/storage')).toBe('locked')
  })
})

describe('visibleNavItems', () => {
  it('admin sees every item', () => {
    expect(visibleNavItems('admin')).toHaveLength(14)
  })

  it('operator sidebar hides locked items (no connect group), keeps 7', () => {
    const visible = visibleNavItems('operator')
    expect(visible.map((i) => i.to)).toEqual([
      '/settings/models',
      '/settings/tools',
      '/settings/skills',
      '/settings/memory',
      '/settings/channels/weixin',
      '/settings/identities',
      '/settings/runtime',
    ])
    expect(visible.some((i) => i.group === 'connect')).toBe(false)
  })
})
