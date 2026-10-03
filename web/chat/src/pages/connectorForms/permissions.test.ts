import { describe, expect, it } from 'vitest'
import {
  emptySelection,
  filterPermissionTools,
  selectionFromLists,
  toggleTool,
  toNameLists,
  toPermissionTools,
} from './permissions'

const tools = [
  { name: 'login' },
  { name: 'list_tickets' },
  { name: 'create_ticket' },
]

describe('selection helpers', () => {
  it('builds selection from existing name lists', () => {
    const sel = selectionFromLists(tools, ['login'], ['create_ticket'])
    expect(sel.login.login).toBe(true)
    expect(sel.login.approval).toBe(false)
    expect(sel.create_ticket.approval).toBe(true)
  })
  it('toggles a flag immutably', () => {
    const sel = emptySelection(tools)
    const next = toggleTool(sel, 'login', 'login')
    expect(sel.login.login).toBe(false)
    expect(next.login.login).toBe(true)
  })
  it('converts back to sorted name lists', () => {
    let sel = selectionFromLists(tools, ['create_ticket', 'login'], ['login'])
    sel = toggleTool(sel, 'list_tickets', 'approval')
    const { loginNames, approvalNames } = toNameLists(sel)
    expect(loginNames).toEqual(['create_ticket', 'login'])
    expect(approvalNames).toEqual(['list_tickets', 'login'])
  })
})

describe('permission tool display helpers', () => {
  it('toPermissionTools trims empty title/description', () => {
    expect(
      toPermissionTools([
        { name: 'a', title: ' 查工单 ', description: '  ' },
        { name: 'b', title: '', description: '列出' },
      ]),
    ).toEqual([
      { name: 'a', title: '查工单', description: undefined },
      { name: 'b', title: undefined, description: '列出' },
    ])
  })

  it('filterPermissionTools matches name title description', () => {
    const list = toPermissionTools([
      { name: 'tickets.list', title: '查工单', description: '按条件列出' },
      { name: 'login', title: '登录', description: '获取令牌' },
    ])
    expect(filterPermissionTools(list, '工单').map((t) => t.name)).toEqual(['tickets.list'])
    expect(filterPermissionTools(list, 'login').map((t) => t.name)).toEqual(['login'])
    expect(filterPermissionTools(list, '令牌').map((t) => t.name)).toEqual(['login'])
    expect(filterPermissionTools(list, '  ').length).toBe(2)
    expect(filterPermissionTools(list, 'nope')).toEqual([])
  })
})
