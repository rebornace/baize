import { authInit, parseJSON } from './http'
import type { EvalExpectState } from './eval'

export type SkillSummary = {
  id: string
  name: string
  description: string
  /** Optional English catalog blurb when the skill ships SKILL.en.md. */
  description_en?: string
  /** Optional domain tag (ticket, crm, ...); non-empty means domain-specific. */
  domain?: string
  tools: string[]
  requires_tools?: string[]
  requires_input?: string[]
  missing_tools?: string[]
  source: 'builtin' | 'user' | 'managed'
  eval_case_count?: number
  has_workflow?: boolean
}

export type SkillWorkflowCondition = {
  all?: SkillWorkflowCondition[]
  any?: SkillWorkflowCondition[]
  not?: SkillWorkflowCondition
  path?: string
  op?: string
  value?: unknown
}

export type SkillWorkflowStep = {
  id: string
  tool: string
  approve?: boolean
  when?: SkillWorkflowCondition
  args?: Record<string, unknown>
  /** Extra invocations after a failed step (0-5). HITL pauses never retry. */
  retry?: number
  /** Per-invocation timeout in milliseconds (0 = none, max 600000). */
  timeout_ms?: number
  /** Record a tolerated failure and keep the pipeline going. */
  continue_on_error?: boolean
}

export type SkillWorkflow = {
  name: string
  steps: SkillWorkflowStep[]
}

export type SkillWorkflowView = {
  skill_id: string
  editable: boolean
  can_create: boolean
  source: string
  workflow?: SkillWorkflow
  yaml?: string
  missing_tools?: string[]
  warnings?: string[]
}

export type SkillWorkflowPreviewStep = {
  id: string
  tool: string
  status: 'rendered' | 'skipped' | 'error'
  args?: Record<string, unknown>
  error?: string
}

export type SkillWorkflowPreview = {
  steps: SkillWorkflowPreviewStep[]
  tree?: Record<string, unknown>
}

export type SkillEvalChainLink = { after: string; before: string }
export type SkillEvalArgContains = { tool: string; substr: string }
export type SkillEvalExpectState = EvalExpectState

export type SkillEvalCase = {
  id: string
  protocol: string
  input: string
  required_reads?: string[]
  consequential?: string[]
  expect_consequential_any?: string[]
  chain?: SkillEvalChainLink[]
  forbid_tools?: string[]
  arg_contains?: SkillEvalArgContains[]
  expect_state?: SkillEvalExpectState[]
  notes?: string
}

export type SkillEvalDoc = {
  harness: string
  cases: SkillEvalCase[]
}

export type SkillEvalRunResult = {
  id: string
  pass: boolean
  codes: string[]
  notes: string
  run_id: string
}

export async function listSkills(locale?: string): Promise<{ skills: SkillSummary[] }> {
  const q = locale?.trim() ? `?locale=${encodeURIComponent(locale.trim())}` : ''
  const res = await fetch(`/v0/skills${q}`, { headers: authInit() })
  return parseJSON<{ skills: SkillSummary[] }>(res)
}

export async function uploadSkill(file: File): Promise<SkillSummary> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch('/v0/skills', {
    method: 'POST',
    headers: authInit(),
    body: form,
  })
  return parseJSON<SkillSummary>(res)
}

export async function deleteSkill(id: string): Promise<void> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}

export async function getSkillWorkflow(id: string): Promise<SkillWorkflowView> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/workflow`, {
    headers: authInit(),
  })
  return parseJSON<SkillWorkflowView>(res)
}

export async function putSkillWorkflow(
  id: string,
  workflow: SkillWorkflow,
): Promise<SkillWorkflowView> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/workflow`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(workflow),
  })
  return parseJSON<SkillWorkflowView>(res)
}

export async function deleteSkillWorkflow(id: string): Promise<void> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/workflow`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}

export async function putSkillWorkflowRaw(
  id: string,
  yaml: string,
): Promise<SkillWorkflowView> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/workflow/raw`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ yaml }),
  })
  return parseJSON<SkillWorkflowView>(res)
}

export async function previewSkillWorkflow(
  id: string,
  input: Record<string, unknown>,
  stepResults?: Record<string, Record<string, unknown>>,
): Promise<SkillWorkflowPreview> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/workflow/preview`, {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ input, step_results: stepResults }),
  })
  return parseJSON<SkillWorkflowPreview>(res)
}

export async function cloneSkillWorkflow(id: string): Promise<SkillWorkflowView> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/workflow/clone`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<SkillWorkflowView>(res)
}

export async function getSkillEval(id: string): Promise<SkillEvalDoc> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/eval`, { headers: authInit() })
  return parseJSON<SkillEvalDoc>(res)
}

export async function putSkillEval(id: string, doc: SkillEvalDoc): Promise<SkillEvalDoc> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/eval`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(doc),
  })
  return parseJSON<SkillEvalDoc>(res)
}

export async function deleteSkillEval(id: string): Promise<void> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/eval`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}

export async function runSkillEval(
  id: string,
  body: { agent_id: string; case_ids?: string[] },
): Promise<{ results: SkillEvalRunResult[] }> {
  const res = await fetch(`/v0/skills/${encodeURIComponent(id)}/eval/run`, {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<{ results: SkillEvalRunResult[] }>(res)
}

export type AgentView = {
  id: string
  system: string
  skills?: string[]
  /** empty/floor = all tools; exclusive = only activated skill tools */
  tool_binding?: '' | 'floor' | 'exclusive'
}

export async function getAgent(id: string): Promise<AgentView> {
  const res = await fetch(`/v0/agents/${encodeURIComponent(id)}`, {
    headers: authInit(),
  })
  return parseJSON<AgentView>(res)
}

export async function putAgent(
  id: string,
  body: { system: string; skills: string[]; tool_binding?: string },
): Promise<{ warnings?: string[] }> {
  const res = await fetch(`/v0/agents/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<AgentView & { warnings?: string[] }>(res)
}
