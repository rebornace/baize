import { describe, expect, it } from 'vitest'
import type { ConnectorInfo, ToolInfo } from '../../api'
import {
  connectorToMcpForm, mcpConnectorIds, mcpSummary, validateMcp, type McpFormValues,
} from './mcp'

const stdio = (over: Partial<McpFormValues> = {}): McpFormValues => ({
  id: 'analytics', transport: 'stdio', command: 'npx', argsText: '-y srv', envText: 'A=1',
  url: '', headersText: '', exportDbReadonly: false,
  oauthClientId: '', oauthClientSecret: '', oauthStatus: '', ...over,
})
const http = (over: Partial<McpFormValues> = {}): McpFormValues => ({
  id: 'remote', transport: 'http', command: '', argsText: '', envText: '',
  url: 'https://mcp.example.com', headersText: 'Authorization=Bearer t', exportDbReadonly: false,
  oauthClientId: '', oauthClientSecret: '', oauthStatus: '', ...over,
})

describe('validateMcp', () => {
  it('accepts a stdio form and builds the mcp config', () => {
    const r = validateMcp(stdio())
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.id).toBe('analytics')
      expect(r.mcp).toEqual({ transport: 'stdio', command: 'npx', args: ['-y', 'srv'], env: { A: '1' } })
    }
  })
  it('omits empty env map', () => {
    const r = validateMcp(stdio({ envText: '  ' }))
    if (r.ok) expect(r.mcp.env).toBeUndefined()
  })
  it('requires id pattern', () => {
    const r = validateMcp(stdio({ id: 'Bad Id' }))
    if (!r.ok) expect(r.fieldErrors.id).toBeTruthy()
    else throw new Error('expected failure')
  })
  it('requires stdio command', () => {
    const r = validateMcp(stdio({ command: '  ' }))
    if (!r.ok) expect(r.fieldErrors.command).toBeTruthy()
    else throw new Error('expected failure')
  })
  it('flags bad env line on the env field', () => {
    const r = validateMcp(stdio({ envText: 'nope' }))
    if (!r.ok) expect(r.fieldErrors.env).toContain('nope')
    else throw new Error('expected failure')
  })
  it('accepts http form with headers', () => {
    const r = validateMcp(http())
    if (r.ok) expect(r.mcp).toEqual({ transport: 'http', url: 'https://mcp.example.com', headers: { Authorization: 'Bearer t' } } )
    else throw new Error('expected success')
  })
  it('requires http url and http(s) scheme', () => {
    const a = validateMcp(http({ url: '' }))
    const b = validateMcp(http({ url: 'ftp://x' }))
    if (!a.ok) expect(a.fieldErrors.url).toBeTruthy()
    if (!b.ok) expect(b.fieldErrors.url).toBeTruthy()
  })
  it('flags bad headers line', () => {
    const r = validateMcp(http({ headersText: 'noequals' }))
    if (!r.ok) expect(r.fieldErrors.headers).toContain('noequals')
    else throw new Error('expected failure')
  })
  it('includes export_db_readonly only when checked', () => {
    const off = validateMcp(stdio({ exportDbReadonly: false }))
    const on = validateMcp(stdio({ exportDbReadonly: true }))
    if (off.ok) expect(off.mcp.export_db_readonly).toBeUndefined()
    else throw new Error('expected success')
    if (on.ok) expect(on.mcp.export_db_readonly).toBe(true)
    else throw new Error('expected success')
  })
  it('http: attaches oauth.client_id when provided', () => {
    const r = validateMcp(http({ oauthClientId: 'cid-1' }))
    if (!r.ok) throw new Error('expected success')
    expect(r.mcp.oauth).toEqual({ client_id: 'cid-1' })
    expect(r.mcp.oauth).not.toHaveProperty('token_bundle')
  })
  it('http: sends client_secret only when typed; never token_bundle', () => {
    const r = validateMcp(http({
      oauthClientId: 'cid-1',
      oauthClientSecret: 's3cret',
      oauthStatus: 'authorized',
    }))
    if (!r.ok) throw new Error('expected success')
    expect(r.mcp.oauth).toMatchObject({
      client_id: 'cid-1',
      client_secret: 's3cret',
      status: 'authorized',
    })
    expect(Object.keys(r.mcp.oauth!)).not.toContain('token_bundle')
  })
  it('stdio: never attaches oauth even if form fields set', () => {
    const r = validateMcp(stdio({ oauthClientId: 'cid', oauthClientSecret: 'x' }))
    if (!r.ok) throw new Error('expected success')
    expect(r.mcp.oauth).toBeUndefined()
  })
})

describe('mcpSummary', () => {
  it('summarizes stdio command and http url', () => {
    expect(mcpSummary({ transport: 'stdio', command: 'npx', args: ['x'] })).toBe('本地程序 · npx')
    expect(mcpSummary({ transport: 'http', url: 'https://h/mcp' })).toBe('远程服务 · https://h/mcp')
  })
  it('falls back gracefully', () => {
    expect(mcpSummary(undefined)).toBe('—')
  })
})

describe('connectorToMcpForm', () => {
  it('maps a connector into editable text fields', () => {
    const c: ConnectorInfo = {
      id: 'a', type: 'mcp',
      mcp: { transport: 'http', url: 'https://h', headers: { K: 'V' } },
      require_approval: ['t1'],
    }
    const f = connectorToMcpForm(c)
    expect(f).toMatchObject({ id: 'a', transport: 'http', url: 'https://h', headersText: 'K=V', exportDbReadonly: false })
  })
  it('echoes export_db_readonly', () => {
    const c: ConnectorInfo = {
      id: 'db', type: 'mcp',
      mcp: { transport: 'stdio', command: 'npx', export_db_readonly: true },
    }
    expect(connectorToMcpForm(c).exportDbReadonly).toBe(true)
  })
  it('maps oauth.client_id and status but never echoes secret or token_bundle', () => {
    const c: ConnectorInfo = {
      id: 'o', type: 'mcp',
      mcp: {
        transport: 'http', url: 'https://h',
        oauth: {
          status: 'needs_reauth',
          client_id: 'cid',
          client_secret: 'should-not-echo',
          token_bundle: 'bz1:sealed',
          token_endpoint: 'https://auth/token',
        },
      },
    }
    const f = connectorToMcpForm(c)
    expect(f.oauthClientId).toBe('cid')
    expect(f.oauthStatus).toBe('needs_reauth')
    expect(f.oauthClientSecret).toBe('')
    expect(f.oauthTokenEndpoint).toBe('https://auth/token')
  })
})

describe('mcpConnectorIds', () => {
  it('keeps mcp connector ids, deduped and ordered', () => {
    const tools = [
      { name: 'a', source: 'mcp', connector_id: 'c1' },
      { name: 'b', source: 'mcp', connector_id: 'c1' },
      { name: 'c', source: 'plugin', connector_id: 'c9' },
      { name: 'd', source: 'mcp', connector_id: 'c2' },
    ] as unknown as ToolInfo[]
    expect(mcpConnectorIds(tools)).toEqual(['c1', 'c2'])
  })
})
