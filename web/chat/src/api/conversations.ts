import { authInit, parseJSON } from './http'
import type { ChatMessage, IdentityView } from './types'

export interface MessageSearchHit {
  conversation_id: string
  conversation_title?: string
  message_id: string
  role: string
  snippet: string
  created_at?: string
}

export interface MessageSearchOptions {
  limit?: number
  /** user / assistant / system_note; omit for all roles. */
  role?: string
  /** Restrict to the last N days (1..3650). */
  days?: number
  /** Restrict to one conversation (still ACL-checked by the server). */
  conversationId?: string
}

export async function searchConversationMessages(
  query: string,
  opts: MessageSearchOptions = {},
): Promise<MessageSearchHit[]> {
  const params = new URLSearchParams({
    q: query,
    limit: String(opts.limit ?? 20),
  })
  if (opts.role) params.set('role', opts.role)
  if (opts.days) params.set('days', String(opts.days))
  if (opts.conversationId) params.set('conversation_id', opts.conversationId)
  const res = await fetch(`/v0/conversations/search?${params.toString()}`, {
    headers: authInit(),
  })
  const body = await parseJSON<{ hits?: MessageSearchHit[] }>(res)
  return body.hits ?? []
}

export type ConversationExportFormat = 'markdown' | 'json' | 'html'

export interface ConversationExportOptions {
  format?: ConversationExportFormat
  includeThinking?: boolean
}

export async function exportConversation(
  conversationId: string,
  opts: ConversationExportOptions = {},
): Promise<string> {
  const params = new URLSearchParams()
  if (opts.format) params.set('format', opts.format)
  if (opts.includeThinking) params.set('include_thinking', '1')
  const query = params.toString()
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/export${query ? `?${query}` : ''}`,
    { headers: authInit() },
  )
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${res.statusText}`)
  }
  return res.text()
}

export async function createConversationHostedLogin(
  conversationId: string,
  connectorId: string,
): Promise<{ session_id: string; url: string }> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/hosted-login`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ connector_id: connectorId }),
    },
  )
  return parseJSON<{ session_id: string; url: string }>(res)
}

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

/** Move a conversation to another workspace. Rejected while a run is active. */
export async function moveConversation(
  conversationId: string,
  workspaceId: string,
): Promise<{ conversation_id: string; workspace_id: string }> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/workspace`,
    {
      method: 'PATCH',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ workspace_id: workspaceId }),
    },
  )
  return parseJSON<{ conversation_id: string; workspace_id: string }>(res)
}

export interface BatchSkip {
  id: string
  reason: string
}

export async function batchMoveConversations(
  conversationIds: string[],
  workspaceId: string,
): Promise<{ moved: number; skipped: BatchSkip[]; workspace_id: string }> {
  const res = await fetch('/v0/conversations/batch/workspace', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ conversation_ids: conversationIds, workspace_id: workspaceId }),
  })
  return parseJSON(res)
}

export async function batchDeleteConversations(
  conversationIds: string[],
): Promise<{ deleted: number; skipped: BatchSkip[] }> {
  const res = await fetch('/v0/conversations/batch/delete', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ conversation_ids: conversationIds }),
  })
  return parseJSON(res)
}

export async function batchArchiveConversations(
  conversationIds: string[],
  archived: boolean,
): Promise<{ updated: number; archived: boolean; skipped: BatchSkip[] }> {
  const res = await fetch('/v0/conversations/batch/archived', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ conversation_ids: conversationIds, archived }),
  })
  return parseJSON(res)
}

export async function renameConversation(
  conversationId: string,
  title: string,
): Promise<{ conversation_id: string; title: string }> {
  const res = await fetch(`/v0/conversations/${encodeURIComponent(conversationId)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ title }),
  })
  return parseJSON(res)
}

export async function setConversationPinned(
  conversationId: string,
  pinned: boolean,
): Promise<{ conversation_id: string; pinned: boolean }> {
  const res = await fetch(`/v0/conversations/${encodeURIComponent(conversationId)}/pin`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ pinned }),
  })
  return parseJSON(res)
}

export async function setConversationArchived(
  conversationId: string,
  archived: boolean,
): Promise<{ conversation_id: string; archived: boolean }> {
  const res = await fetch(`/v0/conversations/${encodeURIComponent(conversationId)}/archived`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ archived }),
  })
  return parseJSON(res)
}

export async function markConversationRead(
  conversationId: string,
): Promise<{ conversation_id: string; read_at: string }> {
  const res = await fetch(`/v0/conversations/${encodeURIComponent(conversationId)}/read`, {
    method: 'PUT',
    headers: authInit(),
  })
  return parseJSON(res)
}

export async function markAllConversationsRead(): Promise<{ read_at: string }> {
  const res = await fetch('/v0/conversations/read-all', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON(res)
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
  opts?: { regenerate?: boolean; agentId?: string; locale?: string },
): Promise<RollbackMessagesResult> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/messages/${encodeURIComponent(messageId)}/rollback`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        regenerate: opts?.regenerate ?? false,
        agent_id: opts?.agentId,
        ...(opts?.locale ? { locale: opts.locale } : {}),
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
  pinned?: boolean
  archived?: boolean
  unread?: boolean
}

export async function listConversations(
  scope?: ConversationScope,
  workspaceId?: string,
  archived?: 'only' | 'all',
): Promise<ConversationSummary[]> {
  const qs = new URLSearchParams()
  if (scope) qs.set('scope', scope)
  if (workspaceId) qs.set('workspace_id', workspaceId)
  if (archived) qs.set('archived', archived)
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  const res = await fetch(`/v0/conversations${suffix}`, { headers: authInit() })
  const body = await parseJSON<{ conversations: ConversationSummary[] }>(res)
  return body.conversations ?? []
}
