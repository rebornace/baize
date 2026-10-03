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
