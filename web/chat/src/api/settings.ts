import { authInit, parseJSON } from './http'
import type { ThinkingLevel } from './types'

export interface WeixinChannelSettings {
  agent_id: string
  allowlist: string[]
  assignee: string
  enabled: boolean
  /** Runtime reconciled state, only present on the PUT response. */
  running?: boolean
  /** Why the channel is not running: "login_required" | "start_failed". */
  reason?: string
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

/**
 * Adapter process lifecycle control. These act on the OS process (start / stop
 * / restart the weixin-adapter), distinct from the enabled toggle which only
 * starts/stops polling. Each returns the reconciled settings + running state.
 */
async function postWeixinProcess(action: 'start' | 'stop' | 'restart'): Promise<WeixinChannelSettings> {
  const res = await fetch(`/v0/settings/channels/weixin/process/${action}`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<WeixinChannelSettings>(res)
}

export const startWeixinProcess = () => postWeixinProcess('start')
export const stopWeixinProcess = () => postWeixinProcess('stop')
export const restartWeixinProcess = () => postWeixinProcess('restart')

export type ChannelOutboundDelivery = {
  id: string
  status: string
  kind: string
  peer_id?: string
  conversation_id?: string
  run_id?: string
  attempt: number
  max_attempts: number
  last_error?: string
  updated_at?: string
}

export async function getChannelOutboundDeliveries(
  channel: string,
  params?: { status?: string; limit?: number },
): Promise<ChannelOutboundDelivery[]> {
  const qs = new URLSearchParams()
  if (params?.status) qs.set('status', params.status)
  if (params?.limit != null) qs.set('limit', String(params.limit))
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  const res = await fetch(
    `/v0/settings/channels/${encodeURIComponent(channel)}/outbound-deliveries${suffix}`,
    { headers: authInit() },
  )
  const body = await parseJSON<{ deliveries: ChannelOutboundDelivery[] }>(res)
  return body.deliveries ?? []
}

export async function retryChannelOutboundDelivery(
  channel: string,
  id: string,
): Promise<{ status: string }> {
  const res = await fetch(
    `/v0/settings/channels/${encodeURIComponent(channel)}/outbound-deliveries/${encodeURIComponent(id)}/retry`,
    {
      method: 'POST',
      headers: authInit(),
    },
  )
  return parseJSON<{ status: string }>(res)
}

// --- Runtime hot-reload settings (engine knobs + control-plane credentials) ---

/** Effective engine knobs (duration expressed in seconds on the wire). */
export interface RuntimeKnobs {
  max_messages: number
  max_steps: number
  tool_timeout_seconds: number
  compaction_enabled: boolean
  compact_threshold: number
  compact_reserve_tokens: number
  compact_keep_recent: number
  compact_summary_timeout_seconds: number
  memory_enabled: boolean
  memory_auto_extract: boolean
}

/** Per-field flags: true when the value is overridden from the YAML baseline. */
export type RuntimeKnobsOverrides = Record<keyof RuntimeKnobs, boolean>

export interface RuntimeKnobsView {
  effective: RuntimeKnobs
  overridden: RuntimeKnobsOverrides
  public_base_url: string
  public_base_url_overridden: boolean
}

/** Partial engine-knob update; omitted fields are left unchanged.
 * public_base_url: omit = leave; "" = clear override to YAML; non-empty = set. */
export type RuntimeKnobsPatch = Partial<{
  max_messages: number
  max_steps: number
  tool_timeout_seconds: number
  compaction_enabled: boolean
  compact_threshold: number
  compact_reserve_tokens: number
  compact_keep_recent: number
  compact_summary_timeout_seconds: number
  memory_enabled: boolean
  memory_auto_extract: boolean
  public_base_url: string
}>

// --- Account memory settings (CRUD for current control-plane subject) ---

export interface MemoryEntry {
  id: string
  owner_id: string
  key?: string
  text: string
  source: string
  created_at: string
  updated_at: string
}

export async function listMemory(opts?: {
  limit?: number
  offset?: number
}): Promise<MemoryEntry[]> {
  const qs = new URLSearchParams()
  if (opts?.limit != null) qs.set('limit', String(opts.limit))
  if (opts?.offset != null) qs.set('offset', String(opts.offset))
  const suffix = qs.toString() ? `?${qs}` : ''
  const res = await fetch(`/v0/settings/memory${suffix}`, { headers: authInit() })
  const body = await parseJSON<{ items: MemoryEntry[] }>(res)
  return body.items ?? []
}

export async function createMemory(body: {
  text: string
  key?: string
}): Promise<MemoryEntry> {
  const res = await fetch('/v0/settings/memory', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<MemoryEntry>(res)
}

export async function patchMemory(
  id: string,
  body: { text?: string; key?: string },
): Promise<MemoryEntry> {
  const res = await fetch(`/v0/settings/memory/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<MemoryEntry>(res)
}

export async function deleteMemory(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/memory/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}

export async function getRuntimeSettings(): Promise<RuntimeKnobsView> {
  const res = await fetch('/v0/settings/runtime', { headers: authInit() })
  return parseJSON<RuntimeKnobsView>(res)
}

export async function patchRuntimeSettings(
  body: RuntimeKnobsPatch,
): Promise<RuntimeKnobsView> {
  const res = await fetch('/v0/settings/runtime', {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<RuntimeKnobsView>(res)
}

/** A masked operator entry (id + source; never contains a token). */
export interface CredentialOperatorView {
  id: string
  source: 'config' | 'runtime'
}

export interface CredentialsView {
  source: 'config' | 'override'
  operator_set: boolean
  admin_set: boolean
  operators: CredentialOperatorView[]
}

export interface CredentialOperatorInput {
  id: string
  token: string
}

/** Partial control-plane credential update. Empty strings are sent verbatim. */
export interface CredentialsPatch {
  operator_token?: string
  admin_token?: string
  add_operators?: CredentialOperatorInput[]
  remove_operators?: string[]
  reset?: boolean
}

export async function getCredentials(): Promise<CredentialsView> {
  const res = await fetch('/v0/settings/credentials', { headers: authInit() })
  return parseJSON<CredentialsView>(res)
}

export async function patchCredentials(body: CredentialsPatch): Promise<CredentialsView> {
  const res = await fetch('/v0/settings/credentials', {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<CredentialsView>(res)
}

export interface StoreSettings {
  driver: string
  effective_driver?: string
  store_config_mismatch?: boolean
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

export interface MCPExportSettings {
  enabled: boolean
  endpoint_path: string
}

export interface MCPExportIdentity {
  id: string
  name: string
  scheme?: string
  headers?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export interface MCPExportKey {
  id: string
  name: string
  identity_id: string
  prefix: string
  revoked_at?: string | null
  created_at?: string
}

export interface MCPExportKeyCreated {
  id: string
  name: string
  identity_id: string
  token: string
  prefix: string
}

export async function getMCPExportSettings(): Promise<MCPExportSettings> {
  const res = await fetch('/v0/settings/mcp-export', { headers: authInit() })
  return parseJSON<MCPExportSettings>(res)
}

export async function listMCPExportIdentities(): Promise<MCPExportIdentity[]> {
  const res = await fetch('/v0/settings/mcp-export/identities', { headers: authInit() })
  return parseJSON<MCPExportIdentity[]>(res)
}

export async function createMCPExportIdentity(body: {
  id?: string
  name: string
  scheme?: string
  headers?: Record<string, string>
}): Promise<MCPExportIdentity> {
  const res = await fetch('/v0/settings/mcp-export/identities', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<MCPExportIdentity>(res)
}

export async function patchMCPExportIdentity(
  id: string,
  body: {
    name?: string
    scheme?: string
    headers?: Record<string, string>
  },
): Promise<MCPExportIdentity> {
  const res = await fetch(`/v0/settings/mcp-export/identities/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<MCPExportIdentity>(res)
}

export async function deleteMCPExportIdentity(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/mcp-export/identities/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}

export async function listMCPExportKeys(identityId?: string): Promise<MCPExportKey[]> {
  const q = identityId?.trim()
    ? `?identity_id=${encodeURIComponent(identityId.trim())}`
    : ''
  const res = await fetch(`/v0/settings/mcp-export/keys${q}`, { headers: authInit() })
  return parseJSON<MCPExportKey[]>(res)
}

export async function createMCPExportKey(body: {
  name: string
  identity_id: string
}): Promise<MCPExportKeyCreated> {
  const res = await fetch('/v0/settings/mcp-export/keys', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<MCPExportKeyCreated>(res)
}

export async function revokeMCPExportKey(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/mcp-export/keys/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}

/** Wire dialect for thinking fields; `auto` lets the server infer. */
export type ThinkingDialect = 'auto' | 'openai' | 'deepseek' | 'qwen' | 'omit'

export interface ModelProfile {
  id: string
  name: string
  provider: string
  base_url: string
  model: string
  /** Redacted mask in list/detail responses; never sent verbatim by the server. */
  api_key?: string
  api_key_env?: string
  /** Derived: thinking_level === 'off'. Kept for older clients. */
  disable_thinking: boolean
  thinking_level: ThinkingLevel
  thinking_dialect: ThinkingDialect
  supports_vision: boolean
  context_tokens: number
  /** Auto-routing capability tier: "light" | "standard" | "power". */
  auto_tier: ModelTier
  created_at?: string
  updated_at?: string
}

/** Capability tiers understood by the task-aware Auto router. */
export type ModelTier = 'light' | 'standard' | 'power'

/** Editable fields of a model profile. Booleans omitted on PATCH are kept.
 * `auto_tier` may also be "auto" on write to request server-side inference. */
export type ModelProfileInput = Partial<{
  name: string
  provider: string
  base_url: string
  model: string
  api_key: string
  api_key_env: string
  disable_thinking: boolean
  thinking_level: ThinkingLevel
  thinking_dialect: ThinkingDialect
  supports_vision: boolean
  context_tokens: number
  auto_tier: ModelTier | 'auto'
}>

export async function listModelProfiles(): Promise<ModelProfile[]> {
  const res = await fetch('/v0/settings/models', { headers: authInit() })
  const body = await parseJSON<{ profiles: ModelProfile[] }>(res)
  return body.profiles ?? []
}

export async function createModelProfile(p: ModelProfileInput): Promise<ModelProfile> {
  const res = await fetch('/v0/settings/models', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(p),
  })
  const body = await parseJSON<{ profile: ModelProfile }>(res)
  return body.profile
}

export async function updateModelProfile(
  id: string,
  p: ModelProfileInput,
): Promise<ModelProfile> {
  const res = await fetch(`/v0/settings/models/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(p),
  })
  const body = await parseJSON<{ profile: ModelProfile }>(res)
  return body.profile
}

export async function deleteModelProfile(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/models/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}
