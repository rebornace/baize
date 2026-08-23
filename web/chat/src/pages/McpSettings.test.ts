import { describe, expect, it } from 'vitest'
import {
  formatKeyValueMap,
  mcpConnectorIds,
  parseArgsText,
  parseKeyValueLines,
  parseLineList,
  validateMcpForm,
  type McpFormState,
} from './McpSettings'

describe('mcpConnectorIds', () => {
  it('dedupes mcp connector ids in order', () => {
    const ids = mcpConnectorIds([
      { name: 'a', connector_id: 'm1', source: 'mcp' },
      { name: 'b', connector_id: 'm1', source: 'mcp' },
      { name: 'c', connector_id: 'm2', source: 'mcp' },
      { name: 'd', connector_id: 'x', source: 'plugin' },
    ])
    expect(ids).toEqual(['m1', 'm2'])
  })
})

describe('parseKeyValueLines', () => {
  it('parses KEY=VALUE lines', () => {
    const parsed = parseKeyValueLines('A=1\nB=two')
    expect(parsed).toEqual({ ok: true, value: { A: '1', B: 'two' } })
  })

  it('rejects invalid lines', () => {
    const parsed = parseKeyValueLines('badline')
    expect(parsed.ok).toBe(false)
  })
})

describe('parseArgsText', () => {
  it('splits by whitespace or newlines', () => {
    expect(parseArgsText('-y @pkg')).toEqual(['-y', '@pkg'])
    expect(parseArgsText('one\ntwo')).toEqual(['one', 'two'])
  })
})

describe('parseLineList', () => {
  it('trims and drops empty lines', () => {
    expect(parseLineList(' a \n\nb ')).toEqual(['a', 'b'])
  })
})

describe('formatKeyValueMap', () => {
  it('round-trips through parseKeyValueLines', () => {
    const text = formatKeyValueMap({ K: 'v', X: 'y' })
    expect(parseKeyValueLines(text)).toEqual({ ok: true, value: { K: 'v', X: 'y' } })
  })
})

describe('validateMcpForm', () => {
  const stdioBase: McpFormState = {
    id: 'db',
    transport: 'stdio',
    command: 'npx',
    argsText: '-y @bytebase/dbhub',
    envText: 'DSN=postgres://localhost',
    url: '',
    headersText: '',
    requireApprovalText: 'write\n',
  }

  it('accepts valid stdio form', () => {
    const result = validateMcpForm(stdioBase)
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.mcp.transport).toBe('stdio')
      expect(result.mcp.command).toBe('npx')
      expect(result.requireApproval).toEqual(['write'])
    }
  })

  it('requires id and command', () => {
    expect(validateMcpForm({ ...stdioBase, id: '' }).ok).toBe(false)
    expect(validateMcpForm({ ...stdioBase, command: '' }).ok).toBe(false)
  })

  it('requires http url', () => {
    const result = validateMcpForm({
      ...stdioBase,
      id: 'remote',
      transport: 'http',
      url: '',
    })
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.code).toBe('invalid_request')
    }
  })
})
