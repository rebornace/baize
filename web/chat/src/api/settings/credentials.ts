import { authInit, parseJSON } from '../http'

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
