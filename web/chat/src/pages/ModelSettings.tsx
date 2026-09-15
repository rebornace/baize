import { useCallback, useEffect, useState } from 'react'
import { Cpu } from 'lucide-react'
import {
  createModelProfile,
  deleteModelProfile,
  listModelProfiles,
  updateModelProfile,
  type ModelProfile,
  type ModelTier,
  type ThinkingDialect,
  type ThinkingLevel,
} from '../api'
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  Modal,
  PageHeader,
  Select,
  ToastRegion,
  useToast,
} from '../components/ui'
import { useGate } from '../gateContext'
import { tierLabel } from '../modelSelect'
import { MODELS, VISION_LABEL, modelErrorText } from '../strings'

const CREATE_ID = '__new__'

function tierOptions(): { value: ModelTier | 'auto'; label: string }[] {
  return [
    { value: 'auto', label: MODELS.tierOptionAuto },
    { value: 'light', label: MODELS.tierOptionLight },
    { value: 'standard', label: MODELS.tierOptionStandard },
    { value: 'power', label: MODELS.tierOptionPower },
  ]
}

function thinkingLevelOptions(): { value: ThinkingLevel; label: string }[] {
  return [
    { value: 'off', label: MODELS.thinkingLevelOff },
    { value: 'low', label: MODELS.thinkingLevelLow },
    { value: 'medium', label: MODELS.thinkingLevelMedium },
    { value: 'high', label: MODELS.thinkingLevelHigh },
  ]
}

function thinkingDialectOptions(): { value: ThinkingDialect; label: string }[] {
  return [
    { value: 'auto', label: MODELS.thinkingDialectAuto },
    { value: 'openai', label: MODELS.thinkingDialectOpenai },
    { value: 'deepseek', label: MODELS.thinkingDialectDeepseek },
    { value: 'qwen', label: MODELS.thinkingDialectQwen },
    { value: 'omit', label: MODELS.thinkingDialectOmit },
  ]
}

export interface ProfileFormState {
  name: string
  baseUrl: string
  model: string
  apiKey: string
  apiKeyEnv: string
  supportsVision: boolean
  thinkingLevel: ThinkingLevel
  thinkingDialect: ThinkingDialect
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
  thinkingLevel: 'medium',
  thinkingDialect: 'auto',
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
  thinking_level?: ThinkingLevel
  thinking_dialect?: ThinkingDialect
  context_tokens?: number
  auto_tier?: ModelTier | 'auto'
}

function resolveThinkingLevel(p: ModelProfile): ThinkingLevel {
  if (p.thinking_level) return p.thinking_level
  return p.disable_thinking ? 'off' : 'medium'
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
    thinkingLevel: resolveThinkingLevel(p),
    thinkingDialect: p.thinking_dialect ?? 'auto',
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
      thinking_level: form.thinkingLevel,
      thinking_dialect: form.thinkingDialect,
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
  if (form.thinkingLevel !== resolveThinkingLevel(original)) {
    payload.thinking_level = form.thinkingLevel
  }
  if (form.thinkingDialect !== (original.thinking_dialect ?? 'auto')) {
    payload.thinking_dialect = form.thinkingDialect
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
            {resolveThinkingLevel(p) === 'off' ? ` · ${MODELS.listThinkingOff}` : ''}
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

export function ModelSettings() {
  const { role } = useGate()
  const readOnly = role !== 'admin'
  const { toasts, push, dismiss } = useToast()
  const [profiles, setProfiles] = useState<ModelProfile[]>([])
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<ProfileFormState>(EMPTY_PROFILE_FORM)
  const [formError, setFormError] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] = useState<ModelProfile | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const modalOpen = editingId != null
  const isCreate = editingId === CREATE_ID
  const isLastModel = profiles.length === 1

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

  const closeEditor = () => {
    if (busy) return
    setEditingId(null)
    setForm(EMPTY_PROFILE_FORM)
    setFormError(null)
  }

  const openCreate = () => {
    setEditingId(CREATE_ID)
    setForm(EMPTY_PROFILE_FORM)
    setFormError(null)
  }

  const startEdit = (p: ModelProfile) => {
    setEditingId(p.id)
    setForm(profileToForm(p))
    setFormError(null)
  }

  const save = async () => {
    setFormError(null)
    if (isCreate) {
      const built = buildCreatePayload(form)
      if (!built.ok) {
        setFormError(built.message)
        return
      }
      setBusy(true)
      try {
        await createModelProfile(built.payload)
        setEditingId(null)
        setForm(EMPTY_PROFILE_FORM)
        setFormError(null)
        push({ tone: 'success', title: MODELS.toastSaved })
        await load()
      } catch (err) {
        const f = modelErrorText(err)
        setFormError(f.detail ? `${f.title} ${f.detail}` : f.title)
        push({ tone: 'error', title: f.title, detail: f.detail })
      } finally {
        setBusy(false)
      }
      return
    }
    if (!editingId) return
    const original = profiles.find((p) => p.id === editingId)
    if (!original) return
    const name = form.name.trim()
    if (!name) {
      setFormError(MODELS.errNameRequired)
      return
    }
    const payload = buildPatchPayload(form, original)
    setBusy(true)
    try {
      await updateModelProfile(editingId, payload)
      setEditingId(null)
      setForm(EMPTY_PROFILE_FORM)
      setFormError(null)
      push({ tone: 'success', title: MODELS.toastSaved })
      await load()
    } catch (err) {
      const f = modelErrorText(err)
      setFormError(f.detail ? `${f.title} ${f.detail}` : f.title)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const beginDelete = (p: ModelProfile) => {
    setDeleteError(null)
    setPendingDelete(p)
  }

  const cancelDelete = () => {
    if (busy) return
    setPendingDelete(null)
    setDeleteError(null)
  }

  const confirmDelete = async () => {
    if (!pendingDelete) return
    setBusy(true)
    setDeleteError(null)
    try {
      await deleteModelProfile(pendingDelete.id)
      if (editingId === pendingDelete.id) {
        setEditingId(null)
        setForm(EMPTY_PROFILE_FORM)
        setFormError(null)
      }
      push({ tone: 'success', title: MODELS.toastDeleted, detail: pendingDelete.name })
      setPendingDelete(null)
      await load()
    } catch (err) {
      const f = modelErrorText(err)
      setDeleteError(f.detail ? `${f.title} ${f.detail}` : f.title)
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

  const modalTitle = isCreate
    ? MODELS.add
    : `${MODELS.edit} ${profiles.find((p) => p.id === editingId)?.name ?? ''}`.trim()

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
          busy={busy || modalOpen}
          readOnly={readOnly}
          onEdit={startEdit}
          onDelete={beginDelete}
        />
      )}

      {!readOnly && (
        <Modal
          open={modalOpen}
          title={modalTitle}
          onClose={busy ? undefined : closeEditor}
          footer={
            <>
              <Button variant="ghost" disabled={busy} onClick={closeEditor}>
                {MODELS.cancel}
              </Button>
              <Button disabled={busy} onClick={() => void save()}>
                {MODELS.save}
              </Button>
            </>
          }
        >
          <ProfileFields form={form} setForm={setForm} busy={busy} isEdit={!isCreate} />
          {formError && (
            <p className="ui-inline-error" role="alert">
              {formError}
            </p>
          )}
        </Modal>
      )}

      {!readOnly && (
        <ConfirmDialog
          open={!!pendingDelete}
          danger
          title={MODELS.confirmDeleteTitle}
          body={
            isLastModel
              ? `${MODELS.confirmDeleteBody}\n${MODELS.confirmDeleteLast}`
              : MODELS.confirmDeleteBody
          }
          confirmText={MODELS.confirmDeleteOk}
          busy={busy}
          error={deleteError}
          onCancel={cancelDelete}
          onConfirm={() => void confirmDelete()}
        />
      )}
    </div>
  )
}
