import { describe, expect, it } from 'vitest'
import {
  buildCaptureFromDraft,
  captureSummaryLabel,
  captureToDraft,
  connectorSupportsLoginCapture,
  mergeAuthPreserveCapture,
  mergeAuthWithCapture,
  parsePathLines,
} from './captureForm'

describe('captureForm', () => {
  it('captureToDraft round-trip', () => {
    const draft = captureToDraft({
      tool_name_glob: '*login*',
      token_json_paths: ['accessToken'],
      label_json_paths: ['email'],
      header_template: 'Bearer {{token}}',
      default_scheme: 'bearer',
    })
    const built = buildCaptureFromDraft(draft)
    expect(built?.tool_name_glob).toBe('*login*')
    expect(built?.token_json_paths).toEqual(['accessToken'])
    expect(built?.default_scheme).toBe('bearer')
  })

  it('parsePathLines skips blanks', () => {
    expect(parsePathLines('a\n\n b ')).toEqual(['a', 'b'])
  })

  it('mergeAuthWithCapture preserves static headers', () => {
    const merged = mergeAuthWithCapture(
      {
        mode: 'static',
        static: { headers: { Authorization: '${TOKEN}' } },
      },
      { toolNameGlob: '__none__', tokenPathsText: '', labelPathsText: '', headerTemplate: '', defaultScheme: '' },
    )
    expect(merged.static?.headers?.Authorization).toBe('${TOKEN}')
    expect(merged.capture?.tool_name_glob).toBe('__none__')
  })
})

describe('connectorSupportsLoginCapture', () => {
  it('allows openapi and http only', () => {
    expect(connectorSupportsLoginCapture('openapi')).toBe(true)
    expect(connectorSupportsLoginCapture('http')).toBe(true)
    expect(connectorSupportsLoginCapture('mcp')).toBe(false)
    expect(connectorSupportsLoginCapture(undefined)).toBe(false)
  })
})

describe('captureSummaryLabel', () => {
  it('returns null when capture empty', () => {
    expect(captureSummaryLabel(undefined)).toBeNull()
    expect(captureSummaryLabel({})).toBeNull()
  })

  it('labels disabled capture', () => {
    expect(captureSummaryLabel({ tool_name_glob: '__none__' })).toBe('捕获已关闭')
  })

  it('labels custom glob', () => {
    expect(captureSummaryLabel({ tool_name_glob: 'custom_*' })).toBe('捕获 custom_*')
  })

  it('labels default when only paths set', () => {
    expect(captureSummaryLabel({ token_json_paths: ['accessToken'] })).toBe('捕获（默认 *login*）')
  })
})

describe('mergeAuthPreserveCapture', () => {
  it('copies capture from existing auth onto built auth', () => {
    const built = { mode: 'static' as const, static: { headers: { Authorization: 'Bearer x' } } }
    const existing = {
      mode: 'static' as const,
      capture: { tool_name_glob: 'custom_*', token_json_paths: ['accessToken'] },
    }
    const merged = mergeAuthPreserveCapture(built, existing)
    expect(merged.capture?.tool_name_glob).toBe('custom_*')
    expect(merged.static?.headers?.Authorization).toBe('Bearer x')
  })

  it('preserves __none__ disable flag', () => {
    const merged = mergeAuthPreserveCapture(
      { mode: 'static' },
      { mode: 'static', capture: { tool_name_glob: '__none__' } },
    )
    expect(merged.capture?.tool_name_glob).toBe('__none__')
  })

  it('does not attach capture when existing had none', () => {
    const merged = mergeAuthPreserveCapture({ mode: 'static' }, { mode: 'static' })
    expect(merged.capture).toBeUndefined()
  })
})
