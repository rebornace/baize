import { Button, Field, Input, Select, Textarea } from '../components/ui'
import { CONNECTORS } from '../strings'
import type { HostedLoginDraft } from './hostedLoginForm'

type Props = {
  draft: HostedLoginDraft
  onDraftChange: (patch: Partial<HostedLoginDraft>) => void
  onInferDraft?: () => void
  inferBusy?: boolean
  disabled?: boolean
  canInfer?: boolean
}

export function HostedLoginSettingsFields({
  draft,
  onDraftChange,
  onInferDraft,
  inferBusy,
  disabled,
  canInfer,
}: Props) {
  const isOAuth = draft.mode === 'oauth'
  return (
    <div className="settings-form">
      <p className="settings-meta">{CONNECTORS.hostedLoginIntro}</p>
      <Field label={CONNECTORS.hostedLoginEnabled}>
        <label className="settings-check">
          <input
            type="checkbox"
            checked={draft.enabled}
            disabled={disabled}
            onChange={(e) => onDraftChange({ enabled: e.target.checked })}
          />{' '}
          {CONNECTORS.hostedLoginEnabledHint}
        </label>
      </Field>
      <Field label={CONNECTORS.hostedLoginMode} hint={CONNECTORS.hostedLoginModeHint}>
        <Select
          value={draft.mode}
          disabled={disabled}
          onChange={(e) => onDraftChange({ mode: e.target.value })}
        >
          <option value="password_form">password_form</option>
          <option value="token_paste">token_paste</option>
          <option value="basic">basic</option>
          <option value="header_pairs">header_pairs</option>
          <option value="oauth">oauth (enterprise IdP)</option>
        </Select>
      </Field>
      {!isOAuth && (
        <>
          <Field label={CONNECTORS.hostedLoginTool} hint={CONNECTORS.hostedLoginToolHint}>
            <Input
              value={draft.loginTool}
              disabled={disabled}
              onChange={(e) => onDraftChange({ loginTool: e.target.value })}
              placeholder={CONNECTORS.phHostedLoginTool}
            />
          </Field>
          <Field label={CONNECTORS.hostedLoginFields} hint={CONNECTORS.hostedLoginFieldsHint}>
            <Textarea
              rows={4}
              value={draft.fieldsText}
              disabled={disabled}
              onChange={(e) => onDraftChange({ fieldsText: e.target.value })}
              placeholder={CONNECTORS.phHostedLoginFields}
            />
          </Field>
          {onInferDraft && (
            <div className="settings-toolbar">
              <Button
                type="button"
                variant="secondary"
                size="sm"
                disabled={disabled || inferBusy || !canInfer}
                onClick={() => void onInferDraft()}
              >
                {inferBusy ? CONNECTORS.hostedLoginInferring : CONNECTORS.hostedLoginInfer}
              </Button>
            </div>
          )}
        </>
      )}
      {isOAuth && (
        <>
          <Field label={CONNECTORS.hostedLoginOAuthAuthorize} hint={CONNECTORS.hostedLoginOAuthAuthorizeHint}>
            <Input
              value={draft.oauthAuthorizeURL}
              disabled={disabled}
              onChange={(e) => onDraftChange({ oauthAuthorizeURL: e.target.value })}
              placeholder="https://idp.example.com/authorize"
            />
          </Field>
          <Field label={CONNECTORS.hostedLoginOAuthToken} hint={CONNECTORS.hostedLoginOAuthTokenHint}>
            <Input
              value={draft.oauthTokenURL}
              disabled={disabled}
              onChange={(e) => onDraftChange({ oauthTokenURL: e.target.value })}
              placeholder="https://idp.example.com/token"
            />
          </Field>
          <Field label={CONNECTORS.hostedLoginOAuthClientID}>
            <Input
              value={draft.oauthClientID}
              disabled={disabled}
              onChange={(e) => onDraftChange({ oauthClientID: e.target.value })}
            />
          </Field>
          <Field label={CONNECTORS.hostedLoginOAuthClientSecret} hint={CONNECTORS.hostedLoginOAuthClientSecretHint}>
            <Input
              type="password"
              value={draft.oauthClientSecret}
              disabled={disabled}
              onChange={(e) => onDraftChange({ oauthClientSecret: e.target.value })}
              autoComplete="off"
            />
          </Field>
          <Field label={CONNECTORS.hostedLoginOAuthScopes} hint={CONNECTORS.hostedLoginOAuthScopesHint}>
            <Input
              value={draft.oauthScopes}
              disabled={disabled}
              onChange={(e) => onDraftChange({ oauthScopes: e.target.value })}
              placeholder="openid profile"
            />
          </Field>
        </>
      )}
    </div>
  )
}
