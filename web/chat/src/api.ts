export type RunStatus =
  | 'queued'
  | 'running'
  | 'waiting_human'
  | 'succeeded'
  | 'failed'

export interface Run {
  id: string
  agent_id: string
  input: string
  status: RunStatus
  output?: string
  error?: string
  created_at: string
}

export interface Event {
  type: string
  timestamp: string
  data?: Record<string, unknown>
}

export interface CreateRunResponse {
  run_id: string
  status: RunStatus
  conversation_id?: string
}

export interface ResumeResponse {
  run_id: string
  status: RunStatus
}

export interface IdentityView {
  id: string
  label: string
  scheme?: string
  source: string
  claims_summary?: Record<string, unknown>
  is_default: boolean
  last_used_at?: string
}

async function parseJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let detail = res.statusText
    try {
      const body = (await res.json()) as { error?: { message?: string } }
      if (body.error?.message) detail = body.error.message
    } catch {
      /* ignore */
    }
    throw new Error(`HTTP ${res.status}: ${detail}`)
  }
  return (await res.json()) as T
}

export async function getUIConfig(): Promise<{ agent_id: string }> {
  const res = await fetch('/v0/ui-config')
  return parseJSON<{ agent_id: string }>(res)
}

export async function createRun(
  agentId: string,
  input: string,
  conversationId: string,
  identityId?: string,
): Promise<CreateRunResponse> {
  const body: Record<string, string> = {
    agent_id: agentId,
    input,
    conversation_id: conversationId,
  }
  if (identityId) body.identity_id = identityId
  const res = await fetch('/v0/runs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  return parseJSON<CreateRunResponse>(res)
}

export async function getRun(runId: string): Promise<Run> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}`)
  return parseJSON<Run>(res)
}

export async function listEvents(runId: string): Promise<Event[]> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/events`)
  return parseJSON<Event[]>(res)
}

export async function resumeRun(
  runId: string,
  decision: 'approve' | 'reject',
  comment = '',
): Promise<ResumeResponse> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/resume`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ decision, comment }),
  })
  return parseJSON<ResumeResponse>(res)
}

export function isTerminal(status: RunStatus): boolean {
  return status === 'succeeded' || status === 'failed'
}

export interface ToolInfo {
  name: string
  description?: string
  connector_id: string
  operation_id?: string
  method?: string
  path?: string
  require_approval?: boolean
}

export async function listTools(): Promise<ToolInfo[]> {
  const res = await fetch('/v0/tools')
  const body = await parseJSON<{ tools: ToolInfo[] }>(res)
  return body.tools ?? []
}

export async function listIdentities(conversationId: string): Promise<IdentityView[]> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities`,
  )
  return parseJSON<IdentityView[]>(res)
}

export async function setDefaultIdentity(conversationId: string, id: string): Promise<void> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities/${encodeURIComponent(id)}/default`,
    { method: 'POST' },
  )
  await parseJSON<{ status: string }>(res)
}

export async function deleteIdentity(conversationId: string, id: string): Promise<void> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  await parseJSON<{ status: string }>(res)
}

export async function clearIdentities(conversationId: string): Promise<void> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities`,
    { method: 'DELETE' },
  )
  await parseJSON<{ status: string }>(res)
}

export interface ChatMessage {
  id: string
  conversation_id: string
  role: 'user' | 'assistant' | 'system_note'
  content: string
  run_id?: string
  created_at: string
}

export async function listMessages(conversationId: string): Promise<ChatMessage[]> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/messages`,
  )
  return parseJSON<ChatMessage[]>(res)
}

export async function clearMessages(conversationId: string): Promise<void> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/messages`,
    { method: 'DELETE' },
  )
  await parseJSON<{ status: string }>(res)
}
