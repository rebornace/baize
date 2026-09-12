import { describe, expect, it } from 'vitest'
import { activeMention, findMentions, replaceMention } from './skillMention'

describe('findMentions', () => {
  it('finds @id and /id mentions aligned with the server regex', () => {
    const m = findMentions('hi @search and /sum then @search again')
    expect(m).toHaveLength(3)
    expect(m[0]).toMatchObject({ trigger: '@', id: 'search', start: 3, end: 10 })
    expect(m[1]).toMatchObject({ trigger: '/', id: 'sum', start: 15, end: 19 })
    expect(m[2]).toMatchObject({ trigger: '@', id: 'search' })
  })

  it('matches a leading mention with no preceding whitespace', () => {
    expect(findMentions('@search hi')).toEqual([
      { trigger: '@', id: 'search', start: 0, end: 7 },
    ])
  })

  it('ignores @ or / inside a word (no preceding whitespace)', () => {
    expect(findMentions('foo@bar')).toEqual([])
    expect(findMentions('a/b')).toEqual([])
  })

  it('stops the id at non-id characters', () => {
    expect(findMentions('@search!')).toEqual([
      { trigger: '@', id: 'search', start: 0, end: 7 },
    ])
  })

  it('allows _ and - inside ids', () => {
    expect(findMentions('@my-skill_1 ok')).toEqual([
      { trigger: '@', id: 'my-skill_1', start: 0, end: 11 },
    ])
  })

  it('requires the id to start with an alphanumeric char', () => {
    expect(findMentions('@-bad')).toEqual([])
  })
})

describe('activeMention', () => {
  it('detects a fresh trigger with empty query', () => {
    expect(activeMention('hi @', 4)).toEqual({
      start: 3,
      end: 4,
      trigger: '@',
      query: '',
    })
  })

  it('detects a partial id being typed', () => {
    expect(activeMention('hi @sear', 9)).toEqual({
      start: 3,
      end: 9,
      trigger: '@',
      query: 'sear',
    })
  })

  it('returns null when caret is not in a mention', () => {
    expect(activeMention('hi there', 8)).toBeNull()
  })

  it('returns null when the trigger is mid-word', () => {
    expect(activeMention('foo@bar', 7)).toBeNull()
  })

  it('returns null when a space breaks the mention', () => {
    expect(activeMention('@foo bar', 8)).toBeNull()
  })

  it('supports / as a trigger too', () => {
    expect(activeMention('/sum', 4)).toEqual({
      start: 0,
      end: 4,
      trigger: '/',
      query: 'sum',
    })
  })
})

describe('replaceMention', () => {
  it('replaces the active mention with @id and a trailing space', () => {
    const text = 'hi @sear rest'
    const r = replaceMention(text, 3, 8, 'search')
    expect(r.text).toBe('hi @search  rest')
    expect(r.caret).toBe(11)
  })

  it('handles replacement at the end of text', () => {
    const r = replaceMention('hi @sear', 3, 8, 'search')
    expect(r.text).toBe('hi @search ')
    expect(r.caret).toBe(11)
  })
})
