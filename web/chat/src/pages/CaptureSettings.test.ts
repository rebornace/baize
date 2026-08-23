import { describe, expect, it } from 'vitest'
import {
  buildCaptureFromDraft,
  captureToDraft,
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
