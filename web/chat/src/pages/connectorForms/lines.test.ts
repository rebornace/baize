import { describe, expect, it } from 'vitest'
import {
  formatKeyValueMap, formatStringList, parseArgsText, parseKeyValueLines, parseLineList,
} from './lines'

describe('parseKeyValueLines', () => {
  it('parses KEY=VALUE lines and trims', () => {
    const r = parseKeyValueLines('A=1\n B = two \n\n')
    expect(r).toEqual({ ok: true, value: { A: '1', B: 'two' } })
  })
  it('empty text becomes an empty map', () => {
    expect(parseKeyValueLines('   ')).toEqual({ ok: true, value: {} })
  })
  it('rejects a line without = or with empty key', () => {
    expect(parseKeyValueLines('nope')).toMatchObject({ ok: false })
    expect(parseKeyValueLines('=v')).toMatchObject({ ok: false })
  })
})

describe('list formatting / parsing', () => {
  it('formatKeyValueMap round trips', () => {
    expect(formatKeyValueMap({ A: '1', B: 'x' })).toBe('A=1\nB=x')
    expect(formatKeyValueMap(undefined)).toBe('')
  })
  it('formatStringList joins lines / empty', () => {
    expect(formatStringList(['a', 'b'])).toBe('a\nb')
    expect(formatStringList(undefined)).toBe('')
  })
  it('parseArgsText splits on whitespace single-line and on newlines multi-line', () => {
    expect(parseArgsText('-y pkg')).toEqual(['-y', 'pkg'])
    expect(parseArgsText('-y\npkg\n')).toEqual(['-y', 'pkg'])
    expect(parseArgsText('  ')).toEqual([])
  })
  it('parseLineList keeps one entry per non-empty line', () => {
    expect(parseLineList('a\n b \n\n')).toEqual(['a', 'b'])
  })
})
