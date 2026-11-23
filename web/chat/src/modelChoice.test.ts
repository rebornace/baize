// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AUTO_MODEL_ID } from './modelSelect'
import type { ModelProfile } from './api'
import { loadModelChoice, saveModelChoice, resolveModelChoice, MODEL_CHOICE_KEY } from './modelChoice'

beforeEach(() => localStorage.clear())
afterEach(() => vi.restoreAllMocks())
const prof = (id: string): ModelProfile =>
  ({ id, name: id, model: 'm', auto_tier: 'standard', supports_vision: false }) as ModelProfile

describe('modelChoice persistence', () => {
  it('defaults to auto', () => {
    expect(loadModelChoice()).toBe(AUTO_MODEL_ID)
  })
  it('round-trips a manual choice', () => {
    saveModelChoice('mp_1')
    expect(localStorage.getItem(MODEL_CHOICE_KEY)).toBe('mp_1')
    expect(loadModelChoice()).toBe('mp_1')
  })
  it('saving auto stores the auto sentinel', () => {
    saveModelChoice('mp_1')
    saveModelChoice(AUTO_MODEL_ID)
    expect(loadModelChoice()).toBe(AUTO_MODEL_ID)
  })
  it('saving blank stores auto', () => {
    saveModelChoice('   ')
    expect(loadModelChoice()).toBe(AUTO_MODEL_ID)
  })
  it('flags stale when the chosen profile is gone', () => {
    saveModelChoice('mp_gone')
    const r = resolveModelChoice(['mp_keep'].map(prof))
    expect(r.choice).toBe(AUTO_MODEL_ID)
    expect(r.stale).toBe(true)
  })
  it('keeps a still-existing manual choice', () => {
    saveModelChoice('mp_keep')
    const r = resolveModelChoice(['mp_keep'].map(prof))
    expect(r.choice).toBe('mp_keep')
    expect(r.stale).toBe(false)
  })
  it('auto is never stale', () => {
    const r = resolveModelChoice([])
    expect(r).toEqual({ choice: AUTO_MODEL_ID, stale: false })
  })
  it('falls back to auto when storage throws (privacy mode)', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('denied')
    })
    expect(loadModelChoice()).toBe(AUTO_MODEL_ID)
    expect(() => saveModelChoice('mp_1')).not.toThrow()
    expect(resolveModelChoice([prof('mp_1')]).choice).toBe(AUTO_MODEL_ID)
  })
})
