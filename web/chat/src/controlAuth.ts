export const CONTROL_TOKEN_KEY = 'baize.control_token'

export function readControlToken(): string {
  return localStorage.getItem(CONTROL_TOKEN_KEY)?.trim() ?? ''
}

export function writeControlToken(token: string): void {
  localStorage.setItem(CONTROL_TOKEN_KEY, token)
}

export function clearControlToken(): void {
  localStorage.removeItem(CONTROL_TOKEN_KEY)
}

export function authHeaders(gateEnabled: boolean): HeadersInit {
  if (!gateEnabled) return {}
  const token = readControlToken()
  if (!token) return {}
  return { Authorization: `Bearer ${token}` }
}
