import { authInit, parseJSON } from './http'

export type SkillSummary = {
  id: string
  name: string
  description: string
  /** Optional English catalog blurb when the skill ships SKILL.en.md. */
  description_en?: string
  tools: string[]
  source: 'builtin' | 'user' | 'managed'
  eval_case_count?: number
}

export type SkillEvalChainLink = { after: string; before: string }
export type SkillEvalArgContains = { tool: string; substr: string }
export type SkillEvalExpectState = { id?: string; title_contains?: string; status?: string }

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
): Promise<void> {
  const res = await fetch(`/v0/agents/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  await parseJSON<AgentView>(res)
}
