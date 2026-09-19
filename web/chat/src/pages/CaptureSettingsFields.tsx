import { TOOLS } from '../strings'
import type { CaptureDraft } from './captureForm'

export interface CaptureSettingsFieldsProps {
  connectorId: string
  connectorType: 'openapi' | 'http'
  draft: CaptureDraft
  onDraftChange: (patch: Partial<CaptureDraft>) => void
}

export function CaptureSettingsFields({
  draft,
  onDraftChange,
}: CaptureSettingsFieldsProps) {
  return (
    <div className="settings-capture-fields">
      <p className="settings-hint">{TOOLS.captureIntro}</p>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureToolGlob}</span>
        <input
          className="settings-input"
          value={draft.toolNameGlob}
          onChange={(e) => onDraftChange({ toolNameGlob: e.target.value })}
          placeholder={TOOLS.phCaptureGlob}
        />
        <span className="settings-hint">{TOOLS.captureToolGlobHint}</span>
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureTokenPaths}</span>
        <textarea
          className="settings-input"
          rows={3}
          value={draft.tokenPathsText}
          onChange={(e) => onDraftChange({ tokenPathsText: e.target.value })}
          placeholder={TOOLS.phCaptureTokenPaths}
        />
        <span className="settings-hint">{TOOLS.captureTokenPathsHint}</span>
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureLabelPaths}</span>
        <textarea
          className="settings-input"
          rows={2}
          value={draft.labelPathsText}
          onChange={(e) => onDraftChange({ labelPathsText: e.target.value })}
          placeholder={TOOLS.phCaptureLabelPaths}
        />
        <span className="settings-hint">{TOOLS.captureLabelPathsHint}</span>
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureHeaderTemplate}</span>
        <input
          className="settings-input"
          value={draft.headerTemplate}
          onChange={(e) => onDraftChange({ headerTemplate: e.target.value })}
          placeholder={TOOLS.phCaptureHeader}
        />
        <span className="settings-hint">{TOOLS.captureHeaderTemplateHint}</span>
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.captureDefaultScheme}</span>
        <input
          className="settings-input"
          value={draft.defaultScheme}
          onChange={(e) => onDraftChange({ defaultScheme: e.target.value })}
          placeholder={TOOLS.phCaptureScheme}
        />
        <span className="settings-hint">{TOOLS.captureDefaultSchemeHint}</span>
      </label>
    </div>
  )
}
