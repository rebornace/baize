import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { SKILLS, skillErrorText } from './strings'

describe('SKILLS', () => {
  it('exposes humanized page copy', () => {
    expect(SKILLS.title).toBe('技能')
    expect(SKILLS.description).toMatch(/@|\//)
    expect(SKILLS.upload).toBeTruthy()
    expect(SKILLS.saveDefaults).toBe('保存为默认技能')
    expect(SKILLS.saveDefaultsHint).toBeTruthy()
    expect(SKILLS.emptyTitle).toBeTruthy()
    expect(SKILLS.emptyDesc).toBeTruthy()
    expect(SKILLS.confirmDeleteTitle).toBeTruthy()
    expect(SKILLS.confirmDeleteBody).toBeTruthy()
    expect(SKILLS.confirmDeleteOk).toBe('删除')
    expect(SKILLS.toastUploaded).toBeTruthy()
    expect(SKILLS.toastSaved).toBe('默认技能已保存')
    expect(SKILLS.toastDeleted).toBeTruthy()
    expect(SKILLS.sourceBuiltin).toBe('内置')
    expect(SKILLS.sourceUser).toBe('用户')
    expect(SKILLS.errGeneric).toBeTruthy()
    expect(SKILLS.errNotFound).toBeTruthy()
    expect(SKILLS.errInvalidRequest).toBeTruthy()
    expect(SKILLS.errInternal).toBeTruthy()
  })
})

describe('skillErrorText', () => {
  it('maps not_found', () => {
    expect(skillErrorText(new ApiError(404, 'not_found', 'x')).title).toBe(SKILLS.errNotFound)
  })

  it('maps invalid_request', () => {
    expect(skillErrorText(new ApiError(400, 'invalid_request', 'file is required')).title).toBe(
      SKILLS.errInvalidRequest,
    )
  })

  it('falls back via friendly shape for internal_error', () => {
    const r = skillErrorText(new ApiError(500, 'internal_error', 'boom'))
    expect(r.title).toBe(SKILLS.errInternal)
    expect(r.detail).toMatch(/internal_error/)
  })

  it('uses generic title for unknown api codes', () => {
    const r = skillErrorText(new ApiError(418, 'weird_code', 'nope'))
    expect(r.title).toBe(SKILLS.errGeneric)
    expect(r.detail).toMatch(/weird_code/)
  })
})
