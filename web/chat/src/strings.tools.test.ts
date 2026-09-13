import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { TOOLS, toolErrorText } from './strings'

describe('TOOLS', () => {
  it('exposes humanized page copy', () => {
    expect(TOOLS.title).toBe('助手功能')
    expect(TOOLS.requireLogin).toBe('需登录')
    expect(TOOLS.requireApprovalBadge).toBe('需确认')
    expect(TOOLS.captureIntro).toBeTruthy()
    expect(TOOLS.captureToolGlob).toBe('登录工具名匹配')
    expect(TOOLS.captureToolGlobHint).toMatch(/\*/)
    expect(TOOLS.captureTokenPaths).toBe('令牌 JSON 路径')
    expect(TOOLS.captureLabelPaths).toBe('身份显示名路径')
    expect(TOOLS.captureHeaderTemplate).toBe('下游请求头模板')
    expect(TOOLS.captureDefaultScheme).toBe('默认认证方案')
    expect(TOOLS.advanced).toBe('高级')
    expect(TOOLS.addTool).toBe('添加')
    expect(TOOLS.addModalTitle).toBe('添加工具')
    expect(TOOLS.fieldConnector).toBe('所属连接')
    expect(TOOLS.fieldMethod).toBe('请求方法')
    expect(TOOLS.fieldPath).toBe('路径')
    expect(TOOLS.fieldSchema).toBe('参数说明')
    expect(TOOLS.toastAdded).toBe('功能已添加')
    expect(TOOLS.viewSchema).toBe('查看参数说明')
    expect(TOOLS.confirmDeleteTitle).toBe('删除这个功能？')
    expect(TOOLS.confirmDeleteOk).toBe('删除')
  })
})

describe('toolErrorText', () => {
  it('maps not_found', () => {
    expect(toolErrorText(new ApiError(404, 'not_found', 'x')).title).toBe(TOOLS.errNotFound)
  })

  it('maps invalid_request', () => {
    expect(toolErrorText(new ApiError(400, 'invalid_request', 'name is required')).title).toBe(
      TOOLS.errInvalidRequest,
    )
  })

  it('falls back via friendly shape for internal_error', () => {
    const r = toolErrorText(new ApiError(500, 'internal_error', 'boom'))
    expect(r.title).toBe(TOOLS.errInternal)
    expect(r.detail).toMatch(/internal_error/)
  })

  it('uses generic title for unknown api codes', () => {
    const r = toolErrorText(new ApiError(418, 'weird_code', 'nope'))
    expect(r.title).toBe(TOOLS.errGeneric)
    expect(r.detail).toMatch(/weird_code/)
  })
})
