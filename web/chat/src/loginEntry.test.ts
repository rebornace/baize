import { describe, expect, it } from 'vitest'
import { filterLoginEntries, fieldsFromEntry } from './loginEntry'

describe('filterLoginEntries', () => {
  it('filters by query against title and tool_name', () => {
    const entries = [
      { id: 'crm/login', title: '登录 · CRM / login', tool_name: 'login', required: [] as string[] },
      { id: 'crm/other', title: '登录 · CRM / other', tool_name: 'other_login', required: ['x'] },
    ]
    expect(filterLoginEntries(entries as any, 'crm').length).toBe(2)
    expect(filterLoginEntries(entries as any, 'other')).toEqual([entries[1]])
  })
})

describe('fieldsFromEntry', () => {
  it('marks sensitive required fields as password inputs', () => {
    const fields = fieldsFromEntry({
      required: ['username', 'password'],
      parameters: {},
    })
    expect(fields).toEqual([
      { name: 'username', type: 'text', required: true },
      { name: 'password', type: 'password', required: true },
    ])
  })
})
