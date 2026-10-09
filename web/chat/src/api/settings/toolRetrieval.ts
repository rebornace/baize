import { authInit, parseJSON } from '../http'

export type ToolRetrievalPhase =
  | 'idle'
  | 'checking'
  | 'downloading_installer'
  | 'launching_installer'
  | 'waiting_ollama'
  | 'pulling_model'
  | 'probing'
  | 'ready'
  | 'failed'
  | 'disabled'

export type ToolRetrievalProvider = 'local' | 'api' | string

export interface ToolRetrievalInstaller {
  goos: string
  mode: 'windows_exe' | 'open_download_page' | string
  installer_url?: string
  mirror_urls?: string[]
  download_page_url: string
}

export interface ToolRetrievalDownload {
  mirror?: string
  bytes: number
  total: number
  percent: number
  note?: string
}

export interface ToolRetrievalPaths {
  app_dir?: string
  config_dir?: string
  models_dir: string
  models_dir_source: 'baize' | 'env' | 'default' | string
  installer_cache_dir: string
}

export interface ToolRetrievalStatus {
  mode: 'standard' | 'enhanced' | string
  phase: ToolRetrievalPhase
  detail?: string
  error?: string
  provider: ToolRetrievalProvider
  ollama_installed: boolean
  ollama_running: boolean
  embedding_ok: boolean
  model: string
  base_url: string
  ollama_base_url: string
  openai_base_url: string
  api_key_set: boolean
  model_present: boolean
  installer_cache_bytes?: number
  paths: ToolRetrievalPaths
  installer: ToolRetrievalInstaller
  download?: ToolRetrievalDownload | null
  busy: boolean
}

export interface ToolRetrievalCleanupReport {
  disabled: boolean
  removed_model: boolean
  removed_ollama: boolean
  installer_cache_bytes_removed: number
  cleared_setting: boolean
  notes?: string[]
}

export interface ToolRetrievalAPIConfig {
  provider: 'api'
  base_url: string
  model: string
  api_key?: string
}

export async function getToolRetrieval(): Promise<ToolRetrievalStatus> {
  const res = await fetch('/v0/settings/tool-retrieval', { headers: authInit() })
  return parseJSON<ToolRetrievalStatus>(res)
}

/** Local Ollama one-click install (async). */
export async function enableToolRetrieval(): Promise<ToolRetrievalStatus> {
  const res = await fetch('/v0/settings/tool-retrieval/enable', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<ToolRetrievalStatus>(res)
}

/** Cloud / self-hosted OpenAI-compatible embeddings. */
export async function enableToolRetrievalAPI(
  cfg: Omit<ToolRetrievalAPIConfig, 'provider'>,
): Promise<ToolRetrievalStatus> {
  const res = await fetch('/v0/settings/tool-retrieval/enable', {
    method: 'POST',
    headers: { ...authInit(), 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider: 'api', ...cfg }),
  })
  return parseJSON<ToolRetrievalStatus>(res)
}

export async function disableToolRetrieval(): Promise<ToolRetrievalStatus> {
  const res = await fetch('/v0/settings/tool-retrieval/disable', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<ToolRetrievalStatus>(res)
}

export async function cleanupToolRetrieval(opts?: {
  remove_ollama?: boolean
}): Promise<{
  cleanup: ToolRetrievalCleanupReport
  status: ToolRetrievalStatus
}> {
  const res = await fetch('/v0/settings/tool-retrieval/cleanup', {
    method: 'POST',
    headers: { ...authInit(), 'Content-Type': 'application/json' },
    body: JSON.stringify(opts ?? {}),
  })
  return parseJSON(res)
}

export async function setToolRetrievalModelsDir(modelsDir: string): Promise<ToolRetrievalStatus> {
  const res = await fetch('/v0/settings/tool-retrieval/paths', {
    method: 'PUT',
    headers: { ...authInit(), 'Content-Type': 'application/json' },
    body: JSON.stringify({ models_dir: modelsDir }),
  })
  return parseJSON<ToolRetrievalStatus>(res)
}
