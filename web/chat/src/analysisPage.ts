import type { Event } from './api'

export interface AnalysisPageParsed {
  artifactUrl: string
}

export function parseAnalysisPageResult(content: unknown): AnalysisPageParsed | null {
  if (content == null) return null
  let obj: Record<string, unknown>
  if (typeof content === 'string') {
    try {
      const parsed: unknown = JSON.parse(content)
      if (parsed == null || typeof parsed !== 'object') return null
      obj = parsed as Record<string, unknown>
    } catch {
      return null
    }
  } else if (typeof content === 'object') {
    obj = content as Record<string, unknown>
  } else {
    return null
  }
  if (obj.kind !== 'analysis_page') return null
  const artifactUrl = obj.artifact_url
  if (typeof artifactUrl !== 'string' || artifactUrl.length === 0) return null
  return { artifactUrl }
}

/** Collect unique analysis page artifact URLs from a run's events (stable order). */
export function extractAnalysisPagesFromEvents(events: Event[]): AnalysisPageParsed[] {
  const out: AnalysisPageParsed[] = []
  const seen = new Set<string>()
  for (const ev of events) {
    if (ev.type !== 'tool.result') continue
    if (ev.data?.is_error) continue
    const parsed = parseAnalysisPageResult(ev.data?.content)
    if (!parsed || seen.has(parsed.artifactUrl)) continue
    seen.add(parsed.artifactUrl)
    out.push(parsed)
  }
  return out
}
