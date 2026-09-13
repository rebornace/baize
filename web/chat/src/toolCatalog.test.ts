import { describe, expect, it } from 'vitest'
import { canDeleteCatalogTool, pathPrefixGroup, toolMatchesQuery, groupToolsTree } from './toolCatalog'
import type { ToolInfo } from './api'

const sample = (over: Partial<ToolInfo> & Pick<ToolInfo, 'name' | 'connector_id'>): ToolInfo => ({
  method: 'GET',
  path: '/x',
  ...over,
})

describe('canDeleteCatalogTool', () => {
  it('only extra', () => {
    expect(canDeleteCatalogTool('extra')).toBe(true)
    expect(canDeleteCatalogTool('spec')).toBe(false)
    expect(canDeleteCatalogTool('plugin')).toBe(false)
  })
})

describe('pathPrefixGroup', () => {
  it('skips api and version segments', () => {
    expect(pathPrefixGroup('/api/v1/tickets/{id}')).toBe('/tickets')
    expect(pathPrefixGroup('/tickets')).toBe('/tickets')
    expect(pathPrefixGroup('/v2/customers')).toBe('/customers')
    expect(pathPrefixGroup(undefined)).toBe('其他')
    expect(pathPrefixGroup('/api/v1')).toBe('其他')
  })
})

describe('toolMatchesQuery', () => {
  it('matches title case-insensitively', () => {
    const t = sample({ name: 'get_ticket', connector_id: 'c', title: '查工单', path: '/tickets/{id}', description: '按 id 取' })
    expect(toolMatchesQuery(t, '查工')).toBe(true)
    expect(toolMatchesQuery(t, 'TICKETS')).toBe(true)
    expect(toolMatchesQuery(t, 'zzz')).toBe(false)
    expect(toolMatchesQuery(t, '  ')).toBe(true)
  })
})

describe('groupToolsTree', () => {
  it('groups by connector then prefix, 其他 last', () => {
    const tools: ToolInfo[] = [
      sample({ name: 'a', connector_id: 'billing', path: '/invoices' }),
      sample({ name: 'b', connector_id: 'ticket', path: '/tickets' }),
      sample({ name: 'c', connector_id: 'ticket', path: '/comments' }),
      sample({ name: 'plug', connector_id: 'ticket', path: undefined, source: 'plugin' }),
    ]
    const tree = groupToolsTree(tools)
    expect(tree.map((g) => g.connectorId)).toEqual(['billing', 'ticket'])
    const ticket = tree[1]
    expect(ticket.prefixes.map((p) => p.prefix)).toEqual(['/comments', '/tickets', '其他'])
    expect(ticket.prefixes[2].tools.map((t) => t.name)).toEqual(['plug'])
  })
})
