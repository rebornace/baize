import { authInit, parseJSON } from './http'

export interface EvalCandidate {
  id: string
  agent_id: string
  status: string
  created_at: string
  ended_at?: string
  duration_ms?: number
  input_preview: string
}

export interface EvalChainLink {
  after: string
  before: string
}

export interface EvalCase {
  id: string
  source_run_id: string
  status: 'draft' | 'confirmed'
  protocol?: string
  input: string
  required_reads?: string[]
  consequential?: string[]
  expect_consequential_any?: string[]
  chain?: EvalChainLink[]
  run_status_at_ingest?: string
  last_score_pass?: boolean
  last_score_codes?: string[]
  last_score_notes?: string
  last_scored_at?: string
  created_at: string
  updated_at: string
}

export interface EvalScoreResult {
  pass: boolean
  codes: string[]
  notes?: string
}

export interface EvalRunsQuery {
  from: string
  to: string
  status?: string
  min_duration_ms?: number
  event_type?: string
  limit?: number
}

function qs(params: Record<string, string | number | undefined>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '') continue
    sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

export async function queryEvalRuns(q: EvalRunsQuery): Promise<{ items: EvalCandidate[] }> {
  const res = await fetch(
    `/v0/settings/eval/runs${qs({
      from: q.from,
      to: q.to,
      status: q.status,
      min_duration_ms: q.min_duration_ms,
      event_type: q.event_type,
      limit: q.limit,
    })}`,
    { headers: authInit() },
  )
  return parseJSON(res)
}

export async function listEvalCases(limit?: number): Promise<{ items: EvalCase[] }> {
  const res = await fetch(
    `/v0/settings/eval/cases${qs({ limit })}`,
    { headers: authInit() },
  )
  return parseJSON(res)
}

export interface IngestEvalCaseBody {
  run_id: string
  replace?: boolean
  protocol?: string
  required_reads?: string[]
  consequential?: string[]
  expect_consequential_any?: string[]
  chain?: EvalChainLink[]
}

export async function ingestEvalCase(body: IngestEvalCaseBody): Promise<EvalCase> {
  const res = await fetch('/v0/settings/eval/cases', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON(res)
}

export interface PatchEvalCaseBody {
  protocol?: string
  required_reads?: string[]
  consequential?: string[]
  expect_consequential_any?: string[]
  chain?: EvalChainLink[]
  status?: 'draft' | 'confirmed'
}

export async function patchEvalCase(id: string, body: PatchEvalCaseBody): Promise<EvalCase> {
  const res = await fetch(`/v0/settings/eval/cases/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON(res)
}

export async function scoreEvalCase(id: string): Promise<EvalScoreResult> {
  const res = await fetch(`/v0/settings/eval/cases/${encodeURIComponent(id)}/score`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON(res)
}

export async function exportEvalCase(id: string): Promise<string> {
  const res = await fetch(`/v0/settings/eval/cases/${encodeURIComponent(id)}/export`, {
    headers: authInit(),
  })
  if (!res.ok) {
    return parseJSON(res) as never
  }
  return res.text()
}

export async function deleteEvalCase(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/eval/cases/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  if (!res.ok) {
    await parseJSON(res)
  }
}
