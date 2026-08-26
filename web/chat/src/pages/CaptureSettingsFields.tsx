import type { CaptureDraft } from './captureForm'

export interface CaptureSettingsFieldsProps {
  connectorId: string
  connectorType: 'openapi' | 'http'
  draft: CaptureDraft
  onDraftChange: (patch: Partial<CaptureDraft>) => void
}

const CAPTURE_HINT: Record<CaptureSettingsFieldsProps['connectorType'], string> = {
  http: '登录捕获：侧车中名称匹配 glob 的工具 invoke 成功后写入会话身份（如 login）。',
  openapi: '登录捕获：匹配 glob 的 operation 响应 JSON 写入会话身份。',
}

export function CaptureSettingsFields({
  connectorType,
  draft,
  onDraftChange,
}: CaptureSettingsFieldsProps) {
  return (
    <>
      <label className="settings-field">
        <span className="settings-label">登录捕获 tool_name_glob</span>
        <input
          className="settings-input"
          value={draft.toolNameGlob}
          onChange={(e) => onDraftChange({ toolNameGlob: e.target.value })}
          placeholder="*login*（__none__ 关闭）"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">token_json_paths（每行一条）</span>
        <textarea
          className="settings-input"
          rows={3}
          value={draft.tokenPathsText}
          onChange={(e) => onDraftChange({ tokenPathsText: e.target.value })}
          placeholder="accessToken&#10;data.token"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">label_json_paths（每行一条）</span>
        <textarea
          className="settings-input"
          rows={2}
          value={draft.labelPathsText}
          onChange={(e) => onDraftChange({ labelPathsText: e.target.value })}
          placeholder="email"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">header_template</span>
        <input
          className="settings-input"
          value={draft.headerTemplate}
          onChange={(e) => onDraftChange({ headerTemplate: e.target.value })}
          placeholder="Bearer {{token}}"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">default_scheme（可选）</span>
        <input
          className="settings-input"
          value={draft.defaultScheme}
          onChange={(e) => onDraftChange({ defaultScheme: e.target.value })}
          placeholder="bearer"
        />
      </label>
      <p className="settings-hint">{CAPTURE_HINT[connectorType]}</p>
    </>
  )
}
