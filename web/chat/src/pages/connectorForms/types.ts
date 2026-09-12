import type { ImportFormat, MCPConfig } from '../../api'

export type ConnectorKind = 'openapi' | 'plugin' | 'mcp'

export interface ConnectionFormValues {
  kind: ConnectorKind
  id: string
  baseUrl: string
  /** 业务系统：本次是否提供了新文档（文件内容或非空 URL）。 */
  hasSpec: boolean
  /** 编辑既有连接时为 true，允许不带新文档保存。 */
  editing?: boolean
}

/** 旧两页第一步字段错误键；MCP 使用 McpFieldErrors。 */
export type FieldErrors = Partial<Record<'id' | 'baseUrl' | 'spec', string>>

/** 第一步保存成功后的判别联合连接负载（供第二步整表回传）。 */
export type SavedConnection =
  | { kind: 'openapi'; id: string; baseUrl: string; spec?: { content?: string; url?: string }; importFormat: ImportFormat }
  | { kind: 'plugin'; id: string; baseUrl: string }
  | { kind: 'mcp'; id: string; mcp: MCPConfig }

export type PermissionFlag = 'login' | 'approval'

export interface ToolPermission {
  login: boolean
  approval: boolean
}

/** key 为工具名。 */
export type PermissionSelection = Record<string, ToolPermission>
