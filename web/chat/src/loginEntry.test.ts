import { describe, expect, it } from 'vitest'
import {
  findLoginRequiredConnector,
  findPendingLoginWait,
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

  it('detects raw 401/expired auth payloads', () => {
    expect(isLoginRequiredContent({ http_status: 401, message: 'x' })).toBe(true)
    expect(
      isLoginRequiredContent({ message: '暂未登录或token已经过期' }),
    ).toBe(true)
    expect(
      isLoginRequiredContent({ message: 'Full authentication is required' }),
    ).toBe(true)
    expect(isLoginRequiredContent({ code: 401, message: 'Unauthorized' })).toBe(true)
  })

  it('finds connector needing login from tool blocks', () => {
    const catalog = [{ name: 'PetsAdminController_findPage', connector_id: 'mall' }]
    expect(
      findLoginRequiredConnector(
        [
          {
            kind: 'tool',
            name: 'PetsAdminController_findPage',
            result: { code: 'login_required', message: 'x' },
          },
        ],
        catalog,
      ),
    ).toBe('mall')
    expect(findLoginRequiredConnector([{ kind: 'tool', name: 'x', result: { ok: 1 } }], catalog)).toBeUndefined()
  })

  it('findPendingLoginWait prefers hitl.waiting kind=login', () => {
    const catalog = [{ name: 'get_pets', connector_id: 'mall' }]
    const wait = findPendingLoginWait(
      [{ kind: 'tool', name: 'get_pets', result: { code: 'login_required' } }],
      catalog,
      [
        { type: 'tool.result', data: { name: 'get_pets' } },
        {
          type: 'hitl.waiting',
          data: { kind: 'login', connector_id: 'mall', tool_name: 'get_pets' },
        },
      ],
    )
    expect(wait).toEqual({ active: true, connectorId: 'mall' })
  })

  it('findPendingLoginWait stays active without catalog connector', () => {
    expect(
      findPendingLoginWait(
        [{ kind: 'tool', name: 'unknown_tool', result: { code: 'login_required' } }],
        [],
      ),
    ).toEqual({ active: true })
  })

  it('findPendingLoginWait clears after login hitl.resumed even if tool still login_required', () => {
    const catalog = [{ name: 'get_pets', connector_id: 'mall' }]
    expect(
      findPendingLoginWait(
        [{ kind: 'tool', name: 'get_pets', result: { code: 'login_required' } }],
        catalog,
        [
          { type: 'hitl.waiting', data: { kind: 'login', connector_id: 'mall' } },
          { type: 'hitl.resumed', data: { decision: 'skip_login', kind: 'login' } },
        ],
      ),
    ).toEqual({ active: false })
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
