import { describe, expect, it } from 'vitest'
import {
  selectedSkillsInOrder,
  toggleSkillSelection,
} from './SkillsSettings'
import type { SkillSummary } from '../api'

const sample: SkillSummary[] = [
  {
    id: 'a',
    name: 'A',
    description: '',
    tools: [],
    source: 'builtin',
  },
  {
    id: 'b',
    name: 'B',
    description: '',
    tools: ['t1'],
    source: 'user',
  },
  {
    id: 'c',
    name: 'C',
    description: '',
    tools: [],
    source: 'user',
  },
]

describe('toggleSkillSelection', () => {
  it('adds and removes ids', () => {
    let set = new Set<string>()
    set = toggleSkillSelection(set, 'a', true)
    expect([...set]).toEqual(['a'])
    set = toggleSkillSelection(set, 'b', true)
    expect(set.has('a') && set.has('b')).toBe(true)
    set = toggleSkillSelection(set, 'a', false)
    expect([...set]).toEqual(['b'])
  })
})

describe('selectedSkillsInOrder', () => {
  it('preserves list order of checked skills', () => {
    const selected = new Set(['c', 'a'])
    expect(selectedSkillsInOrder(sample, selected)).toEqual(['a', 'c'])
  })
})
