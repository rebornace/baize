import { authInit, parseJSON } from './http'

export interface ConversationAttachment {
  object: string
  filename: string
  media_type: string
  size_bytes: number
  created_at: string
  url: string
}

export async function listConversationAttachments(
  conversationId: string,
  limit = 100,
): Promise<{ items: ConversationAttachment[] }> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/attachments?limit=${limit}`,
    { headers: authInit() },
  )
  return parseJSON(res)
}

export async function deleteConversationAttachments(
  conversationId: string,
  objects: string[],
): Promise<{ deleted: number; failed: number }> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/attachments/delete`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ objects }),
    },
  )
  return parseJSON(res)
}

export interface ConversationRetention {
  days: number
  effective_days: number
  global_days: number
}

export async function getConversationRetention(
  conversationId: string,
): Promise<ConversationRetention> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/attachment-retention`,
    { headers: authInit() },
  )
  return parseJSON(res)
}

export async function putConversationRetention(
  conversationId: string,
  days: number,
): Promise<ConversationRetention> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/attachment-retention`,
    {
      method: 'PUT',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ days }),
    },
  )
  return parseJSON(res)
}

export async function getConversationMessageRetention(
  conversationId: string,
): Promise<ConversationRetention> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/message-retention`,
    { headers: authInit() },
  )
  return parseJSON(res)
}

export async function putConversationMessageRetention(
  conversationId: string,
  days: number,
): Promise<ConversationRetention> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/message-retention`,
    {
      method: 'PUT',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ days }),
    },
  )
  return parseJSON(res)
}

export async function getConversationRunRetention(
  conversationId: string,
): Promise<ConversationRetention> {
  const res = await fetch(`/v0/conversations/${encodeURIComponent(conversationId)}/run-retention`, {
    headers: authInit(),
  })
  return parseJSON(res)
}

export async function putConversationRunRetention(
  conversationId: string,
  days: number,
): Promise<ConversationRetention> {
  const res = await fetch(`/v0/conversations/${encodeURIComponent(conversationId)}/run-retention`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ days }),
  })
  return parseJSON(res)
}

export interface ConversationCostBudget {
  budget_usd_per_day: number
  budget_usd_per_month: number
  warning_ratio: number
  exceeded_ratio: number
  spent_usd_per_day: number
  spent_usd_per_month: number
}

export async function getConversationCostBudget(
  conversationId: string,
): Promise<ConversationCostBudget> {
  const res = await fetch(`/v0/conversations/${encodeURIComponent(conversationId)}/cost-budget`, {
    headers: authInit(),
  })
  return parseJSON(res)
}

export async function putConversationCostBudget(
  conversationId: string,
  day: number,
  month: number,
  warningRatio = 0,
  exceededRatio = 0,
): Promise<ConversationCostBudget> {
  const res = await fetch(`/v0/conversations/${encodeURIComponent(conversationId)}/cost-budget`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({
      budget_usd_per_day: day,
      budget_usd_per_month: month,
      warning_ratio: warningRatio,
      exceeded_ratio: exceededRatio,
    }),
  })
  return parseJSON(res)
}
