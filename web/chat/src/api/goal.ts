import { authInit, parseJSON } from './http'

export type GoalFrame = {
  id: string
  text: string
  details?: string
  criteria?: string[]
  status: 'active' | 'done' | 'interrupted' | 'superseded'
  source: 'user' | 'harness' | 'agent'
  revision: number
}

export type GoalStackView = {
  conversation_id: string
  revision: number
  frames: GoalFrame[]
  root?: GoalFrame | null
  tip?: GoalFrame | null
}

function goalURL(conversationId: string, suffix = ''): string {
  return `/v0/conversations/${encodeURIComponent(conversationId)}/goal${suffix}`
}

export async function getGoal(conversationId: string): Promise<GoalStackView> {
  const res = await fetch(goalURL(conversationId), { headers: authInit() })
  return parseJSON<GoalStackView>(res)
}

export async function putGoal(
  conversationId: string,
  text: string,
  criteria?: string[],
  details?: string,
): Promise<GoalStackView> {
  const res = await fetch(goalURL(conversationId), {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({
      text,
      criteria: criteria ?? undefined,
      details: details ?? undefined,
    }),
  })
  return parseJSON<GoalStackView>(res)
}

export async function pushGoal(conversationId: string, text: string): Promise<GoalStackView> {
  const res = await fetch(goalURL(conversationId, '/push'), {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ text }),
  })
  return parseJSON<GoalStackView>(res)
}

export async function popGoal(conversationId: string): Promise<GoalStackView> {
  const res = await fetch(goalURL(conversationId, '/pop'), {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<GoalStackView>(res)
}

export async function stopGoal(conversationId: string): Promise<GoalStackView> {
  const res = await fetch(goalURL(conversationId, '/stop'), {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<GoalStackView>(res)
}

export async function resumeGoal(conversationId: string): Promise<GoalStackView> {
  const res = await fetch(goalURL(conversationId, '/resume'), {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<GoalStackView>(res)
}

export async function patchGoalTip(
  conversationId: string,
  text: string,
  details?: string,
): Promise<GoalStackView> {
  const res = await fetch(goalURL(conversationId, '/tip'), {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({
      text,
      details: details !== undefined ? details : undefined,
    }),
  })
  return parseJSON<GoalStackView>(res)
}
