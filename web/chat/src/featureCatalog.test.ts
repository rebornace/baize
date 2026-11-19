import { describe, expect, it } from 'vitest'
import { featureEntries, navigationForEntry, searchFeatures } from './featureCatalog'

describe('featureCatalog', () => {
  it('filters admin-only pages by role', () => {
    const operator = featureEntries('operator')
    expect(operator.some((e) => e.to === '/settings/memory')).toBe(true)
    expect(operator.some((e) => e.to === '/settings/openapi')).toBe(false)

    const admin = featureEntries('admin')
    expect(admin.some((e) => e.to === '/settings/openapi')).toBe(true)
  })

  it('finds run binding by Chinese and English keywords', () => {
    const entries = featureEntries('operator')
    for (const q of ['运行绑定', 'binding', 'run binding', '绑定']) {
      const got = searchFeatures(entries, q)
      expect(got[0]?.id).toBe('action:view_run_binding')
      expect(got[0]?.location).toContain('助手回复')
    }
  })

  it('finds settings pages by Chinese and English', () => {
    const entries = featureEntries('operator')
    expect(searchFeatures(entries, '记忆')[0]?.to).toBe('/settings/memory')
    expect(searchFeatures(entries, 'memory')[0]?.to).toBe('/settings/memory')
    expect(searchFeatures(entries, '日程')[0]?.to).toBe('/settings/schedules')
    expect(searchFeatures(entries, 'schedule')[0]?.to).toBe('/settings/schedules')
  })

  it('requires every whitespace token', () => {
    const entries = featureEntries('operator')
    expect(searchFeatures(entries, '运行 绑定')[0]?.id).toBe('action:view_run_binding')
    expect(searchFeatures(entries, '运行 不存在')).toEqual([])
  })

  it('returns six defaults in the intended order for an empty query', () => {
    const got = searchFeatures(featureEntries('operator'), '')
    expect(got.map((e) => e.id)).toEqual([
      'page:/settings',
      'action:view_run_binding',
      'page:/settings/memory',
      'page:/settings/schedules',
      'page:/settings/models',
      'action:new_chat',
    ])
  })

  it('caps results at ten', () => {
    const got = searchFeatures(featureEntries('admin'), '设置')
    expect(got.length).toBeLessThanOrEqual(10)
  })

  it('keeps hint-only navigation for an action without a parent page', () => {
    const entries = featureEntries('operator')
    const base = entries.find((e) => e.id === 'action:view_run_binding')!
    expect(navigationForEntry({ ...base, to: undefined })).toEqual({
      to: undefined,
      showHint: true,
    })
  })
})
