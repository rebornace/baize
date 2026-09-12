import { TOOLS } from '../strings'
import type { CaptureDraft } from './captureForm'

export interface CaptureSettingsFieldsProps {
  connectorId: string
  connectorType: 'openapi' | 'http'
  draft: CaptureDraft
  onDraftChange: (patch: Partial<CaptureDraft>) => void
}

const CAPTURE_HINT: Record<CaptureSettingsFieldsProps['connectorType'], string> = {
  http: '名称匹配的登录功能成功后，会把返回里的令牌写入本会话身份。',
  openapi: '匹配的登录接口返回后，会把令牌写入本会话身份。',
}

export function CaptureSettingsFields({
  connectorType,
  draft,
  onDraftChange,
}: CaptureSettingsFieldsProps) {
  return (
    <>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureToolGlob}</span>
        <input
          className="settings-input"
          value={draft.toolNameGlob}
          onChange={(e) => onDraftChange({ toolNameGlob: e.target.value })}
          placeholder="*login*（__none__ 关闭）"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureTokenPaths}</span>
        <textarea
          className="settings-input"
          rows={3}
          value={draft.tokenPathsText}
          onChange={(e) => onDraftChange({ tokenPathsText: e.target.value })}
          placeholder="accessToken&#10;data.token"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureLabelPaths}</span>
        <textarea
          className="settings-input"
          rows={2}
          value={draft.labelPathsText}
          onChange={(e) => onDraftChange({ labelPathsText: e.target.value })}
          placeholder="email"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureHeaderTemplate}</span>
        <input
          className="settings-input"
          value={draft.headerTemplate}
          onChange={(e) => onDraftChange({ headerTemplate: e.target.value })}
          placeholder="Bearer {{token}}"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureDefaultScheme}</span>
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
