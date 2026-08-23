import { describe, expect, it } from 'vitest'
import { parseAnalysisPageResult } from './analysisPage'

describe('parseAnalysisPageResult', () => {
  it('parses artifact_url', () => {
    const p = parseAnalysisPageResult({
      kind: 'analysis_page',
      artifact_url: '/v0/artifacts/art_1',
    })
    expect(p?.artifactUrl).toBe('/v0/artifacts/art_1')
  })

  it('returns null for wrong kind', () => {
    expect(
      parseAnalysisPageResult({ kind: 'html_report', artifact_url: '/v0/artifacts/art_1' }),
    ).toBeNull()
  })

  it('returns null when artifact_url missing', () => {
    expect(parseAnalysisPageResult({ kind: 'analysis_page' })).toBeNull()
  })

  it('returns null for non-object content', () => {
    expect(parseAnalysisPageResult(null)).toBeNull()
    expect(parseAnalysisPageResult('text')).toBeNull()
  })
})
