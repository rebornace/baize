// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  loadThinkingChoice,
  saveThinkingChoice,
  thinkingChoiceKey,
} from './thinkingChoice'

beforeEach(() => sessionStorage.clear())
afterEach(() => vi.restoreAllMocks())

describe('thinkingChoice persistence', () => {
  it('defaults to empty (follow model) when nothing is stored', () => {
    expect(loadThinkingChoice('convA')).toBe('')
  })

  it('round-trips a level for the same conversation', () => {
    saveThinkingChoice('convA', 'high')
    expect(sessionStorage.getItem(thinkingChoiceKey('convA'))).toBe('high')
    expect(loadThinkingChoice('convA')).toBe('high')
  })

  it('does not restore another conversation’s override', () => {
    saveThinkingChoice('convA', 'high')
    expect(loadThinkingChoice('convB')).toBe('')
  })

  it('saving blank clears the key back to default', () => {
    saveThinkingChoice('convA', 'low')
    saveThinkingChoice('convA', '')
    expect(sessionStorage.getItem(thinkingChoiceKey('convA'))).toBeNull()
    expect(loadThinkingChoice('convA')).toBe('')
  })

  it('falls back to default when storage throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('denied')
    })
    expect(loadThinkingChoice('convA')).toBe('')
    expect(() => saveThinkingChoice('convA', 'medium')).not.toThrow()
  })
})
