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

export interface EvalArgContains {
  tool: string
  substr: string
}

export type EvalFieldOp = 'eq' | 'contains' | 'regex'

/** Field input may be a bare scalar (exact match) or an explicit op/value. */
export type EvalFieldExpect =
  | string
  | number
  | boolean
  | { op?: EvalFieldOp; value: unknown }

export interface EvalExpectState {
  /** Dot paths; array segments are numeric indexes. */
  fields: Record<string, EvalFieldExpect>
}

export interface EvalStateSnapshot {
  records?: Record<string, unknown>[]
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
  human_agree?: boolean | null
  human_note?: string
  calibrated_at?: string
  forbid_tools?: string[]
  arg_contains?: EvalArgContains[]
  expect_state?: EvalExpectState[]
  has_state_snapshot?: boolean
  state_snapshot?: EvalStateSnapshot
  created_at: string
  updated_at: string
}

export interface EvalCalibrationSummary {
  scored_count: number
  calibrated_count: number
  agree_count: number
  disagree_count: number
  agreement_rate: number | null
  uncalibrated_scored_count: number
}

export interface PutCalibrationBody {
  agree: boolean | null
  note?: string
}

export interface EvalScoreResult {
  pass: boolean
  codes: string[]
  notes?: string
}

export interface EvalScoreRecord {
  scored_at: string
  pass: boolean
  codes?: string[]
  notes?: string
}

export interface EvalScoresQuery {
  /** Only return scores from the last N days; omit for the whole history. */
  days?: number
  limit?: number
}

export interface EvalScoresResponse {
  items: EvalScoreRecord[]
  /** How many scores each case keeps before older ones are pruned. */
  history_cap?: number
}

export async function listEvalScores(
  id: string,
  query: EvalScoresQuery = {},
): Promise<EvalScoresResponse> {
  const { days, limit = 20 } = query
  const res = await fetch(
    `/v0/settings/eval/cases/${encodeURIComponent(id)}/scores${qs({ limit, days })}`,
    { headers: authInit() },
  )
  return parseJSON(res)
}

export interface EvalGateFailure {
  id: string
  input: string
  codes: string[]
  notes: string
}

export interface EvalGateResult {
  total: number
  passed: number
  failed: number
  pass_rate: number
  min_pass_rate: number
  pass: boolean
  empty: boolean
  ran_at: string
  failures: EvalGateFailure[]
}

export interface EvalGateState {
  min_pass_rate: number
  last: EvalGateResult | null
}

export async function getEvalGate(): Promise<EvalGateState> {
  const res = await fetch('/v0/settings/eval/gate', { headers: authInit() })
  return parseJSON(res)
}

export async function putEvalGate(minPassRate: number): Promise<void> {
  const res = await fetch('/v0/settings/eval/gate', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ min_pass_rate: minPassRate }),
  })
  await parseJSON(res)
}

export async function runEvalGate(): Promise<EvalGateResult> {
  const res = await fetch('/v0/settings/eval/gate/run', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON(res)
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
  forbid_tools?: string[]
  arg_contains?: EvalArgContains[]
  expect_state?: EvalExpectState[]
}

export async function ingestEvalCase(body: IngestEvalCaseBody): Promise<EvalCase> {
  const res = await fetch('/v0/settings/eval/cases', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON(res)
}

export interface SampleEvalBody {
  from: string
  to: string
  status?: string
  min_duration_ms?: number
  event_type?: string
  max_n?: number
  seed?: string
  mode?: 'draft' | 'score'
  protocol?: string
  required_reads?: string[]
  consequential?: string[]
}

export interface SampleEvalResult {
  mode: string
  candidates: number
  sampled: number
  created: number
  skipped: number
  scored: number
  errors: { run_id: string; reason: string }[]
  case_ids: string[]
}

export async function sampleEvalRuns(body: SampleEvalBody): Promise<SampleEvalResult> {
  const res = await fetch('/v0/settings/eval/sample', {
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
  forbid_tools?: string[]
  arg_contains?: EvalArgContains[]
  expect_state?: EvalExpectState[]
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

export async function checkEvalCaseState(id: string, stateUrl: string): Promise<EvalCase> {
  const res = await fetch(
    `/v0/settings/eval/cases/${encodeURIComponent(id)}/check-state`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ state_url: stateUrl }),
    },
  )
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

export async function getCalibrationSummary(): Promise<EvalCalibrationSummary> {
  const res = await fetch('/v0/settings/eval/calibration-summary', { headers: authInit() })
  return parseJSON(res)
}

export async function putCalibration(id: string, body: PutCalibrationBody): Promise<EvalCase> {
  const res = await fetch(
    `/v0/settings/eval/cases/${encodeURIComponent(id)}/calibration`,
    {
      method: 'PATCH',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    },
  )
  return parseJSON(res)
}

export interface SkillDraftTimelineEntry {
  run_id: string
  type: string
  timestamp: string
  summary: string
}

export interface SkillDraftResponse {
  source_run_id: string
  conversation_id?: string
  timeline: SkillDraftTimelineEntry[]
  suggested_id: string
  markdown: string
}

export async function fetchSkillDraft(runId: string): Promise<SkillDraftResponse> {
  const res = await fetch(
    `/v0/settings/eval/runs/${encodeURIComponent(runId)}/skill-draft`,
    { headers: authInit() },
  )
  return parseJSON(res)
}

export async function fetchConversationSkillDraft(
  conversationId: string,
): Promise<SkillDraftResponse> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/skill-draft`,
    { headers: authInit() },
  )
  return parseJSON(res)
}

export async function polishSkillDraft(
  runId: string,
  body?: { markdown?: string },
): Promise<{ markdown: string }> {
  const res = await fetch(
    `/v0/settings/eval/runs/${encodeURIComponent(runId)}/skill-draft/polish`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body ?? {}),
    },
  )
  return parseJSON(res)
}

export async function installSkillDraft(
  runId: string,
  body: { markdown: string; id?: string; create_eval_stub?: boolean },
): Promise<{ id: string; eval_stub?: string }> {
  const res = await fetch(
    `/v0/settings/eval/runs/${encodeURIComponent(runId)}/skill-draft/install`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    },
  )
  return parseJSON(res)
}
