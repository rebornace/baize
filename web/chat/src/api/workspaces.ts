import { authInit, parseJSON } from './http'

export interface Workspace {
  id: string
  name: string
  created_at?: string
}

export async function listWorkspaces(): Promise<Workspace[]> {
  const res = await fetch('/v0/workspaces', { headers: authInit() })
  const body = await parseJSON<{ workspaces: Workspace[] }>(res)
  return body.workspaces ?? []
}

export async function createWorkspace(name: string): Promise<Workspace> {
  const res = await fetch('/v0/workspaces', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ name }),
  })
  return parseJSON<Workspace>(res)
}
