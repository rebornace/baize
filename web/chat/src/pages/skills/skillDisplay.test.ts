import { afterEach, describe, expect, it } from 'vitest'
import { setPack } from '../../locale/pack'
import { enPack } from '../../locales/en'
import { zhPack } from '../../locales/zh'
import { skillDisplayName, skillSourceLabel } from './skillDisplay'

afterEach(() => setPack(zhPack))

describe('skillDisplayName', () => {
  it('localizes builtin and managed skills; keeps user text', () => {
    setPack(zhPack)
    expect(
      skillDisplayName({
        id: 'data-analytics',
        name: 'data-analytics',
        description: 'ignored when builtin map hits',
        tools: [],
        source: 'builtin',
      }),
    ).toContain('数据统计')
    expect(
      skillDisplayName({
        id: 'login-mall',
        name: 'login-mall',
        description: '连接器 mall 的登录相关能力（由连接器自动维护）',
        tools: [],
        source: 'managed',
      }),
    ).toBe('连接器 mall 的登录能力（自动维护）')
    expect(
      skillDisplayName({
        id: 'custom',
        name: 'custom',
        description: '我手写的技能说明',
        tools: [],
        source: 'user',
      }),
    ).toBe('我手写的技能说明')
  })

  it('switches managed/builtin titles with English pack', () => {
    setPack(enPack)
    expect(
      skillDisplayName({
        id: 'data-analytics',
        name: 'data-analytics',
        description: '中文描述',
        tools: [],
        source: 'builtin',
      }),
    ).toMatch(/analytics/i)
    expect(
      skillDisplayName({
        id: 'login-mall',
        name: 'login-mall',
        description: '中文',
        tools: [],
        source: 'managed',
      }),
    ).toMatch(/Login capability for connector mall/i)
    expect(skillSourceLabel('managed')).toBe('Auto-generated')
  })
})
