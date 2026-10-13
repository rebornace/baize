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
})
