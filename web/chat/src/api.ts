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
  | 'cancelled'

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

export interface UIConfig {
  agent_id: string
  gate_enabled: boolean
  supports_vision: boolean
}

export async function getUIConfig(): Promise<UIConfig> {
  const res = await fetch('/v0/ui-config')
  return parseJSON(res)
}

export interface MeResponse {
  role: string
  operator_id?: string
  gate_enabled?: boolean
}

export async function getMe(): Promise<MeResponse> {
  const res = await fetch('/v0/me', { headers: { ...authHeaders(true) } })
  return parseJSON(res)
}

export interface WeixinChannelSettings {
  agent_id: string
  allowlist: string[]
  assignee: string
  enabled: boolean
}

export interface WeixinLoginStart {
  ticket: string
  qr_url: string
}

export interface WeixinLoginStatus {
  status: string
}

export async function startWeixinLogin(): Promise<WeixinLoginStart> {
  const res = await fetch('/v0/settings/channels/weixin/login/start', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<WeixinLoginStart>(res)
}

export async function getWeixinLoginStatus(ticket: string): Promise<WeixinLoginStatus> {
  const qs = new URLSearchParams({ ticket })
  const res = await fetch(`/v0/settings/channels/weixin/login/status?${qs}`, {
    headers: authInit(),
  })
  return parseJSON<WeixinLoginStatus>(res)
}

export async function logoutWeixin(): Promise<{ status: string }> {
  const res = await fetch('/v0/settings/channels/weixin/logout', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ status: string }>(res)
}

export async function getWeixinSettings(): Promise<WeixinChannelSettings> {
  const res = await fetch('/v0/settings/channels/weixin', { headers: authInit() })
  return parseJSON<WeixinChannelSettings>(res)
}

export async function putWeixinSettings(
  body: WeixinChannelSettings,
): Promise<WeixinChannelSettings> {
  const res = await fetch('/v0/settings/channels/weixin', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<WeixinChannelSettings>(res)
}

export interface Attachment {
  filename: string
  media_type: string
  content_base64: string
}

// EXTENSION_MIME maps supported attachment extensions to the canonical MIME
// the backend's attach.Process accepts. We infer from the extension (rather
// than trusting File.type) because Windows often gives .md an empty type and
// .csv "application/vnd.ms-excel", which would otherwise be rejected as
// unsupported_attachment before the bytes are even inspected.
const EXTENSION_MIME: Record<string, string> = {
  '.txt': 'text/plain',
  '.md': 'text/markdown',
  '.csv': 'text/csv',
  '.docx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  '.xlsx': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  '.pdf': 'application/pdf',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.webp': 'image/webp',
  '.gif': 'image/gif',
}

/**
 * inferMediaType returns the canonical backend MIME for a file by extension,
 * falling back to the browser-provided type (or application/octet-stream) for
 * unknown extensions. This normalizes platform quirks (e.g. .csv reported as
 * application/vnd.ms-excel on Windows) so the backend doesn't reject a
 * supported file as unsupported_attachment.
 */
export function inferMediaType(file: File): string {
  const name = file.name.toLowerCase()
  const dot = name.lastIndexOf('.')
  if (dot >= 0) {
    const ext = name.slice(dot)
    const canonical = EXTENSION_MIME[ext]
    if (canonical) return canonical
  }
  return file.type || 'application/octet-stream'
}

/** Read a File into an Attachment (base64-encoded content). */
export function fileToAttachment(file: File): Promise<Attachment> {
  const media_type = inferMediaType(file)
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error ?? new Error('read failed'))
    reader.onload = () => {
      const result = reader.result
      if (typeof result !== 'string') {
        reject(new Error('unsupported file read result'))
        return
      }
      const comma = result.indexOf(',')
      const content_base64 = comma < 0 ? result : result.slice(comma + 1)
      resolve({
        filename: file.name,
        media_type,
        content_base64,
      })
    }
    reader.readAsDataURL(file)
  })
}

const IMAGE_MIMES = new Set([
  'image/png',
  'image/jpeg',
  'image/jpg',
  'image/webp',
  'image/gif',
])

/** True for image MIME types the backend accepts as vision attachments. */
export function isImageAttachment(mediaType: string): boolean {
  return IMAGE_MIMES.has(mediaType.toLowerCase())
}

export interface CreateRunOptions {
  identityId?: string
  sessionToken?: string
  webhookUrl?: string
  webhookHeaders?: Record<string, string>
  skills?: string[]
  attachments?: Attachment[]
}

export async function createRun(
  agentId: string,
  input: string,
  conversationId: string,
  options?: CreateRunOptions,
): Promise<CreateRunResponse> {
  const body: Record<string, unknown> = {
    agent_id: agentId,
    input,
    conversation_id: conversationId,
  }
  if (options?.identityId) body.identity_id = options.identityId
  if (options?.sessionToken) body.session_token = options.sessionToken
  if (options?.webhookUrl) body.webhook_url = options.webhookUrl
  if (options?.webhookHeaders && Object.keys(options.webhookHeaders).length > 0) {
    body.webhook_headers = options.webhookHeaders
  }
  if (options?.skills && options.skills.length > 0) {
    body.skills = options.skills
  }
  if (options?.attachments && options.attachments.length > 0) {
    body.attachments = options.attachments
  }
  const res = await fetch('/v0/runs', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    let code = 'unknown'
    let message = res.statusText
    try {
      const errBody = (await res.json()) as { error?: { code?: string; message?: string } }
      if (errBody.error?.code) code = errBody.error.code
      if (errBody.error?.message) message = errBody.error.message
    } catch {
      /* ignore */
    }
    throw new ApiError(res.status, code, message)
  }
  return parseJSON<CreateRunResponse>(res)
}

export interface StoreSettings {
  driver: string
  sqlite_path?: string
  dsn?: string
  dsn_redacted?: string
  drivers: string[]
  config_path?: string
  overlay_path?: string
}

export async function getStoreSettings(): Promise<StoreSettings> {
  const res = await fetch('/v0/settings/store', { headers: authInit() })
  return parseJSON<StoreSettings>(res)
}

export async function putStoreSettings(body: {
  driver: string
  sqlite_path?: string
  dsn?: string
  acknowledge_no_migrate: boolean
  restart?: boolean
}): Promise<{ status: string; message?: string }> {
  const res = await fetch('/v0/settings/store', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<{ status: string; message?: string }>(res)
}

export async function restartAfterStoreChange(): Promise<{ status: string }> {
  const res = await fetch('/v0/settings/store/restart', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ status: string }>(res)
}

export interface EventsWebhookConfig {
  url: string
  headers: Record<string, string>
}

export async function getEventsWebhook(): Promise<EventsWebhookConfig> {
  const res = await fetch('/v0/settings/events-webhook', { headers: authInit() })
  return parseJSON<EventsWebhookConfig>(res)
}

export async function putEventsWebhook(body: EventsWebhookConfig): Promise<EventsWebhookConfig> {
  const res = await fetch('/v0/settings/events-webhook', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<EventsWebhookConfig>(res)
}

export async function testEventsWebhook(): Promise<{ status: string }> {
  const res = await fetch('/v0/settings/events-webhook/test', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ status: string }>(res)
}

export interface EventsWebhookDelivery {
  id: string
  run_id: string
  kind: string
  event_index: number
  status: string
  attempt: number
  max_attempts: number
  last_error?: string
  next_retry_at: string
  created_at: string
  updated_at: string
}

export async function getEventsWebhookDeliveries(params?: {
  status?: string
  limit?: number
}): Promise<EventsWebhookDelivery[]> {
  const qs = new URLSearchParams()
  if (params?.status) qs.set('status', params.status)
  if (params?.limit != null) qs.set('limit', String(params.limit))
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  const res = await fetch(`/v0/settings/events-webhook/deliveries${suffix}`, {
    headers: authInit(),
  })
  const body = await parseJSON<{ deliveries: EventsWebhookDelivery[] }>(res)
  return body.deliveries ?? []
}

export async function retryEventsWebhookDelivery(id: string): Promise<{ status: string }> {
  const res = await fetch(`/v0/settings/events-webhook/deliveries/${encodeURIComponent(id)}/retry`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ status: string }>(res)
}

export type InboxChannel = {
  id: string
  agent_id: string
  enabled: boolean
  skills?: string[]
  description?: string
  webhook_url?: string
  webhook_headers?: Record<string, string>
  secret_hint?: string
}

export async function getInboxChannels(): Promise<InboxChannel[]> {
  const res = await fetch('/v0/settings/inbox-channels', { headers: authInit() })
  const body = await parseJSON<{ channels: InboxChannel[] }>(res)
  return body.channels ?? []
}

export async function putInboxChannels(channels: InboxChannel[]): Promise<void> {
  const res = await fetch('/v0/settings/inbox-channels', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ channels }),
  })
  await parseJSON<{ channels: InboxChannel[] }>(res)
}

export async function rotateInboxSecret(id: string): Promise<{ secret: string }> {
  const res = await fetch(`/v0/settings/inbox-channels/${encodeURIComponent(id)}/rotate-secret`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ secret: string }>(res)
}

export async function testInboxChannel(
  id: string,
): Promise<{ delivery_id: string; run_id: string }> {
  const res = await fetch(`/v0/settings/inbox-channels/${encodeURIComponent(id)}/test`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ delivery_id: string; run_id: string }>(res)
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

export async function cancelRun(runId: string): Promise<ResumeResponse> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/cancel`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<ResumeResponse>(res)
}

export function isTerminal(status: RunStatus): boolean {
  return status === 'succeeded' || status === 'failed' || status === 'cancelled'
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

export interface MCPConfig {
  transport: 'stdio' | 'http'
  command?: string
  args?: string[]
  env?: Record<string, string>
  url?: string
  headers?: Record<string, string>
}

export interface ConnectorAuth {
  mode?: string
  static?: { headers?: Record<string, string> }
  passthrough?: { headers?: string[] }
  vault_ref?: { headers?: Record<string, string> }
  capture?: {
    tool_name_glob?: string
    token_json_paths?: string[]
    label_json_paths?: string[]
    header_template?: string
    default_scheme?: string
  }
}

export type ImportFormat = 'auto' | 'openapi3' | 'swagger2' | 'postman'

export interface ConnectorInfo {
  id: string
  type: string
  spec?: string
  base_url?: string
  execution_callback_url?: string
  import_format_detected?: string
  auth?: ConnectorAuth
  mcp?: MCPConfig
  require_approval?: string[]
  require_login?: string[]
  tools?: ToolInfo[]
}

export interface PutConnectorBody {
  type: string
  spec?: string
  spec_content?: string
  spec_url?: string
  import_format?: ImportFormat
  base_url?: string
  execution_callback_url?: string
  auth?: ConnectorAuth
  mcp?: MCPConfig
  require_approval?: string[]
  require_login?: string[]
}

export class ApiError extends Error {
  readonly code: string
  readonly status: number

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

async function parseConnectorJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let code = 'unknown'
    let message = res.statusText
    try {
      const body = (await res.json()) as { error?: { code?: string; message?: string } }
      if (body.error?.code) code = body.error.code
      if (body.error?.message) message = body.error.message
    } catch {
      /* ignore */
    }
    throw new ApiError(res.status, code, message)
  }
  return (await res.json()) as T
}

export async function getConnector(id: string): Promise<ConnectorInfo> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(id)}`, { headers: authInit() })
  return parseConnectorJSON<ConnectorInfo>(res)
}

export async function putConnector(id: string, body: PutConnectorBody): Promise<ConnectorInfo> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseConnectorJSON<ConnectorInfo>(res)
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

export async function deleteConnector(id: string): Promise<void> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  if (res.status === 204) return
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
