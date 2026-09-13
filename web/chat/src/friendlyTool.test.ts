import { describe, expect, it } from 'vitest'
import { friendlyToolName, toolPhrase, type ToolCatalog } from './friendlyTool'

const catalog: ToolCatalog = [
  { name: 'query_order', title: '查询订单', description: '按单号查订单' },
  { name: 'no_title', title: '', description: '' },
]

describe('friendlyToolName', () => {
  it('prefers catalog title, falls back to technical name', () => {
    expect(friendlyToolName('query_order', catalog)).toBe('查询订单')
    expect(friendlyToolName('no_title', catalog)).toBe('no_title')
    expect(friendlyToolName('totally_new', catalog)).toBe('totally_new')
    expect(friendlyToolName('query_order', [])).toBe('query_order')
  })
})

describe('toolPhrase', () => {
  it('builds human phrases per status without fabricating verbs', () => {
    expect(toolPhrase('query_order', 'running', catalog)).toBe('正在处理：查询订单…')
    expect(toolPhrase('query_order', 'waiting_human', catalog)).toBe('待确认：查询订单')
    expect(toolPhrase('query_order', 'succeeded', catalog)).toBe('已完成：查询订单')
    expect(toolPhrase('query_order', 'failed', catalog)).toBe('处理失败：查询订单')
    expect(toolPhrase('query_order', 'approved', catalog)).toBe('已同意：查询订单')
    expect(toolPhrase('query_order', 'rejected', catalog)).toBe('已拒绝：查询订单')
  })
  it('uses technical name in phrase when no title', () => {
    expect(toolPhrase('totally_new', 'running', catalog)).toBe('正在处理：totally_new…')
  })
})
