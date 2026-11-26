import { authInit, parseJSON } from '../http'

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

export interface AttachmentRetention {
  days: number
}

export async function getAttachmentRetention(): Promise<AttachmentRetention> {
  const res = await fetch('/v0/settings/attachment-retention', { headers: authInit() })
  return parseJSON<AttachmentRetention>(res)
}

export async function putAttachmentRetention(days: number): Promise<AttachmentRetention> {
  const res = await fetch('/v0/settings/attachment-retention', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ days }),
  })
  return parseJSON<AttachmentRetention>(res)
}

export interface WorkspaceAttachmentRetentionItem {
  workspace_id: string
  name: string
  days: number
  effective_days: number
}

export interface WorkspaceAttachmentRetention {
  global_days: number
  items: WorkspaceAttachmentRetentionItem[]
}

export async function getWorkspaceAttachmentRetention(): Promise<WorkspaceAttachmentRetention> {
  const res = await fetch('/v0/settings/workspace-attachment-retention', {
    headers: authInit(),
  })
  return parseJSON<WorkspaceAttachmentRetention>(res)
}

export async function putWorkspaceAttachmentRetention(
  workspaceId: string,
  days: number,
): Promise<WorkspaceAttachmentRetentionItem & { global_days: number }> {
  const res = await fetch('/v0/settings/workspace-attachment-retention', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ workspace_id: workspaceId, days }),
  })
  return parseJSON(res)
}

export interface AgentRunRetentionItem {
  agent_id: string
  days: number
}

export interface AgentRunRetention {
  global_days: number
  items: AgentRunRetentionItem[]
}

export async function getAgentRunRetention(): Promise<AgentRunRetention> {
  const res = await fetch('/v0/settings/agent-run-retention', { headers: authInit() })
  return parseJSON<AgentRunRetention>(res)
}

export async function putAgentRunRetention(
  agentId: string,
  days: number,
): Promise<AgentRunRetentionItem> {
  const res = await fetch('/v0/settings/agent-run-retention', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ agent_id: agentId, days }),
  })
  return parseJSON<AgentRunRetentionItem>(res)
}

export interface RunRetention {
  days: number
}

export async function getRunRetention(): Promise<RunRetention> {
  const res = await fetch('/v0/settings/run-retention', { headers: authInit() })
  return parseJSON<RunRetention>(res)
}

export async function putRunRetention(days: number): Promise<RunRetention> {
  const res = await fetch('/v0/settings/run-retention', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ days }),
  })
  return parseJSON<RunRetention>(res)
}

export interface MessageRetention {
  days: number
}

export async function getMessageRetention(): Promise<MessageRetention> {
  const res = await fetch('/v0/settings/message-retention', { headers: authInit() })
  return parseJSON<MessageRetention>(res)
}

export async function putMessageRetention(days: number): Promise<MessageRetention> {
  const res = await fetch('/v0/settings/message-retention', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ days }),
  })
  return parseJSON<MessageRetention>(res)
}

export interface WorkspaceRetentionItem {
  workspace_id: string
  name: string
  message_days: number
  run_days: number
  effective_message_days: number
  effective_run_days: number
}

export interface WorkspaceRetention {
  global_message_days: number
  global_run_days: number
  items: WorkspaceRetentionItem[]
}

export async function getWorkspaceRetention(): Promise<WorkspaceRetention> {
  const res = await fetch('/v0/settings/workspace-retention', { headers: authInit() })
  return parseJSON<WorkspaceRetention>(res)
}

export async function putWorkspaceRetention(
  workspaceId: string,
  patch: { message_days?: number; run_days?: number },
): Promise<WorkspaceRetentionItem> {
  const res = await fetch('/v0/settings/workspace-retention', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ workspace_id: workspaceId, ...patch }),
  })
  return parseJSON<WorkspaceRetentionItem>(res)
}

export interface RetentionPreviewConversation {
  conversation_id: string
  workspace_id?: string
  days: number
  count: number
}

export interface RetentionPreviewAgent {
  agent_id: string
  days: number
  count: number
}

export interface RetentionPreview {
  messages: {
    total: number
    conversations: RetentionPreviewConversation[]
  }
  runs: {
    total: number
    conversations: RetentionPreviewConversation[]
    agents: RetentionPreviewAgent[]
    blocked?: boolean
  }
  archived: {
    total: number
    days: number
    conversations: {
      conversation_id: string
      workspace_id?: string
      archived_at: string
    }[]
  }
}

export async function getRetentionPreview(): Promise<RetentionPreview> {
  const res = await fetch('/v0/settings/retention/preview', { headers: authInit() })
  return parseJSON<RetentionPreview>(res)
}

export async function runRetentionNow(): Promise<{
  messages_deleted: number
  runs_deleted: number
  archived_deleted: number
}> {
  const res = await fetch('/v0/settings/retention/run', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON(res)
}

export interface ArchiveRetention {
  days: number
}

export async function getArchiveRetention(): Promise<ArchiveRetention> {
  const res = await fetch('/v0/settings/archive-retention', { headers: authInit() })
  return parseJSON<ArchiveRetention>(res)
}

export async function putArchiveRetention(days: number): Promise<ArchiveRetention> {
  const res = await fetch('/v0/settings/archive-retention', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ days }),
  })
  return parseJSON<ArchiveRetention>(res)
}
