import { authInit, parseJSON } from '../http'
import type { ToolRetrievalDownload, ToolRetrievalInstaller, ToolRetrievalPhase } from './toolRetrieval'

export type SystemOnePhase = ToolRetrievalPhase

export type SystemOneProvider = 'local' | 'api' | string

export interface SystemOneStatus {
  mode: 'off' | 'on' | string
  phase: SystemOnePhase
  detail?: string
  error?: string
  provider: SystemOneProvider
  ollama_installed: boolean
  ollama_running: boolean
  systemone_ok: boolean
  model: string
  base_url: string
  api_key_set: boolean
  model_present: boolean
  installer: ToolRetrievalInstaller
  download?: ToolRetrievalDownload | null
  busy: boolean
}

export interface SystemOneCleanupReport {
  disabled: boolean
  removed_model: boolean
  cleared_setting: boolean
  notes?: string[]
}

export interface SystemOneAPIConfig {
  provider: 'api'
  base_url: string
  model: string
  api_key?: string
}

export async function getSystemOne(): Promise<SystemOneStatus> {
  const res = await fetch('/v0/settings/systemone', { headers: authInit() })
  return parseJSON<SystemOneStatus>(res)
}

/** Local Ollama foolproof install + pull tev1 (async). */
export async function enableSystemOneLocal(): Promise<SystemOneStatus> {
  const res = await fetch('/v0/settings/systemone/enable', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<SystemOneStatus>(res)
}

/** Cloud / self-hosted System One API. */
export async function enableSystemOneAPI(
  cfg: Omit<SystemOneAPIConfig, 'provider'>,
): Promise<SystemOneStatus> {
  const res = await fetch('/v0/settings/systemone/enable', {
    method: 'POST',
    headers: { ...authInit(), 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider: 'api', ...cfg }),
  })
  return parseJSON<SystemOneStatus>(res)
}

export async function disableSystemOne(): Promise<SystemOneStatus> {
  const res = await fetch('/v0/settings/systemone/disable', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<SystemOneStatus>(res)
}

export async function cleanupSystemOne(): Promise<{
  cleanup: SystemOneCleanupReport
  status: SystemOneStatus
}> {
  const res = await fetch('/v0/settings/systemone/cleanup', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON(res)
}
