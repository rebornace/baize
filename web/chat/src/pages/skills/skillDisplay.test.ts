import { afterEach, describe, expect, it } from 'vitest'
import { setPack } from '../../locale/pack'
import { enPack } from '../../locales/en'
import { zhPack } from '../../locales/zh'
import {
  skillDisplayName,
  skillLocalizedDescription,
  skillShowSecondaryDescription,
  skillSourceLabel,
  skillToolsCountLabel,
} from './skillDisplay'

afterEach(() => setPack(zhPack))

describe('skillDisplayName', () => {
  it('uses Chinese catalog description for zh-CN', () => {
    setPack(zhPack)
    expect(
      skillDisplayName(
        {
          id: 'data-analytics',
          name: 'data-analytics',
          description: '多源数据统计与完整交互分析页（筛选、下钻、导出 PDF）',
          tools: [],
          source: 'builtin',
        },
        'zh-CN',
      ),
    ).toBe('多源数据统计与完整交互分析页（筛选、下钻、导出 PDF）')
  })

  it('switches data-analytics to English via description_en', () => {
    setPack(enPack)
    expect(
      skillDisplayName(
        {
          id: 'data-analytics',
          name: 'data-analytics',
          description: '多源数据统计与完整交互分析页（筛选、下钻、导出 PDF）',
          description_en: 'Multi-source analytics with interactive pages (filters, drill-down, PDF export)',
          tools: [],
          source: 'builtin',
        },
        'en',
      ),
    ).toMatch(/Multi-source analytics/i)
  })

  it('does not keep Chinese description when English UI lacks description_en', () => {
    setPack(enPack)
    expect(
      skillDisplayName(
        {
          id: 'data-analytics',
          name: 'data-analytics',
          description: '多源数据统计与完整交互分析页（筛选、下钻、导出 PDF）',
          tools: [],
          source: 'builtin',
        },
        'en',
      ),
    ).toMatch(/analytics/i)
    expect(
      skillDisplayName(
        {
          id: 'data-analytics',
          name: 'data-analytics',
          description: '多源数据统计与完整交互分析页（筛选、下钻、导出 PDF）',
          tools: [],
          source: 'builtin',
        },
        'en',
      ),
    ).not.toMatch(/多源/)
  })

  it('localizes managed login titles; keeps user text', () => {
    setPack(zhPack)
    expect(
      skillDisplayName(
        {
          id: 'login-mall',
          name: 'login-mall',
          description: '连接器 mall 的登录相关能力（由连接器自动维护）',
          tools: [],
          source: 'managed',
        },
        'zh-CN',
      ),
    ).toBe('连接器 mall 的登录能力（自动维护）')
    expect(
      skillDisplayName(
        {
          id: 'custom',
          name: 'custom',
          description: '我手写的技能说明',
          tools: [],
          source: 'user',
        },
        'zh-CN',
      ),
    ).toBe('我手写的技能说明')
  })

  it('switches managed titles with English pack', () => {
    setPack(enPack)
    expect(
      skillDisplayName(
        {
          id: 'login-mall',
          name: 'login-mall',
          description: '中文',
          tools: [],
          source: 'managed',
        },
        'en',
      ),
    ).toMatch(/Login capability for connector mall/i)
    expect(skillSourceLabel('managed')).toBe('Auto-generated')
  })
})

describe('skillShowSecondaryDescription', () => {
  it('hides secondary line when title already is the catalog description', () => {
    const s = {
      id: 'baize-help',
      name: 'baize-help',
      description: '解答白泽自身怎么用',
      description_en: 'Explain how Baize itself works',
      tools: [] as string[],
      source: 'builtin' as const,
    }
    expect(skillShowSecondaryDescription(s, 'zh-CN')).toBe(false)
    expect(skillShowSecondaryDescription(s, 'en')).toBe(false)
  })

  it('hides secondary line for managed login skills', () => {
    expect(
      skillShowSecondaryDescription(
        {
          id: 'login-mall',
          name: 'login-mall',
          description: '连接器 mall 的登录相关能力（由连接器自动维护）',
          tools: ['a', 'b'],
          source: 'managed',
        },
        'zh-CN',
      ),
    ).toBe(false)
  })
})

describe('skillLocalizedDescription', () => {
  it('picks description_en for English UI and does not fall back to Chinese', () => {
    const s = {
      id: 'data-analytics',
      name: 'data-analytics',
      description: '多源数据统计与完整交互分析页（筛选、下钻、导出 PDF）',
      description_en: 'Multi-source analytics with interactive pages (filters, drill-down, PDF export)',
      tools: [] as string[],
      source: 'builtin' as const,
    }
    expect(skillLocalizedDescription(s, 'en')).toMatch(/Multi-source analytics/i)
    expect(skillLocalizedDescription(s, 'zh-CN')).toMatch(/多源/)
    expect(
      skillLocalizedDescription(
        {
          ...s,
          description_en: undefined,
        },
        'en',
      ),
    ).toBe('')
  })
})

describe('skillToolsCountLabel', () => {
  it('returns a localized count instead of dumping tool ids', () => {
    setPack(zhPack)
    expect(skillToolsCountLabel(0)).toBe('')
    expect(skillToolsCountLabel(3)).toBe('3 个相关工具')
    setPack(enPack)
    expect(skillToolsCountLabel(1)).toBe('1 related tool')
    expect(skillToolsCountLabel(3)).toBe('3 related tools')
  })
})
