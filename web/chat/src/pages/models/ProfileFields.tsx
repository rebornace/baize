import type { ThinkingDialect, ThinkingLevel } from '../../api'
import { Field, Input, Select } from '../../components/ui'
import { MODELS } from '../../strings'
import {
  tierOptions,
  thinkingDialectOptions,
  thinkingLevelOptions,
  type ProfileFormState,
} from './modelSettingsHelpers'

interface ProfileFieldsProps {
  form: ProfileFormState
  setForm: (updater: (f: ProfileFormState) => ProfileFormState) => void
  busy: boolean
  isEdit: boolean
}

export function ProfileFields({ form, setForm, busy, isEdit }: ProfileFieldsProps) {
  return (
    <div className="connector-form">
      <Field label={MODELS.fieldName} required>
        <Input
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          disabled={busy}
          placeholder="例如：工作模型 / 视觉模型"
        />
      </Field>
      <Field label={MODELS.fieldBaseUrl} required>
        <Input
          value={form.baseUrl}
          onChange={(e) => setForm((f) => ({ ...f, baseUrl: e.target.value }))}
          disabled={busy}
          placeholder="https://api.openai.com/v1"
        />
      </Field>
      <Field label={MODELS.fieldModel} required>
        <Input
          value={form.model}
          onChange={(e) => setForm((f) => ({ ...f, model: e.target.value }))}
          disabled={busy}
          placeholder="gpt-4o"
        />
      </Field>
      <Field label={MODELS.fieldApiKey}>
        <Input
          type="password"
          value={form.apiKey}
          onChange={(e) => setForm((f) => ({ ...f, apiKey: e.target.value }))}
          disabled={busy}
          placeholder={isEdit ? '留空则不修改' : '留空则使用环境变量'}
          autoComplete="off"
        />
      </Field>
      <Field
        label={MODELS.fieldTier}
        hint="智能选择按对话难度在快速/标准/深度思考档位间选模型；图片消息只走勾选了「视觉」的模型。"
      >
        <Select
          value={form.tier}
          onChange={(e) =>
            setForm((f) => ({ ...f, tier: e.target.value as ProfileFormState['tier'] }))
          }
          disabled={busy}
        >
          {tierOptions().map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </Select>
      </Field>
      <Field label={MODELS.fieldThinkingLevel}>
        <Select
          value={form.thinkingLevel}
          onChange={(e) =>
            setForm((f) => ({
              ...f,
              thinkingLevel: e.target.value as ThinkingLevel,
            }))
          }
          disabled={busy}
        >
          {thinkingLevelOptions().map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </Select>
      </Field>
      <details className="settings-advanced">
        <summary>{MODELS.advanced}</summary>
        <label className="ui-checkbox-row">
          <input
            type="checkbox"
            checked={form.supportsVision}
            onChange={(e) => setForm((f) => ({ ...f, supportsVision: e.target.checked }))}
            disabled={busy}
          />
          <span>{MODELS.fieldVision}</span>
        </label>
        <Field label={MODELS.fieldThinkingDialect}>
          <Select
            value={form.thinkingDialect}
            onChange={(e) =>
              setForm((f) => ({
                ...f,
                thinkingDialect: e.target.value as ThinkingDialect,
              }))
            }
            disabled={busy}
          >
            {thinkingDialectOptions().map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={MODELS.fieldContextTokens} hint="tokens，留空/0 用默认 128000">
          <Input
            type="number"
            min={1024}
            step={1000}
            value={form.contextTokens}
            onChange={(e) => setForm((f) => ({ ...f, contextTokens: Number(e.target.value) }))}
            disabled={busy}
          />
        </Field>
        <Field label={MODELS.fieldApiKeyEnv}>
          <Input
            value={form.apiKeyEnv}
            onChange={(e) => setForm((f) => ({ ...f, apiKeyEnv: e.target.value }))}
            disabled={busy}
            placeholder="OPENAI_API_KEY"
          />
        </Field>
      </details>
    </div>
  )
}
