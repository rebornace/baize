export type ConnectorKind = 'openapi' | 'plugin'

export interface ConnectionFormValues {
  kind: ConnectorKind
  id: string
  baseUrl: string
  /** 业务系统：本次是否提供了新文档（文件内容或非空 URL）。 */
  hasSpec: boolean
  /** 编辑既有连接时为 true，允许不带新文档保存。 */
  editing?: boolean
}

export type FieldErrors = Partial<Record<'id' | 'baseUrl' | 'spec', string>>

export type PermissionFlag = 'login' | 'approval'

export interface ToolPermission {
  login: boolean
  approval: boolean
}

/** key 为工具名。 */
export type PermissionSelection = Record<string, ToolPermission>
