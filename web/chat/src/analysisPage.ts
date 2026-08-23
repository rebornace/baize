export interface AnalysisPageParsed {
  artifactUrl: string
}

export function parseAnalysisPageResult(content: unknown): AnalysisPageParsed | null {
  if (content == null || typeof content !== 'object') return null
  const obj = content as Record<string, unknown>
  if (obj.kind !== 'analysis_page') return null
  const artifactUrl = obj.artifact_url
  if (typeof artifactUrl !== 'string' || artifactUrl.length === 0) return null
  return { artifactUrl }
}
