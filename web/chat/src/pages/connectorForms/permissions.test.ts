import { describe, expect, it } from 'vitest'
import { emptySelection, selectionFromLists, toggleTool, toNameLists } from './permissions'

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
