import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { Cpu } from 'lucide-react'
import {
  createModelProfile,
  deleteModelProfile,
  listModelProfiles,
  updateModelProfile,
  type ModelProfile,
  type ModelTier,
} from '../api'
import {
  Badge,
  Button,
  Card,
  EmptyState,
  PageHeader,
  ToastRegion,
  useToast,
} from '../components/ui'
import { useGate } from '../gateContext'
import { tierLabel } from '../modelSelect'
import { MODELS, VISION_LABEL, modelErrorText } from '../strings'

const CREATE_ID = '__new__'

// Editable tier options plus "auto" (infer from the model name server-side).
const TIER_OPTIONS: { value: ModelTier | 'auto'; label: string }[] = [
  { value: 'auto', label: '自动识别（按模型名）' },
  { value: 'light', label: '快速（简单、省钱的日常任务）' },
  { value: 'standard', label: '标准（适合多数任务）' },
  { value: 'power', label: '深度思考（复杂推理、长任务）' },
]

export interface ProfileFormState {
  name: string
  baseUrl: string
  model: string
  apiKey: string
  apiKeyEnv: string
  supportsVision: boolean
  disableThinking: boolean
  contextTokens: number
  tier: ModelTier | 'auto'
}

export const EMPTY_PROFILE_FORM: ProfileFormState = {
  name: '',
  baseUrl: '',
  model: '',
  apiKey: '',
  apiKeyEnv: '',
  supportsVision: false,
  disableThinking: false,
  contextTokens: 128000,
  tier: 'auto',
}

export interface ModelProfilePayload {
  name?: string
  base_url?: string
  model?: string
  api_key?: string
  api_key_env?: string
  supports_vision?: boolean
  disable_thinking?: boolean
  context_tokens?: number
  auto_tier?: ModelTier | 'auto'
}

export function profileToForm(p: ModelProfile): ProfileFormState {
  return {
    name: p.name,
    baseUrl: p.base_url,
    model: p.model,
    // Never prefill: the list value is a redacted mask and must not be
    // echoed back; an empty field means "keep the stored key".
    apiKey: '',
    apiKeyEnv: p.api_key_env ?? '',
    supportsVision: p.supports_vision,
    disableThinking: p.disable_thinking,
    contextTokens: p.context_tokens > 0 ? p.context_tokens : 128000,
    tier: p.auto_tier ?? 'standard',
  }
}

export function buildCreatePayload(form: ProfileFormState):
  | { ok: true; payload: ModelProfilePayload }
  | { ok: false; message: string } {
  const name = form.name.trim()
  if (!name) return { ok: false, message: MODELS.errNameRequired }
  const baseUrl = form.baseUrl.trim()
  if (!baseUrl) return { ok: false, message: MODELS.errBaseUrlRequired }
  const model = form.model.trim()
  if (!model) return { ok: false, message: MODELS.errModelRequired }
  const apiKey = form.apiKey.trim()
  const apiKeyEnv = form.apiKeyEnv.trim()
  if (!apiKey && !apiKeyEnv) {
    return { ok: false, message: MODELS.errApiKeyRequired }
  }
  return {
    ok: true,
    payload: {
      name,
      base_url: baseUrl,
      model,
      ...(apiKey ? { api_key: apiKey } : {}),
      ...(apiKeyEnv ? { api_key_env: apiKeyEnv } : {}),
      supports_vision: form.supportsVision,
      disable_thinking: form.disableThinking,
      context_tokens: form.contextTokens > 0 ? form.contextTokens : 128000,
      auto_tier: form.tier,
    },
  }
}

// buildPatchPayload returns only the fields that differ from the stored
// profile. The backend merges field-level (pointer fields), so omitted
// fields — including an empty api_key — keep their stored value.
export function buildPatchPayload(
  form: ProfileFormState,
  original: ModelProfile,
): ModelProfilePayload {
  const payload: ModelProfilePayload = {}
  const name = form.name.trim()
  if (name !== original.name) payload.name = name
  const baseUrl = form.baseUrl.trim()
  if (baseUrl !== original.base_url) payload.base_url = baseUrl
  const model = form.model.trim()
  if (model !== original.model) payload.model = model
  const apiKey = form.apiKey.trim()
  if (apiKey) payload.api_key = apiKey
  const apiKeyEnv = form.apiKeyEnv.trim()
  if (apiKeyEnv !== (original.api_key_env ?? '')) payload.api_key_env = apiKeyEnv
  if (form.supportsVision !== original.supports_vision) {
    payload.supports_vision = form.supportsVision
  }
  if (form.disableThinking !== original.disable_thinking) {
    payload.disable_thinking = form.disableThinking
  }
  const contextTokens = Math.floor(Number(form.contextTokens))
  if (contextTokens > 0 && contextTokens !== original.context_tokens) {
    payload.context_tokens = contextTokens
  }
  if (form.tier !== (original.auto_tier ?? 'standard')) {
    payload.auto_tier = form.tier
  }
  return payload
}

function credentialHint(p: ModelProfile): string {
  if (p.api_key_env) return `env:${p.api_key_env}`
  if (p.api_key) return `key:${p.api_key}`
  return '无凭据'
}

interface ProfileFieldsProps {
  form: ProfileFormState
  setForm: (updater: (f: ProfileFormState) => ProfileFormState) => void
  busy: boolean
  isEdit: boolean
}

function ProfileFields({ form, setForm, busy, isEdit }: ProfileFieldsProps) {
  return (
    <>
      <label className="settings-field">
        <span className="settings-field-label">{MODELS.fieldName}</span>
        <input
          className="settings-input"
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          disabled={busy}
          placeholder="例如：工作模型 / 视觉模型"
          required
        />
      </label>
      <label className="settings-field">
        <span className="settings-field-label">{MODELS.fieldBaseUrl}</span>
        <input
          className="settings-input"
          value={form.baseUrl}
          onChange={(e) => setForm((f) => ({ ...f, baseUrl: e.target.value }))}
          disabled={busy}
          placeholder="https://api.openai.com/v1"
          required
        />
      </label>
      <label className="settings-field">
        <span className="settings-field-label">{MODELS.fieldModel}</span>
        <input
          className="settings-input"
          value={form.model}
          onChange={(e) => setForm((f) => ({ ...f, model: e.target.value }))}
          disabled={busy}
          placeholder="gpt-4o"
          required
        />
      </label>
      <label className="settings-field">
        <span className="settings-field-label">{MODELS.fieldApiKey}</span>
        <input
          className="settings-input"
          type="password"
          value={form.apiKey}
          onChange={(e) => setForm((f) => ({ ...f, apiKey: e.target.value }))}
          disabled={busy}
          placeholder={isEdit ? '留空则不修改' : '留空则使用环境变量'}
          autoComplete="off"
        />
      </label>
      <label className="settings-field">
        <span className="settings-field-label">{MODELS.fieldApiKeyEnv}</span>
        <input
          className="settings-input"
          value={form.apiKeyEnv}
          onChange={(e) => setForm((f) => ({ ...f, apiKeyEnv: e.target.value }))}
          disabled={busy}
          placeholder="OPENAI_API_KEY"
        />
      </label>
      <label className="settings-checkbox">
        <input
          type="checkbox"
          checked={form.supportsVision}
          onChange={(e) => setForm((f) => ({ ...f, supportsVision: e.target.checked }))}
          disabled={busy}
        />
        {MODELS.fieldVision}
      </label>
      <label className="settings-checkbox">
        <input
          type="checkbox"
          checked={form.disableThinking}
          onChange={(e) => setForm((f) => ({ ...f, disableThinking: e.target.checked }))}
          disabled={busy}
        />
        {MODELS.fieldDisableThinking}（disable_thinking）
      </label>
      <label className="settings-field">
        <span className="settings-field-label">{MODELS.fieldContextTokens}（tokens，留空/0 用默认 128000）</span>
        <input
          className="settings-input"
          type="number"
          min={1024}
          step={1000}
          value={form.contextTokens}
          onChange={(e) => setForm((f) => ({ ...f, contextTokens: Number(e.target.value) }))}
          disabled={busy}
        />
      </label>
      <label className="settings-field">
        <span className="settings-field-label">{MODELS.fieldTier}</span>
        <select
          className="settings-input"
          value={form.tier}
          onChange={(e) =>
            setForm((f) => ({ ...f, tier: e.target.value as ProfileFormState['tier'] }))
          }
          disabled={busy}
        >
          {TIER_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
        <small className="settings-muted">
          智能选择按对话难度在快速/标准/深度思考档位间选模型；图片消息只走勾选了「视觉」的模型。
        </small>
      </label>
    </>
  )
}

export interface ModelProfileListProps {
  profiles: ModelProfile[]
  busy: boolean
  readOnly?: boolean
  onEdit: (p: ModelProfile) => void
  onDelete: (p: ModelProfile) => void
}

export function ModelProfileList({
  profiles,
  busy,
  readOnly = false,
  onEdit,
  onDelete,
}: ModelProfileListProps) {
  if (profiles.length === 0) return null
  return (
    <div className="connector-list">
      {profiles.map((p) => (
        <Card
          key={p.id}
          title={p.name}
          description={`${p.model} · ${p.base_url}`}
          trailing={
            <div className="accounts-actions">
              <Badge tone="info">{tierLabel(p.auto_tier)}</Badge>
              {p.supports_vision && <Badge tone="info">{VISION_LABEL}</Badge>}
            </div>
          }
        >
          <p className="settings-muted">
            {credentialHint(p)}
            {p.disable_thinking ? ' · 禁用思考' : ''}
            {p.context_tokens > 0 ? ` · ${p.context_tokens} ctx` : ''}
          </p>
          {!readOnly && (
            <div className="settings-toolbar">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={busy}
                onClick={() => onEdit(p)}
              >
                {MODELS.edit}
              </Button>
              <Button
                type="button"
                variant="danger"
                size="sm"
                disabled={busy}
                onClick={() => onDelete(p)}
              >
                {MODELS.delete}
              </Button>
            </div>
          )}
        </Card>
      ))}
    </div>
  )
}

export interface ModelProfileFormProps {
  form: ProfileFormState
  setForm: (updater: (f: ProfileFormState) => ProfileFormState) => void
  busy: boolean
  isEdit: boolean
  title: string
  submitLabel: string
  onSubmit: (e: FormEvent) => void
  onCancel?: () => void
}

export function ModelProfileForm({
  form,
  setForm,
  busy,
  isEdit,
  title,
  submitLabel,
  onSubmit,
  onCancel,
}: ModelProfileFormProps) {
  return (
    <form className="settings-form" onSubmit={onSubmit}>
      <h3 className="settings-subheading">{title}</h3>
      <ProfileFields form={form} setForm={setForm} busy={busy} isEdit={isEdit} />
      <div className="settings-toolbar">
        <button type="submit" className="btn primary sm" disabled={busy}>
          {submitLabel}
        </button>
        {onCancel && (
          <button type="button" className="btn ghost sm" disabled={busy} onClick={onCancel}>
            {MODELS.cancel}
          </button>
        )}
      </div>
    </form>
  )
}

export function ModelSettings() {
  const { role } = useGate()
  const readOnly = role !== 'admin'
  const { toasts, push, dismiss } = useToast()
  const [profiles, setProfiles] = useState<ModelProfile[]>([])
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [createForm, setCreateForm] = useState<ProfileFormState>(EMPTY_PROFILE_FORM)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editForm, setEditForm] = useState<ProfileFormState>(EMPTY_PROFILE_FORM)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const list = await listModelProfiles()
      setProfiles(list)
      setLoadFailed(false)
    } catch (err) {
      setLoadFailed(true)
      const f = modelErrorText(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setLoading(false)
    }
  }, [push])

  useEffect(() => {
    void load()
  }, [load])

  const openCreate = () => {
    setEditingId(CREATE_ID)
    setCreateForm(EMPTY_PROFILE_FORM)
  }

  const closeEditor = () => {
    setEditingId(null)
    setCreateForm(EMPTY_PROFILE_FORM)
    setEditForm(EMPTY_PROFILE_FORM)
  }

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    const built = buildCreatePayload(createForm)
    if (!built.ok) {
      push({ tone: 'error', title: built.message })
      return
    }
    setBusy(true)
    try {
      await createModelProfile(built.payload)
      closeEditor()
      push({ tone: 'success', title: MODELS.toastSaved })
      await load()
    } catch (err) {
      const f = modelErrorText(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const startEdit = (p: ModelProfile) => {
    setEditingId(p.id)
    setEditForm(profileToForm(p))
  }

  const onSaveEdit = async (e: FormEvent) => {
    e.preventDefault()
    if (!editingId || editingId === CREATE_ID) return
    const original = profiles.find((p) => p.id === editingId)
    if (!original) return
    const payload = buildPatchPayload(editForm, original)
    setBusy(true)
    try {
      await updateModelProfile(editingId, payload)
      closeEditor()
      push({ tone: 'success', title: MODELS.toastSaved })
      await load()
    } catch (err) {
      const f = modelErrorText(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const onDelete = async (p: ModelProfile) => {
    const suffix =
      profiles.length === 1
        ? `\n\n${MODELS.confirmDeleteLast}`
        : ''
    if (!window.confirm(`${MODELS.confirmDeleteTitle}\n${MODELS.confirmDeleteBody}${suffix}`)) {
      return
    }
    setBusy(true)
    try {
      await deleteModelProfile(p.id)
      if (editingId === p.id) closeEditor()
      push({ tone: 'success', title: MODELS.toastDeleted, detail: p.name })
      await load()
    } catch (err) {
      const f = modelErrorText(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const showEmpty = !loading && profiles.length === 0 && !loadFailed
  const headerAdd =
    showEmpty || readOnly ? undefined : (
      <Button variant="primary" size="sm" onClick={openCreate}>
        {MODELS.add}
      </Button>
    )

  return (
    <div className="settings-panel settings-models">
      <PageHeader title={MODELS.title} description={MODELS.description} actions={headerAdd} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {loading && <p className="settings-muted">加载中…</p>}

      {showEmpty && (
        <EmptyState
          icon={<Cpu size={28} aria-hidden="true" />}
          title={MODELS.emptyTitle}
          description={readOnly ? MODELS.emptyDescOperator : MODELS.emptyDescAdmin}
          action={
            readOnly ? undefined : (
              <Button variant="primary" onClick={openCreate}>
                {MODELS.add}
              </Button>
            )
          }
        />
      )}

      {!loading && profiles.length > 0 && (
        <ModelProfileList
          profiles={profiles}
          busy={busy || editingId != null}
          readOnly={readOnly}
          onEdit={startEdit}
          onDelete={(target) => void onDelete(target)}
        />
      )}

      {!readOnly && editingId === CREATE_ID && (
        <ModelProfileForm
          form={createForm}
          setForm={setCreateForm}
          busy={busy}
          isEdit={false}
          title="新建模型"
          submitLabel="创建模型"
          onSubmit={(e) => void onCreate(e)}
          onCancel={closeEditor}
        />
      )}

      {!readOnly && editingId != null && editingId !== CREATE_ID && (
        <ModelProfileForm
          form={editForm}
          setForm={setEditForm}
          busy={busy}
          isEdit
          title={`编辑 ${profiles.find((p) => p.id === editingId)?.name ?? ''}`}
          submitLabel={MODELS.save}
          onSubmit={(e) => void onSaveEdit(e)}
          onCancel={closeEditor}
        />
      )}
    </div>
  )
}
