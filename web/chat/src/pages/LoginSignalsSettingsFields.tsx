import { TOOLS } from '../strings'
import type { LoginSignalsDraft } from './loginSignalsForm'

export interface LoginSignalsSettingsFieldsProps {
  draft: LoginSignalsDraft
  onDraftChange: (patch: Partial<LoginSignalsDraft>) => void
}

export function LoginSignalsSettingsFields({
  draft,
  onDraftChange,
}: LoginSignalsSettingsFieldsProps) {
  return (
    <div className="settings-capture-fields" data-testid="login-signals-fields">
      <p className="settings-hint">{TOOLS.loginSignalsIntro}</p>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.loginSignalsHttpStatuses}</span>
        <textarea
          className="settings-input"
          rows={2}
          value={draft.httpStatusesText}
          onChange={(e) => onDraftChange({ httpStatusesText: e.target.value })}
          placeholder={TOOLS.phLoginSignalsHttp}
        />
        <span className="settings-hint">{TOOLS.loginSignalsHttpStatusesHint}</span>
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.loginSignalsHttpNeedSignal}</span>
        <textarea
          className="settings-input"
          rows={2}
          value={draft.httpStatusesNeedSignalText}
          onChange={(e) => onDraftChange({ httpStatusesNeedSignalText: e.target.value })}
          placeholder={TOOLS.phLoginSignalsHttpNeed}
        />
        <span className="settings-hint">{TOOLS.loginSignalsHttpNeedSignalHint}</span>
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.loginSignalsBodyCodePath}</span>
        <input
          className="settings-input"
          value={draft.bodyCodePath}
          onChange={(e) => onDraftChange({ bodyCodePath: e.target.value })}
          placeholder={TOOLS.phLoginSignalsBodyPath}
        />
        <span className="settings-hint">{TOOLS.loginSignalsBodyCodePathHint}</span>
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.loginSignalsBodyCodes}</span>
        <textarea
          className="settings-input"
          rows={3}
          value={draft.bodyCodesText}
          onChange={(e) => onDraftChange({ bodyCodesText: e.target.value })}
          placeholder={TOOLS.phLoginSignalsBodyCodes}
        />
        <span className="settings-hint">{TOOLS.loginSignalsBodyCodesHint}</span>
      </label>
      <label className="settings-field">
        <span className="settings-label">{TOOLS.loginSignalsMessageContains}</span>
        <textarea
          className="settings-input"
          rows={3}
          value={draft.messageContainsText}
          onChange={(e) => onDraftChange({ messageContainsText: e.target.value })}
          placeholder={TOOLS.phLoginSignalsMessages}
        />
        <span className="settings-hint">{TOOLS.loginSignalsMessageContainsHint}</span>
      </label>
    </div>
  )
}
