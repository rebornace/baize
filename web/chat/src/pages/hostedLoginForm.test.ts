import { describe, expect, it } from 'vitest'
import {
  buildHostedLoginFromDraft,
  formatHostedLoginFields,
  hostedLoginToDraft,
  mergeAuthWithHostedLogin,
  parseHostedLoginFields,
} from './hostedLoginForm'

describe('hostedLoginForm', () => {
  it('round-trips fields text', () => {
    const fields = [
      { name: 'user', arg: 'username', input: 'text', label: '用户名' },
      { name: 'pass', arg: 'password', input: 'password', label: '密码' },
    ]
    const text = formatHostedLoginFields(fields)
    expect(parseHostedLoginFields(text)).toEqual(fields)
  })

  it('builds recipe from draft and merges into auth', () => {
    const draft = hostedLoginToDraft({
      enabled: true,
      mode: 'basic',
      login_tool: '',
      fields: [{ name: 'username', arg: 'username', input: 'text' }],
    })
    const recipe = buildHostedLoginFromDraft(draft)
    expect(recipe.enabled).toBe(true)
    expect(recipe.mode).toBe('basic')
    const auth = mergeAuthWithHostedLogin({ mode: 'static', capture: { tool_name_glob: '*login*' } }, draft)
    expect(auth.capture?.tool_name_glob).toBe('*login*')
    expect(auth.hosted_login?.mode).toBe('basic')
  })

  it('builds oauth recipe', () => {
    const draft = hostedLoginToDraft({
      enabled: true,
      mode: 'oauth',
      oauth: {
        authorize_url: 'https://idp/authorize',
        token_url: 'https://idp/token',
        client_id: 'cid',
        scopes: ['openid'],
      },
    })
    const recipe = buildHostedLoginFromDraft(draft)
    expect(recipe.oauth?.authorize_url).toBe('https://idp/authorize')
    expect(recipe.oauth?.client_id).toBe('cid')
    expect(recipe.oauth?.scopes).toEqual(['openid'])
  })

  it('omits default empty hosted_login on create; clears when previously set', () => {
    const empty = hostedLoginToDraft(undefined)
    const created = mergeAuthWithHostedLogin({ mode: 'static', capture: {} }, empty)
    expect(created).toEqual({ mode: 'static', capture: {} })
    expect(created.hosted_login).toBeUndefined()

    const cleared = mergeAuthWithHostedLogin(
      { mode: 'static', capture: {}, hosted_login: { enabled: true, mode: 'basic' } },
      empty,
    )
    expect(cleared.hosted_login).toEqual({ enabled: false })
  })
})
