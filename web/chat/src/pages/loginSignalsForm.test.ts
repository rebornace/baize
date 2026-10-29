import { describe, expect, it } from 'vitest'
import {
  buildLoginSignalsFromDraft,
  loginSignalsToDraft,
  mergeAuthWithAdvanced,
  parseIntLines,
} from './loginSignalsForm'
import { EMPTY_CAPTURE_DRAFT } from './captureForm'

describe('loginSignalsForm', () => {
  it('parses statuses from lines and commas', () => {
    expect(parseIntLines('401\n403, 400')).toEqual([401, 403, 400])
    expect(parseIntLines('99\nabc\n200')).toEqual([200])
  })

  it('round-trips draft', () => {
    const draft = loginSignalsToDraft({
      http_statuses: [401, 403],
      http_statuses_need_signal: [400, 404],
      body_code_path: 'data.code',
      body_codes: ['401', 'AUTH'],
      message_contains: ['未登录'],
    })
    expect(buildLoginSignalsFromDraft(draft)).toEqual({
      http_statuses: [401, 403],
      http_statuses_need_signal: [400, 404],
      body_code_path: 'data.code',
      body_codes: ['401', 'AUTH'],
      message_contains: ['未登录'],
    })
  })

  it('moves 400/404 from alone statuses into need-signal', () => {
    expect(
      buildLoginSignalsFromDraft({
        httpStatusesText: '400\n401\n403\n404',
        httpStatusesNeedSignalText: '',
        bodyCodePath: '',
        bodyCodesText: '',
        messageContainsText: '',
      }),
    ).toEqual({
      http_statuses: [401, 403],
      http_statuses_need_signal: [400, 404],
    })
  })

  it('mergeAuthWithAdvanced includes login_signals on PUT auth', () => {
    const auth = mergeAuthWithAdvanced(
      { mode: 'static', capture: { tool_name_glob: '*login*' } },
      { ...EMPTY_CAPTURE_DRAFT, toolNameGlob: '*login*' },
      {
        httpStatusesText: '401',
        httpStatusesNeedSignalText: '400\n404',
        bodyCodePath: 'code',
        bodyCodesText: '401',
        messageContainsText: '未登录',
      },
    )
    expect(auth.login_signals).toEqual({
      http_statuses: [401],
      http_statuses_need_signal: [400, 404],
      body_code_path: 'code',
      body_codes: ['401'],
      message_contains: ['未登录'],
    })
    expect(auth.capture?.tool_name_glob).toBe('*login*')
  })
})
