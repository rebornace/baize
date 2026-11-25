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
