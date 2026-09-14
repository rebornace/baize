import { describe, expect, it } from 'vitest'
import {
  isLoginRequiredContent,
  loginSkillID,
  normalizeConnectorID,
  resolveConnectorId,
} from './loginEntry'

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

describe('normalizeConnectorID / loginSkillID', () => {
  it('matches backend [a-zA-Z0-9_-] normalization', () => {
    expect(normalizeConnectorID('crm')).toBe('crm')
    expect(normalizeConnectorID('a/b')).toBe('ab')
    expect(normalizeConnectorID('my.conn_1-x')).toBe('myconn_1-x')
    expect(normalizeConnectorID('///')).toBe('')
    expect(loginSkillID('crm')).toBe('login-crm')
    expect(loginSkillID('a/b')).toBe('login-ab')
    expect(loginSkillID('///')).toBeUndefined()
    expect(loginSkillID('  a/b  ')).toBe('login-ab')
  })
})
