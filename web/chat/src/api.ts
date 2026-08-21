import { authHeaders } from './controlAuth'
import { consumeSSE } from './parseSSE'

let gateEnabled = false

export function setGateEnabled(v: boolean): void {
  gateEnabled = v
}

function authInit(extra?: Record<string, string>): HeadersInit {
  return { ...(authHeaders(gateEnabled) as Record<string, string>), ...extra }
}

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

export async function getUIConfig(): Promise<{ agent_id: string; gate_enabled: boolean }> {
  const res = await fetch('/v0/ui-config')
  return parseJSON(res)
}

export async function getMe(): Promise<{ role: string }> {
  const res = await fetch('/v0/me', { headers: { ...authHeaders(true) } })
  return parseJSON(res)
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
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<CreateRunResponse>(res)
}

export async function getRun(runId: string): Promise<Run> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}`, {
    headers: authInit(),
  })
  return parseJSON<Run>(res)
}

export async function listEvents(runId: string): Promise<Event[]> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/events`, {
    headers: authInit(),
  })
  return parseJSON<Event[]>(res)
}

export async function resumeRun(
  runId: string,
  decision: 'approve' | 'reject',
  comment = '',
): Promise<ResumeResponse> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/resume`, {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ decision, comment }),
  })
  return parseJSON<ResumeResponse>(res)
}

export function isTerminal(status: RunStatus): boolean {
  return status === 'succeeded' || status === 'failed'
}

export interface ToolInfo {
  name: string
  title?: string
  description?: string
  description_custom?: boolean
  connector_id: string
  operation_id?: string
  method?: string
  path?: string
  require_approval?: boolean
  require_login?: boolean
  enabled?: boolean
  source?: string
  input_schema?: Record<string, unknown>
}

export async function listTools(): Promise<ToolInfo[]> {
  const res = await fetch('/v0/tools', { headers: authInit() })
  const body = await parseJSON<{ tools: ToolInfo[] }>(res)
  return body.tools ?? []
}

export async function patchTool(
  name: string,
  body: { enabled?: boolean; require_login?: boolean; title?: string; description?: string },
): Promise<ToolInfo> {
  const res = await fetch(`/v0/tools/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<ToolInfo>(res)
}

export async function patchToolRequireLogin(name: string, requireLogin: boolean): Promise<ToolInfo> {
  return patchTool(name, { require_login: requireLogin })
}

export async function createConnectorTool(
  connectorId: string,
  body: {
    name: string
    method: string
    path: string
    title?: string
    description?: string
    input_schema?: Record<string, unknown>
  },
): Promise<ToolInfo> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(connectorId)}/tools`, {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<ToolInfo>(res)
}

export async function deleteConnectorTool(connectorId: string, name: string): Promise<void> {
  const res = await fetch(
    `/v0/connectors/${encodeURIComponent(connectorId)}/tools/${encodeURIComponent(name)}`,
    { method: 'DELETE', headers: authInit() },
  )
  if (!res.ok) {
    let detail = res.statusText
    try {
      const errBody = (await res.json()) as { error?: { message?: string } }
      if (errBody.error?.message) detail = errBody.error.message
    } catch {
      /* ignore */
    }
    throw new Error(`HTTP ${res.status}: ${detail}`)
  }
}

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

export async function listIdentities(conversationId: string): Promise<IdentityView[]> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/identities`,
    { headers: authInit() },
  )
  return parseJSON<IdentityView[]>(res)
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
    { headers: authInit() },
  )
  return parseJSON<ChatMessage[]>(res)
}

export async function clearMessages(conversationId: string): Promise<void> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/messages`,
    { method: 'DELETE', headers: authInit() },
  )
  await parseJSON<{ status: string }>(res)
}

export async function listConversations(): Promise<
  { id: string; title: string; updated_at: string }[]
> {
  const res = await fetch('/v0/conversations', { headers: authInit() })
  const body = await parseJSON<{
    conversations: { id: string; title: string; updated_at: string }[]
  }>(res)
  return body.conversations ?? []
}

export function openRunStream(
  runId: string,
  after: number,
  onEvent: (e: Event, index: number) => void,
  onEnded: (status: string) => void,
  onFatal: () => void,
): () => void {
  const url =
    `${window.location.origin}/v0/runs/${encodeURIComponent(runId)}/stream` +
    `?after=${after}`
  const ac = new AbortController()
  let closed = false

  const finish = (fn: () => void) => {
    if (closed) return
    closed = true
    ac.abort()
    fn()
  }

  void (async () => {
    try {
      const res = await fetch(url, {
        headers: authHeaders(gateEnabled),
        signal: ac.signal,
      })
      const contentType = res.headers.get('content-type') ?? ''
      if (!res.ok || !contentType.includes('event-stream') || !res.body) {
        finish(() => onFatal())
        return
      }
      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      while (!closed) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const { frames, rest } = consumeSSE(buffer)
        buffer = rest
        for (const frame of frames) {
          if (closed) return
          if (frame.event === 'run.ended') {
            let status = ''
            try {
              status = (JSON.parse(frame.data) as { status?: string }).status ?? ''
            } catch {
              status = ''
            }
            finish(() => onEnded(status))
            return
          }
          let parsed: Event
          try {
            parsed = JSON.parse(frame.data) as Event
          } catch {
            continue
          }
          const index = Number.parseInt(frame.id, 10)
          onEvent(parsed, Number.isFinite(index) ? index : -1)
        }
      }
      if (!closed) finish(() => onFatal())
    } catch (err) {
      if (closed) return
      if (err instanceof DOMException && err.name === 'AbortError') return
      finish(() => onFatal())
    }
  })()

  return () => {
    if (closed) return
    closed = true
    ac.abort()
  }
}
