import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import {
  ACTIONS,
  AUTO_LABEL,
  friendlyError,
  HITL,
  tierLabel,
  VISION_LABEL,
  WELCOME,
} from './strings'

describe('tierLabel', () => {
  it('maps internal tier ids to mainstream labels', () => {
    expect(tierLabel('light')).toBe('快速')
    expect(tierLabel('standard')).toBe('标准')
    expect(tierLabel('power')).toBe('深度思考')
  })
  it('falls back to 标准 for unknown/empty tier', () => {
    expect(tierLabel(undefined)).toBe('标准')
    expect(tierLabel('nope')).toBe('标准')
  })
})

describe('labels', () => {
  it('exposes friendly auto/vision/action/hitl/welcome strings', () => {
    expect(AUTO_LABEL).toBe('智能选择')
    expect(VISION_LABEL).toBe('视觉')
    expect(ACTIONS.copy).toBe('复制')
    expect(ACTIONS.regenerate).toBe('重新回答')
    expect(ACTIONS.editAndReanswer).toBe('编辑后重新回答')
    expect(ACTIONS.forkAsNew).toBe('复制成新对话')
    expect(ACTIONS.rollbackHere).toBe('回到这里')
    expect(HITL.title).toBe('需要你确认')
    expect(HITL.approve).toBe('同意')
    expect(HITL.reject).toBe('拒绝')
    expect(WELCOME.title).toBe('有什么可以帮你？')
  })
})

describe('friendlyError', () => {
  it('maps known ApiError codes to Chinese titles', () => {
    expect(friendlyError(new ApiError(409, 'conversation_busy', 'busy')).title).toContain('稍候')
    expect(friendlyError(new ApiError(400, 'no_model_configured', 'x')).title).toContain('AI 模型')
    expect(friendlyError(new ApiError(400, 'vision_unsupported', 'x')).title).toContain('图片')
    expect(friendlyError(new ApiError(401, 'unauthorized', 'x')).title).toContain('权限')
    // 403 与非码表 401 code 都必须走通用权限文案，防止码表/分支顺序调整后静默退化。
    expect(friendlyError(new ApiError(403, 'forbidden', 'x')).title).toContain('权限')
    expect(friendlyError(new ApiError(401, 'some_other_code', 'x')).title).toContain('权限')
    expect(friendlyError(new ApiError(500, 'internal_error', 'x')).title).toContain('稍后')
    expect(friendlyError(new ApiError(404, 'not_found', 'x')).title).toContain('不存在')
  })
  it('maps invalid_signature explicitly', () => {
    expect(friendlyError(new ApiError(401, 'invalid_signature', 'bad sig')).title).toContain('刷新')
  })
  it('falls back to a generic title with detail for unknown codes', () => {
    const r = friendlyError(new ApiError(418, 'weird_teapot', 'boom'))
    expect(r.title).toBeTruthy()
    expect(r.detail).toContain('weird_teapot')
    expect(r.detail).toContain('boom')
  })
  it('treats fetch/network failures as network errors', () => {
    expect(friendlyError(new TypeError('Failed to fetch')).title).toContain('网络')
    expect(friendlyError(new Error('NetworkError when attempting to fetch resource')).title).toContain('网络')
  })
  it('falls back safely for non-Error values', () => {
    expect(friendlyError('nope').title).toBeTruthy()
  })
})
