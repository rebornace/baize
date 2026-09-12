import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { MCP_EXPORTS, mcpExportErrorText } from './strings'

describe('mcpExportErrorText', () => {
  it('maps key not found', () => {
    expect(mcpExportErrorText(new ApiError(404, 'not_found', 'HTTP 404: key not found')).title).toBe(
      MCP_EXPORTS.errKeyNotFound,
    )
  })

  it('maps identity not found', () => {
    expect(
      mcpExportErrorText(new ApiError(404, 'not_found', 'HTTP 404: identity not found')).title,
    ).toBe(MCP_EXPORTS.errIdentityNotFound)
  })

  it('maps internal error to a friendly title and keeps technical detail', () => {
    const out = mcpExportErrorText(new ApiError(500, 'internal_error', 'HTTP 500: boom'))
    expect(out.title).toBe(MCP_EXPORTS.errInternal)
    expect(out.detail).toContain('internal_error')
  })

  it('maps validation messages', () => {
    expect(
      mcpExportErrorText(new ApiError(400, 'invalid_request', 'name is required')).title,
    ).toBe(MCP_EXPORTS.errNameRequired)
    expect(
      mcpExportErrorText(new ApiError(400, 'invalid_request', 'identity_id is required')).title,
    ).toBe(MCP_EXPORTS.errIdentityRequired)
    expect(
      mcpExportErrorText(new ApiError(400, 'invalid_request', 'unknown identity_id')).title,
    ).toBe(MCP_EXPORTS.errIdentityRequired)
    expect(
      mcpExportErrorText(new ApiError(400, 'invalid_request', 'invalid json body')).title,
    ).toBe(MCP_EXPORTS.errInvalidRequest)
  })

  it('falls back to generic title for unknown codes', () => {
    const out = mcpExportErrorText(new ApiError(400, 'weird', 'boom'))
    expect(out.title).toBe(MCP_EXPORTS.errGeneric)
    expect(out.detail).toBe('weird: boom')
  })

  it('falls back to friendlyError for non-ApiError', () => {
    expect(mcpExportErrorText(new Error('failed to fetch')).title).toContain('网络')
  })
})
