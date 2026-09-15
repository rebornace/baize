import { authInit, parseJSON } from './http'
import type { ChatMessage, IdentityView } from './types'

export async function listIdentities(conversationId: string): Promise<IdentityView[]> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities`,
    { headers: authInit() },
  )
  return parseJSON<IdentityView[]>(res)
}

export async function createIdentity(
  conversationId: string,
  token: string,
  label?: string,
): Promise<IdentityView> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        token,
        label: label?.trim() || undefined,
      }),
    },
  )
  return parseJSON<IdentityView>(res)
}

export async function setDefaultIdentity(conversationId: string, id: string): Promise<void> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities/${encodeURIComponent(id)}/default`,
    { method: 'POST', headers: authInit() },
  )
  await parseJSON<{ status: string }>(res)
}

export async function deleteIdentity(conversationId: string, id: string): Promise<void> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities/${encodeURIComponent(id)}`,
    { method: 'DELETE', headers: authInit() },
  )
  await parseJSON<{ status: string }>(res)
}

export async function clearIdentities(conversationId: string): Promise<void> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities`,
    { method: 'DELETE', headers: authInit() },
  )
  await parseJSON<{ status: string }>(res)
}

export async function listMessages(conversationId: string): Promise<ChatMessage[]> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/messages`,
    { headers: authInit() },
  )
  return parseJSON<ChatMessage[]>(res)
}

/**
 * Permanently delete a conversation: its messages, ownership meta and captured
 * identities. The server rejects (409 conversation_busy) a conversation with
 * an active run.
 */
export async function deleteConversation(conversationId: string): Promise<void> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}`,
    { method: 'DELETE', headers: authInit() },
  )
  await parseJSON<{ status: string }>(res)
}

export interface RollbackMessagesResult {
  conversation_id: string
  deleted_count: number
  messages: ChatMessage[]
  regenerated_run?: { run_id: string; status: string }
}

export async function rollbackMessages(
  conversationId: string,
  messageId: string,
  opts?: { regenerate?: boolean; agentId?: string },
): Promise<RollbackMessagesResult> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/messages/${encodeURIComponent(messageId)}/rollback`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        regenerate: opts?.regenerate ?? false,
        agent_id: opts?.agentId,
      }),
    },
  )
  return parseJSON<RollbackMessagesResult>(res)
}

export interface ForkConversationResult {
  source_conversation_id: string
  conversation_id: string
  copied_count: number
  messages: ChatMessage[]
}

export async function forkConversation(
  conversationId: string,
  throughMessageId: string,
): Promise<ForkConversationResult> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/fork`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ through_message_id: throughMessageId }),
    },
  )
  return parseJSON<ForkConversationResult>(res)
}

export type ConversationScope = 'mine' | 'all'

export interface ConversationSummary {
  id: string
  title: string
  updated_at: string
}

export async function listConversations(
  scope?: ConversationScope,
): Promise<ConversationSummary[]> {
  const qs = scope ? `?scope=${encodeURIComponent(scope)}` : ''
  const res = await fetch(`/v0/conversations${qs}`, { headers: authInit() })
  const body = await parseJSON<{ conversations: ConversationSummary[] }>(res)
  return body.conversations ?? []
}
