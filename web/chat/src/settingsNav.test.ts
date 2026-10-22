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
      'conversation',
      'connect',
      'quality',
      'system',
    ])
    expect(SETTINGS_GROUPS.map((g) => g.label)).toEqual([
      '对话相关',
      '连接与工具',
      '质量与评测',
      '系统',
    ])
  })

  it('switches group labels to English', () => {
    setPack(enPack)
    expect(SETTINGS_GROUPS.map((g) => g.label)).toEqual([
      'Chat',
      'Connections',
      'Quality',
      'System',
    ])
  })
})

describe('settingsNavItems(admin)', () => {
  it('returns 19 items with unique routes and complete metadata', () => {
    const adminItems = settingsNavItems('admin')
    expect(adminItems).toHaveLength(19)
    const tos = adminItems.map((i) => i.to)
    expect(new Set(tos).size).toBe(19)
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
    expect(byTo['/settings/tool-matching']).toBe('智能匹配')
    expect(byTo['/settings/openapi']).toBe('业务系统')
    expect(byTo['/settings/plugins']).toBe('插件服务')
    expect(byTo['/settings/mcp']).toBe('扩展工具')
    expect(byTo['/settings/external-login']).toBe('对外登录')
    expect(byTo['/settings/mcp-export']).toBe('对外提供能力')
    expect(byTo['/settings/channels/weixin']).toBe('微信')
    expect(byTo['/settings/runtime']).toBe('高级参数')
    expect(byTo['/settings/metrics']).toBe('运行状态')
    expect(byTo['/settings/eval']).toBe('质量检查')
    expect(byTo['/settings/schedules']).toBe('对话日程')
  })

  it('uses English labels when en pack is active', () => {
    setPack(enPack)
    const byTo = Object.fromEntries(settingsNavItems('admin').map((i) => [i.to, i.label]))
    expect(byTo['/settings/models']).toBe('Models')
    expect(byTo['/settings/tools']).toBe('Assistant capabilities')
    expect(byTo['/settings/tool-matching']).toBe('Smart matching')
    expect(byTo['/settings/mcp']).toBe('Extensions')
    expect(byTo['/settings/runtime']).toBe('Advanced')
    expect(byTo['/settings/metrics']).toBe('Run status')
    expect(byTo['/settings/eval']).toBe('Quality checks')
  })

  it('groups items correctly', () => {
    const adminItems = settingsNavItems('admin')
    const inGroup = (g: string) => adminItems.filter((i) => i.group === g).map((i) => i.to)
    expect(inGroup('conversation')).toEqual([
      '/settings/models',
      '/settings/tools',
      '/settings/skills',
      '/settings/memory',
      '/settings/schedules',
      '/settings/tool-matching',
    ])
    expect(inGroup('connect')).toEqual([
      '/settings/openapi',
      '/settings/mcp',
      '/settings/plugins',
      '/settings/external-login',
      '/settings/mcp-export',
      '/settings/channels/weixin',
      '/settings/webhooks',
      '/settings/inbox',
    ])
    expect(inGroup('quality')).toEqual(['/settings/metrics', '/settings/eval'])
    expect(inGroup('system')).toEqual([
      '/settings/identities',
      '/settings/storage',
      '/settings/runtime',
    ])
  })

  it('declares badge kinds for the cards that have live status', () => {
    const adminItems = settingsNavItems('admin')
    const badgeByTo = Object.fromEntries(adminItems.map((i) => [i.to, i.badge]))
    expect(badgeByTo['/settings/models']).toBe('models')
    expect(badgeByTo['/settings/tool-matching']).toBe('toolMatching')
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
    expect(access('/settings/schedules')).toBe('full')
    expect(access('/settings/tool-matching')).toBe('read')
    expect(access('/settings/runtime')).toBe('read')
    expect(access('/settings/channels/weixin')).toBe('login')
    expect(access('/settings/identities')).toBe('full')
    expect(access('/settings/openapi')).toBe('locked')
    expect(access('/settings/storage')).toBe('locked')
    expect(access('/settings/metrics')).toBe('locked')
    expect(access('/settings/eval')).toBe('locked')
  })
})

describe('visibleNavItems', () => {
  it('admin sees every item', () => {
    expect(visibleNavItems('admin')).toHaveLength(19)
  })

  it('operator sidebar hides locked items, keeps 9 reachable routes', () => {
    const visible = visibleNavItems('operator')
    expect(visible.map((i) => i.to)).toEqual([
      '/settings/models',
      '/settings/tools',
      '/settings/skills',
      '/settings/memory',
      '/settings/schedules',
      '/settings/tool-matching',
      '/settings/channels/weixin',
      '/settings/identities',
      '/settings/runtime',
    ])
    expect(visible.filter((i) => i.group === 'connect').map((i) => i.to)).toEqual([
      '/settings/channels/weixin',
    ])
  })
})
