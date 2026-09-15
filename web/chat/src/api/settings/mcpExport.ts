import { authInit, parseJSON } from '../http'

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
