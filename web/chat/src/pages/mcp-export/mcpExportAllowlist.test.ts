import { describe, expect, it } from 'vitest'
import type { ToolInfo } from '../../api'
import {
  isReadOnlyExportCandidate,
  mergeAllowedToolSelection,
  readOnlyExportCandidateNames,
  selectAllConnectorTools,
} from './mcpExportAllowlist'

const base = (over: Partial<ToolInfo>): ToolInfo => ({
  name: 'tool',
  connector_id: 'hr',
  ...over,
})

describe('isReadOnlyExportCandidate', () => {
  it('accepts GET/HEAD when not force_deny', () => {
    expect(isReadOnlyExportCandidate(base({ name: 'a', method: 'GET' }))).toBe(true)
    expect(isReadOnlyExportCandidate(base({ name: 'b', method: 'head' }))).toBe(true)
  })

  it('rejects force_deny and non-read methods', () => {
    expect(isReadOnlyExportCandidate(base({ method: 'GET', export: 'force_deny' }))).toBe(false)
    expect(isReadOnlyExportCandidate(base({ method: 'POST' }))).toBe(false)
    expect(isReadOnlyExportCandidate(base({ method: '' }))).toBe(false)
  })

  it('skips write signal names by default', () => {
    expect(isReadOnlyExportCandidate(base({ name: 'db.insert_row', method: 'GET' }))).toBe(false)
    expect(
      isReadOnlyExportCandidate(base({ name: 'query', method: 'GET', description: 'run update' })),
    ).toBe(false)
  })
})

describe('readOnlyExportCandidateNames', () => {
  it('collects matching tool names', () => {
    const tools = [
      base({ name: 'get_ok', method: 'GET' }),
      base({ name: 'post_no', method: 'POST' }),
      base({ name: 'deny', method: 'GET', export: 'force_deny' }),
    ]
    expect(readOnlyExportCandidateNames(tools)).toEqual(['get_ok'])
  })
})

describe('mergeAllowedToolSelection', () => {
  it('preserves prior order then appends new picks in catalog order', () => {
    const selected = new Set(['b', 'c'])
    expect(mergeAllowedToolSelection(['a', 'b'], selected, ['a', 'b', 'c', 'd'])).toEqual(['b', 'c'])
  })
})

describe('selectAllConnectorTools', () => {
  it('adds all tools for a connector', () => {
    const tools = [
      base({ name: 'a', connector_id: 'hr' }),
      base({ name: 'b', connector_id: 'hr' }),
      base({ name: 'c', connector_id: 'ops' }),
    ]
    const next = selectAllConnectorTools(tools, 'hr', new Set(['c']))
    expect([...next].sort()).toEqual(['a', 'b', 'c'])
  })
})
