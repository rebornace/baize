import { ApiError, authInit, parseJSON } from './http'

export type ToolExportMode = 'default' | 'force_allow' | 'force_deny'

export interface ToolInfo {
  name: string
  title?: string
  description?: string
  description_custom?: boolean
  connector_id: string
  operation_id?: string
  method?: string
  path?: string
  require_approval?: boolean
  require_login?: boolean
  enabled?: boolean
  source?: string
  /** MCP export override; empty omitted means default. */
  export?: ToolExportMode | ''
  input_schema?: Record<string, unknown>
}

export async function listTools(): Promise<ToolInfo[]> {
  const res = await fetch('/v0/tools', { headers: authInit() })
  const body = await parseJSON<{ tools: ToolInfo[] }>(res)
  return body.tools ?? []
}

export async function patchTool(
  name: string,
  body: {
    enabled?: boolean
    require_login?: boolean
    require_approval?: boolean
    title?: string
    description?: string
    export?: ToolExportMode
  },
): Promise<ToolInfo> {
  const res = await fetch(`/v0/tools/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<ToolInfo>(res)
}

export async function createConnectorTool(
  connectorId: string,
  body: {
    name: string
    method: string
    path: string
    title?: string
    description?: string
    input_schema?: Record<string, unknown>
  },
): Promise<ToolInfo> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(connectorId)}/tools`, {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<ToolInfo>(res)
}

/** MCP OAuth 公开字段；GET 不回显 token_bundle / client_secret。PUT 可带明文 client_secret。 */
export interface MCPOAuthConfig {
  status?: string
  client_id?: string
  /** 仅创建/更新时发送明文；GET 恒为空。 */
  client_secret?: string
  /** 前端不得从表单写回；GET 亦已脱敏。 */
  token_bundle?: string
  authorization_endpoint?: string
  token_endpoint?: string
  registration_endpoint?: string
  resource_metadata_url?: string
}

export interface MCPConfig {
  transport: 'stdio' | 'http'
  command?: string
  args?: string[]
  env?: Record<string, string>
  url?: string
  headers?: Record<string, string>
  export_db_readonly?: boolean
  oauth?: MCPOAuthConfig
}

export interface MCPOAuthStartResult {
  authorization_url: string
}

export interface ConnectorAuth {
  mode?: string
  static?: { headers?: Record<string, string> }
  passthrough?: { headers?: string[] }
  vault_ref?: { headers?: Record<string, string> }
  capture?: {
    tool_name_glob?: string
    token_json_paths?: string[]
    label_json_paths?: string[]
    header_template?: string
    default_scheme?: string
  }
}

export type ImportFormat = 'auto' | 'openapi3' | 'swagger2' | 'postman'

export interface ConnectorInfo {
  id: string
  type: string
  spec?: string
  base_url?: string
  execution_callback_url?: string
  import_format_detected?: string
  auth?: ConnectorAuth
  mcp?: MCPConfig
  require_approval?: string[]
  require_login?: string[]
  tools?: ToolInfo[]
}

export interface PutConnectorBody {
  type: string
  spec?: string
  spec_content?: string
  spec_url?: string
  import_format?: ImportFormat
  base_url?: string
  execution_callback_url?: string
  auth?: ConnectorAuth
  mcp?: MCPConfig
  require_approval?: string[]
  require_login?: string[]
}

async function parseConnectorJSON<T>(res: Response): Promise<T> {
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
    throw new ApiError(res.status, code, message)
  }
  return (await res.json()) as T
}

export async function listConnectors(type?: string): Promise<ConnectorInfo[]> {
  const q = type ? `?type=${encodeURIComponent(type)}` : ''
  const res = await fetch(`/v0/connectors${q}`, { headers: authInit() })
  const body = await parseConnectorJSON<{ connectors: ConnectorInfo[] }>(res)
  return body.connectors ?? []
}

export async function getConnector(id: string): Promise<ConnectorInfo> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(id)}`, { headers: authInit() })
  return parseConnectorJSON<ConnectorInfo>(res)
}

export async function putConnector(id: string, body: PutConnectorBody): Promise<ConnectorInfo> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseConnectorJSON<ConnectorInfo>(res)
}

export async function deleteConnectorTool(connectorId: string, name: string): Promise<void> {
  const res = await fetch(
    `/v0/connectors/${encodeURIComponent(connectorId)}/tools/${encodeURIComponent(name)}`,
    { method: 'DELETE', headers: authInit() },
  )
  if (!res.ok) {
    let detail = res.statusText
    try {
      const errBody = (await res.json()) as { error?: { message?: string } }
      if (errBody.error?.message) detail = errBody.error.message
    } catch {
      /* ignore */
    }
    throw new Error(`HTTP ${res.status}: ${detail}`)
  }
}

export async function startMcpOAuth(id: string): Promise<MCPOAuthStartResult> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(id)}/mcp/oauth/start`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseConnectorJSON<MCPOAuthStartResult>(res)
}

export async function disconnectMcpOAuth(id: string): Promise<{ id: string; status: string }> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(id)}/mcp/oauth/disconnect`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseConnectorJSON<{ id: string; status: string }>(res)
}

export async function deleteConnector(id: string): Promise<void> {
  const res = await fetch(`/v0/connectors/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  if (res.status === 204) return
  if (!res.ok) {
    let detail = res.statusText
    try {
      const errBody = (await res.json()) as { error?: { message?: string } }
      if (errBody.error?.message) detail = errBody.error.message
    } catch {
      /* ignore */
    }
    throw new Error(`HTTP ${res.status}: ${detail}`)
  }
}
