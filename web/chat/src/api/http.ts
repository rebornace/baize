import { authHeaders } from '../controlAuth'

let gateEnabled = false

export function setGateEnabled(v: boolean): void {
  gateEnabled = v
}

export function authInit(extra?: Record<string, string>): HeadersInit {
  return { ...(authHeaders(gateEnabled) as Record<string, string>), ...extra }
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

export async function parseJSON<T>(res: Response): Promise<T> {
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
    // 保留 "HTTP <status>:" 前缀，兼容 GateRoot 的 startsWith('HTTP 401:') 判断。
    throw new ApiError(res.status, code, `HTTP ${res.status}: ${message}`)
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
