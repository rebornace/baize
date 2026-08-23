import { describe, expect, it } from 'vitest'
import { extractAnalysisPagesFromEvents, parseAnalysisPageResult } from './analysisPage'

describe('parseAnalysisPageResult', () => {
  it('parses artifact_url', () => {
    const p = parseAnalysisPageResult({
      kind: 'analysis_page',
      artifact_url: '/v0/artifacts/art_1',
    })
    expect(p?.artifactUrl).toBe('/v0/artifacts/art_1')
  })

  it('parses JSON string content', () => {
    const p = parseAnalysisPageResult(
      JSON.stringify({ kind: 'analysis_page', artifact_url: '/v0/artifacts/art_2' }),
    )
    expect(p?.artifactUrl).toBe('/v0/artifacts/art_2')
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

describe('extractAnalysisPagesFromEvents', () => {
  it('extracts unique analysis pages in order', () => {
    const pages = extractAnalysisPagesFromEvents([
      {
        type: 'tool.result',
        timestamp: 't1',
        data: {
          name: 'create_analysis_page',
          content: { kind: 'analysis_page', artifact_url: '/v0/artifacts/a' },
        },
      },
      {
        type: 'tool.result',
        timestamp: 't2',
        data: {
          name: 'create_analysis_page',
          content: { kind: 'analysis_page', artifact_url: '/v0/artifacts/a' },
        },
      },
      {
        type: 'tool.result',
        timestamp: 't3',
        data: {
          name: 'create_analysis_page',
          content: { kind: 'analysis_page', artifact_url: '/v0/artifacts/b' },
        },
      },
      {
        type: 'llm.message',
        timestamp: 't4',
        data: { content: 'done' },
      },
    ])
    expect(pages.map((p) => p.artifactUrl)).toEqual([
      '/v0/artifacts/a',
      '/v0/artifacts/b',
    ])
  })
})
