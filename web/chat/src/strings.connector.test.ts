import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { CONNECTORS, connectorErrorText, permissionSummary } from './strings'

describe('connectorErrorText', () => {
  it('maps known connector codes', () => {
    expect(connectorErrorText(new ApiError(400, 'invalid_spec', 'x')).title).toContain('接口文档无法解析')
    expect(connectorErrorText(new ApiError(400, 'invalid_spec_url', 'x')).title).toContain('文档链接格式')
    expect(connectorErrorText(new ApiError(400, 'spec_fetch_blocked', 'x')).title).toContain('安全策略')
    expect(connectorErrorText(new ApiError(400, 'spec_fetch_failed', 'x')).title).toContain('无法下载')
    expect(connectorErrorText(new ApiError(400, 'unsupported_import_format', 'x')).title).toContain('不支持的文档格式')
    expect(connectorErrorText(new ApiError(400, 'invalid_plugin', 'x')).title).toContain('插件')
    expect(connectorErrorText(new ApiError(409, 'tool_conflict', 'x')).title).toContain('重名')
    expect(connectorErrorText(new ApiError(400, 'invalid_auth', 'x')).title).toContain('凭证')
  })

  it('maps known plain validation messages', () => {
    expect(connectorErrorText(new ApiError(400, 'invalid_request', 'base_url is required')).title).toContain('服务地址')
    expect(connectorErrorText(new ApiError(400, 'invalid_request', 'spec is required')).title).toContain('接口文档')
  })

  it('maps invalid_mcp to a human title', () => {
    expect(connectorErrorText(new ApiError(400, 'invalid_mcp', 'x')).title).toBe(CONNECTORS.errMcpConnect)
  })

  it('maps reachable invalid_mcp backend messages', () => {
    expect(connectorErrorText(new ApiError(400, 'invalid_mcp', 'invalid_mcp: mcp.command is required')).title).toContain('连接配置不完整')
    expect(connectorErrorText(new ApiError(400, 'invalid_mcp', 'invalid_mcp: unsupported mcp transport: ws')).title).toContain('连接配置不完整')
    expect(connectorErrorText(new ApiError(400, 'invalid_mcp', 'invalid_mcp: mcp.url is required')).title).toContain('远程服务地址')
  })

  it('falls back to a generic title for unknown codes', () => {
    const out = connectorErrorText(new ApiError(400, 'weird_code', 'boom'))
    expect(out.title).toBe(CONNECTORS.errorGeneric)
    expect(out.detail).toBe('weird_code: boom')
  })
})

describe('permissionSummary', () => {
  it('renders nothing when no tools gated', () => {
    expect(permissionSummary([], [])).toBeNull()
  })
  it('renders login and approval counts', () => {
    expect(permissionSummary(['a', 'b'], ['c'])).toBe('2 个工具需本人登录 · 1 个需审批')
  })
  it('renders login only', () => {
    expect(permissionSummary(['a'], [])).toBe('1 个工具需本人登录')
  })
})
