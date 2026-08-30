import { describe, expect, it } from 'vitest'
import {
  identityToForm,
  mcpExportEndpointUrl,
  validateIdentityForm,
} from './McpExportSettings'

describe('mcpExportEndpointUrl', () => {
  it('joins origin and path', () => {
    expect(mcpExportEndpointUrl('https://example.com', '/v0/mcp/export')).toBe(
      'https://example.com/v0/mcp/export',
    )
  })

  it('strips trailing slash on origin', () => {
    expect(mcpExportEndpointUrl('https://example.com/', '/v0/mcp/export')).toBe(
      'https://example.com/v0/mcp/export',
    )
  })

  it('ensures leading slash on path', () => {
    expect(mcpExportEndpointUrl('https://example.com', 'v0/mcp/export')).toBe(
      'https://example.com/v0/mcp/export',
    )
  })
})

describe('validateIdentityForm', () => {
  it('requires name', () => {
    expect(validateIdentityForm({ name: '  ', scheme: '', headersText: '' })).toEqual({
      ok: false,
      message: '名称不能为空',
    })
  })

  it('parses headers via parseKeyValueLines', () => {
    expect(
      validateIdentityForm({
        name: 'Ops',
        scheme: 'Bearer',
        headersText: 'X-Team=ops\nAuthorization=Bearer t',
      }),
    ).toEqual({
      ok: true,
      name: 'Ops',
      scheme: 'Bearer',
      headers: { 'X-Team': 'ops', Authorization: 'Bearer t' },
    })
  })

  it('rejects invalid header lines', () => {
    expect(
      validateIdentityForm({ name: 'Ops', scheme: '', headersText: 'no-equals' }),
    ).toEqual({
      ok: false,
      message: '无效键值行：no-equals',
    })
  })
})

describe('identityToForm', () => {
  it('formats headers as KEY=VALUE lines', () => {
    expect(
      identityToForm({
        id: 'mei_1',
        name: 'Ops',
        scheme: 'Bearer',
        headers: { 'X-Team': 'ops' },
      }),
    ).toEqual({
      name: 'Ops',
      scheme: 'Bearer',
      headersText: 'X-Team=ops',
    })
  })
})
