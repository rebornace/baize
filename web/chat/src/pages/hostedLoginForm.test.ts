import { describe, expect, it } from 'vitest'
import {
  buildHostedLoginFromDraft,
  formatHostedLoginFields,
  hostedLoginToDraft,
  mergeAuthWithHostedLogin,
  parseHostedLoginFields,
  patchPrimaryMethodDraft,
  primaryMethodDraft,
} from './hostedLoginForm'

describe('hostedLoginForm', () => {
  it('round-trips methods config', () => {
    const cfg = {
      enabled: true,
      default_method_id: 'pwd',
      methods: [{
        id: 'pwd',
        label: '密码',
        mode: 'basic' as const,
        role: 'admin',
        identity_ids: ['mei_u', 'mei_a'],
      }],
    }
    const draft = hostedLoginToDraft(cfg)
    const built = buildHostedLoginFromDraft(draft)
    expect(built.default_method_id).toBe('pwd')
    expect(built.methods?.[0]?.role).toBe('admin')
    expect(built.methods?.[0]?.identity_ids).toEqual(['mei_u', 'mei_a'])
  })

  it('omits empty role on build; draft keeps blank for runtime fallback to id', () => {
    const draft = hostedLoginToDraft({
      enabled: true,
      default_method_id: 'pwd',
      methods: [{ id: 'pwd', mode: 'basic' }],
    })
    expect(draft.methods[0]?.role).toBe('')
    expect(buildHostedLoginFromDraft(draft).methods?.[0]?.role).toBeUndefined()
  })

  it('round-trips fields text', () => {
    const fields = [
      { name: 'user', arg: 'username', input: 'text', label: '用户名' },
      { name: 'pass', arg: 'password', input: 'password', label: '密码' },
    ]
    const text = formatHostedLoginFields(fields)
    expect(parseHostedLoginFields(text)).toEqual(fields)
  })

  it('builds config from draft and merges into auth', () => {
    const draft = hostedLoginToDraft({
      enabled: true,
      default_method_id: 'basic',
      methods: [
        {
          id: 'basic',
          mode: 'basic',
          login_tool: '',
          fields: [{ name: 'username', arg: 'username', input: 'text' }],
        },
      ],
    })
    const config = buildHostedLoginFromDraft(draft)
    expect(config.enabled).toBe(true)
    expect(config.methods?.[0]?.mode).toBe('basic')
    const auth = mergeAuthWithHostedLogin({ mode: 'static', capture: { tool_name_glob: '*login*' } }, draft)
    expect(auth.capture?.tool_name_glob).toBe('*login*')
    expect(auth.hosted_login?.methods?.[0]?.mode).toBe('basic')
  })

  it('builds oauth method on config', () => {
    const draft = hostedLoginToDraft({
      enabled: true,
      default_method_id: 'oauth',
      methods: [
        {
          id: 'oauth',
          mode: 'oauth',
          oauth: {
            authorize_url: 'https://idp/authorize',
            token_url: 'https://idp/token',
            client_id: 'cid',
            scopes: ['openid'],
          },
        },
      ],
    })
    const config = buildHostedLoginFromDraft(draft)
    expect(config.methods?.[0]?.oauth?.authorize_url).toBe('https://idp/authorize')
    expect(config.methods?.[0]?.oauth?.client_id).toBe('cid')
    expect(config.methods?.[0]?.oauth?.scopes).toEqual(['openid'])
  })

  it('patches primary method draft for legacy editor fields', () => {
    const draft = hostedLoginToDraft(undefined)
    expect(primaryMethodDraft(draft).id).toBe('default')
    const patched = patchPrimaryMethodDraft(draft, { mode: 'token_paste', loginTool: 'login' })
    const next = { ...draft, ...patched }
    expect(primaryMethodDraft(next).mode).toBe('token_paste')
    expect(primaryMethodDraft(next).loginTool).toBe('login')
  })

  it('omits default empty hosted_login on create; clears when previously set', () => {
    const empty = hostedLoginToDraft(undefined)
    const created = mergeAuthWithHostedLogin({ mode: 'static', capture: {} }, empty)
    expect(created).toEqual({ mode: 'static', capture: {} })
    expect(created.hosted_login).toBeUndefined()

    const cleared = mergeAuthWithHostedLogin(
      {
        mode: 'static',
        capture: {},
        hosted_login: {
          enabled: true,
          default_method_id: 'basic',
          methods: [{ id: 'basic', mode: 'basic' }],
        },
      },
      empty,
    )
    expect(cleared.hosted_login).toEqual({ enabled: false })
  })
})
