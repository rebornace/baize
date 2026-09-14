import { describe, expect, it } from 'vitest'
import {
  filterLoginEntries,
  fieldsFromEntry,
  isLoginRequiredContent,
  loginPickerEntriesForConnector,
  resolveConnectorId,
} from './loginEntry'

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

describe('isLoginRequiredContent', () => {
  it('detects login_required code on tool result content', () => {
    expect(isLoginRequiredContent({ code: 'login_required', message: '此工具需要先登录' })).toBe(
      true,
    )
    expect(isLoginRequiredContent({ code: 'other' })).toBe(false)
    expect(isLoginRequiredContent(null)).toBe(false)
    expect(isLoginRequiredContent('login_required')).toBe(false)
  })
})

describe('loginPickerEntriesForConnector', () => {
  const entries = [
    {
      id: 'crm/login',
      connector_id: 'crm',
      title: 'CRM login',
      tool_name: 'login',
      required: [] as string[],
    },
    {
      id: 'hr/login',
      connector_id: 'hr',
      title: 'HR login',
      tool_name: 'login',
      required: [] as string[],
    },
  ]

  it('returns only matching connector entries when id is present', () => {
    expect(loginPickerEntriesForConnector(entries as any, 'crm')).toEqual([entries[0]])
    expect(loginPickerEntriesForConnector(entries as any, ' hr ')).toEqual([entries[1]])
  })

  it('never falls back to all connectors when connector_id is missing or blank', () => {
    expect(loginPickerEntriesForConnector(entries as any, undefined)).toEqual([])
    expect(loginPickerEntriesForConnector(entries as any, null)).toEqual([])
    expect(loginPickerEntriesForConnector(entries as any, '')).toEqual([])
    expect(loginPickerEntriesForConnector(entries as any, '   ')).toEqual([])
  })
})

describe('resolveConnectorId', () => {
  it('trims and rejects blank ids', () => {
    expect(resolveConnectorId('crm')).toBe('crm')
    expect(resolveConnectorId('  crm  ')).toBe('crm')
    expect(resolveConnectorId('')).toBeUndefined()
    expect(resolveConnectorId('   ')).toBeUndefined()
    expect(resolveConnectorId(undefined)).toBeUndefined()
    expect(resolveConnectorId(null)).toBeUndefined()
  })
})
