import { describe, expect, it } from 'vitest'
import type { ToolInfo } from '../api'
import { defaultExpandedSets, prefixExpandKey } from './ToolsSettings'

const sample = (over: Partial<ToolInfo> & Pick<ToolInfo, 'name' | 'connector_id'>): ToolInfo => ({
  method: 'GET',
  path: '/x',
  ...over,
})

describe('defaultExpandedSets', () => {
  it('expands unique connector and all its prefixes', () => {
    const tools: ToolInfo[] = [
      sample({ name: 'list', connector_id: 'ticket', path: '/tickets' }),
      sample({ name: 'get', connector_id: 'ticket', path: '/tickets/{id}' }),
      sample({ name: 'comment', connector_id: 'ticket', path: '/comments' }),
      sample({ name: 'plug', connector_id: 'ticket', path: undefined }),
    ]
    const { connectors, prefixes } = defaultExpandedSets(tools)
    expect([...connectors]).toEqual(['ticket'])
    expect([...prefixes].sort()).toEqual(
      [
        prefixExpandKey('ticket', '/comments'),
        prefixExpandKey('ticket', '/tickets'),
        prefixExpandKey('ticket', '其他'),
      ].sort(),
    )
  })

  it('keeps multiple connectors and prefixes collapsed', () => {
    const tools: ToolInfo[] = [
      sample({ name: 'a', connector_id: 'billing', path: '/invoices' }),
      sample({ name: 'b', connector_id: 'ticket', path: '/tickets' }),
    ]
    const { connectors, prefixes } = defaultExpandedSets(tools)
    expect(connectors.size).toBe(0)
    expect(prefixes.size).toBe(0)
  })

  it('returns empty sets for empty catalog', () => {
    const { connectors, prefixes } = defaultExpandedSets([])
    expect(connectors.size).toBe(0)
    expect(prefixes.size).toBe(0)
  })
})
