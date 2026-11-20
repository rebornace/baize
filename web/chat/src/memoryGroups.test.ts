import { describe, expect, it } from 'vitest'
import type { MemoryEntry } from './api'
import { bigramSimilarity, groupMemory } from './memoryGroups'

function entry(id: string, text: string, key?: string, updated = '2026-11-20T00:00:00Z'): MemoryEntry {
  return {
    id,
    owner_id: 'alice',
    key,
    text,
    source: 'explicit',
    created_at: updated,
    updated_at: updated,
  }
}

describe('bigramSimilarity', () => {
  it('ignores whitespace and punctuation', () => {
    expect(bigramSimilarity('喜欢 绿茶。', '喜欢绿茶')).toBe(1)
  })

  it('scores near rephrasing above the 0.72 threshold', () => {
    expect(bigramSimilarity('用户喜欢按完成人数汇报问卷', '用户喜欢按完成人数汇报')).toBeGreaterThanOrEqual(0.72)
  })

  it('does not match unrelated text', () => {
    expect(bigramSimilarity('喜欢绿茶', '住在杭州')).toBe(0)
  })
})

describe('groupMemory', () => {
  it('groups by key and puts ungrouped last', () => {
    const groups = groupMemory([
      entry('m1', '喜欢绿茶', 'preference.tea'),
      entry('m2', '住在杭州'),
      entry('m3', '北京出差', 'city'),
    ])
    expect(groups.map((g) => g.key)).toEqual(['city', 'preference.tea', ''])
    expect(groups[2].entries.map((e) => e.id)).toEqual(['m2'])
  })

  it('marks same-key conflicts', () => {
    const groups = groupMemory([
      entry('m1', '按配额汇报', 'survey.reporting'),
      entry('m2', '按完成人数汇报', 'survey.reporting'),
    ])
    expect(groups).toHaveLength(1)
    expect(groups[0].conflict).toBe(true)
    expect(groups[0].entries).toHaveLength(2)
  })

  it('marks near-duplicate texts even across keys', () => {
    const groups = groupMemory([
      entry('m1', '用户喜欢按完成人数汇报问卷', 'a'),
      entry('m2', '用户喜欢按完成人数汇报', 'b'),
      entry('m3', '完全不同的内容', 'c'),
    ])
    const byKey = new Map(groups.map((g) => [g.key, g]))
    expect(byKey.get('a')?.nearDupIds.has('m1')).toBe(true)
    expect(byKey.get('b')?.nearDupIds.has('m2')).toBe(true)
    expect(byKey.get('c')?.nearDupIds.size).toBe(0)
    expect(byKey.get('a')?.conflict).toBe(true)
  })

  it('sorts risky groups first', () => {
    const groups = groupMemory([
      entry('m1', '普通一', 'zzz'),
      entry('m2', '同 key', 'dup'),
      entry('m3', '同 key 另一条', 'dup'),
    ])
    expect(groups[0].key).toBe('dup')
  })
})
