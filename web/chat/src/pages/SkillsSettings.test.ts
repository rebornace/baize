import { describe, expect, it } from 'vitest'
import { mergeSkillSelection, toggleSkillSelection } from './SkillsSettings'

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

describe('mergeSkillSelection', () => {
  const catalog = ['a', 'b', 'c', 'd']

  it('keeps previous order for still-checked ids', () => {
    const previous = ['c', 'a', 'b']
    const selected = new Set(['a', 'b', 'c'])
    expect(mergeSkillSelection(previous, selected, catalog)).toEqual(['c', 'a', 'b'])
  })

  it('drops unchecked and appends new picks in catalog order', () => {
    const previous = ['c', 'a', 'b']
    const selected = new Set(['a', 'd', 'c'])
    expect(mergeSkillSelection(previous, selected, catalog)).toEqual(['c', 'a', 'd'])
  })

  it('skips duplicate previous ids', () => {
    const previous = ['b', 'a', 'b']
    const selected = new Set(['a', 'b'])
    expect(mergeSkillSelection(previous, selected, catalog)).toEqual(['b', 'a'])
  })
})
