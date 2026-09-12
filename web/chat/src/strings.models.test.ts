import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { MODELS, modelErrorText } from './strings'

describe('MODELS', () => {
  it('exposes humanized page copy', () => {
    expect(MODELS.title).toBe('模型')
    expect(MODELS.add).toBe('添加模型')
    expect(MODELS.advanced).toBe('高级')
    expect(MODELS.fieldBaseUrl).toBe('服务地址')
    expect(MODELS.fieldDisableThinking).toBe('禁用思考')
  })
})

describe('modelErrorText', () => {
  it('maps not_found', () => {
    expect(modelErrorText(new ApiError(404, 'not_found', 'x')).title).toBe(MODELS.errNotFound)
  })
  it('maps invalid_request', () => {
    expect(modelErrorText(new ApiError(400, 'invalid_request', 'name is required')).title).toBe(
      MODELS.errInvalidRequest,
    )
  })
  it('falls back via friendly shape for unknown', () => {
    const r = modelErrorText(new ApiError(500, 'internal_error', 'boom'))
    expect(r.title).toBe(MODELS.errInternal)
    expect(r.detail).toMatch(/internal_error/)
  })
})
