import { authInit, parseJSON } from './http'

export type SkillSummary = {
  id: string
  name: string
  description: string
  tools: string[]
  source: 'builtin' | 'user'
}

export async function listSkills(): Promise<{ skills: SkillSummary[] }> {
  const res = await fetch('/v0/skills', { headers: authInit() })
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

export async function getAgent(
  id: string,
): Promise<{ id: string; system: string; skills?: string[] }> {
  const res = await fetch(`/v0/agents/${encodeURIComponent(id)}`, {
    headers: authInit(),
  })
  return parseJSON<{ id: string; system: string; skills?: string[] }>(res)
}

export async function putAgent(
  id: string,
  body: { system: string; skills: string[] },
): Promise<void> {
  const res = await fetch(`/v0/agents/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  await parseJSON<{ id: string; system: string; skills?: string[] }>(res)
}
