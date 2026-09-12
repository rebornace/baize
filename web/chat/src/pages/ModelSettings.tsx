import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  createModelProfile,
  deleteModelProfile,
  listModelProfiles,
  updateModelProfile,
  type ModelProfile,
  type ModelTier,
} from '../api'
import { useGate } from '../gateContext'
import { tierLabel } from '../modelSelect'
import { VISION_LABEL } from '../strings'

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
  if (!name) return { ok: false, message: '名称不能为空' }
  const baseUrl = form.baseUrl.trim()
  if (!baseUrl) return { ok: false, message: 'Base URL 不能为空' }
  const model = form.model.trim()
  if (!model) return { ok: false, message: '模型不能为空' }
  const apiKey = form.apiKey.trim()
  const apiKeyEnv = form.apiKeyEnv.trim()
  if (!apiKey && !apiKeyEnv) {
    return { ok: false, message: 'API Key 与环境变量名至少填写一项' }
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

function apiErrorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
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
        <span className="settings-field-label">名称</span>
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
        <span className="settings-field-label">Base URL</span>
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
        <span className="settings-field-label">模型</span>
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
        <span className="settings-field-label">API Key</span>
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
        <span className="settings-field-label">API Key 环境变量名</span>
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
        视觉（支持图片附件）
      </label>
      <label className="settings-checkbox">
        <input
          type="checkbox"
          checked={form.disableThinking}
          onChange={(e) => setForm((f) => ({ ...f, disableThinking: e.target.checked }))}
          disabled={busy}
        />
        禁用思考（disable_thinking）
      </label>
      <label className="settings-field">
        <span className="settings-field-label">上下文长度（tokens，留空/0 用默认 128000）</span>
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
        <span className="settings-field-label">Auto 路由档位</span>
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
  if (profiles.length === 0) {
    return (
      <p className="settings-empty">
        {readOnly
          ? '尚未配置任何模型，请联系管理员配置，否则无法发起对话。'
          : '尚未配置任何模型。请先在下方「新建模型」添加至少一个模型，否则无法发起对话。'}
      </p>
    )
  }
  return (
    <ul className="settings-list">
      {profiles.map((p) => (
        <li key={p.id} className="settings-list-item">
          <span className="settings-tool-line">
            <span className="settings-tool-title">{p.name}</span>
            <span className="settings-badge">{tierLabel(p.auto_tier)}</span>
            {p.supports_vision && <span className="settings-badge">{VISION_LABEL}</span>}
            <span className="settings-muted"> · {p.model}</span>
            <span className="settings-muted"> · {p.base_url}</span>
          </span>
          <p className="settings-muted">
            {credentialHint(p)}
            {p.disable_thinking ? ' · 禁用思考' : ''}
            {p.context_tokens > 0 ? ` · ${p.context_tokens} ctx` : ''}
          </p>
          {!readOnly && (
            <div className="settings-toolbar">
              <button
                type="button"
                className="btn ghost sm"
                disabled={busy}
                onClick={() => onEdit(p)}
              >
                编辑
              </button>
              <button
                type="button"
                className="btn danger sm"
                disabled={busy}
                onClick={() => onDelete(p)}
              >
                删除
              </button>
            </div>
          )}
        </li>
      ))}
    </ul>
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
            取消
          </button>
        )}
      </div>
    </form>
  )
}

export function ModelSettings() {
  const { role } = useGate()
  const readOnly = role !== 'admin'
  const [profiles, setProfiles] = useState<ModelProfile[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [createForm, setCreateForm] = useState<ProfileFormState>(EMPTY_PROFILE_FORM)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editForm, setEditForm] = useState<ProfileFormState>(EMPTY_PROFILE_FORM)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const list = await listModelProfiles()
      setProfiles(list)
      setError(null)
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    const built = buildCreatePayload(createForm)
    if (!built.ok) {
      setError(built.message)
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      await createModelProfile(built.payload)
      setCreateForm(EMPTY_PROFILE_FORM)
      setStatus('已创建模型 profile')
      await load()
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const startEdit = (p: ModelProfile) => {
    setEditingId(p.id)
    setEditForm(profileToForm(p))
    setError(null)
    setStatus(null)
  }

  const onSaveEdit = async (e: FormEvent) => {
    e.preventDefault()
    if (!editingId) return
    const original = profiles.find((p) => p.id === editingId)
    if (!original) return
    const payload = buildPatchPayload(editForm, original)
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      await updateModelProfile(editingId, payload)
      setEditingId(null)
      setEditForm(EMPTY_PROFILE_FORM)
      setStatus('已更新模型 profile')
      await load()
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const onDelete = async (p: ModelProfile) => {
    const suffix = profiles.length === 1 ? '\n\n这是最后一个模型，删除后将无法发起对话，请尽快添加新模型。' : ''
    if (!window.confirm(`删除模型「${p.name}」？此操作不可恢复。${suffix}`)) return
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      await deleteModelProfile(p.id)
      if (editingId === p.id) {
        setEditingId(null)
        setEditForm(EMPTY_PROFILE_FORM)
      }
      setStatus(`已删除模型 ${p.name}`)
      await load()
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-section settings-models">
      <h1 className="settings-heading">模型</h1>
      <div className="settings-meta">
        <p>
          配置多个 OpenAI 兼容模型，并为每个模型选择「Auto 路由档位」（快速 / 标准 / 深度思考）。聊天默认走
          「智能选择」：根据每轮对话的实际难度、长度、代码与附件情况，以及是否含图片，自动挑选最合适档位的模型——
          图片消息只会使用勾选了「视觉」的模型。也可在输入框手动指定某条消息固定使用某个模型（此时不自动路由）。
          API Key 保存在本地库中，界面仅显示脱敏值；也可只填环境变量名，由进程环境提供密钥。
        </p>
      </div>

      {loading && <p className="settings-muted">加载中…</p>}
      {!loading && error && <p className="settings-error">{error}</p>}
      {!loading && status && <p className="settings-muted">{status}</p>}
      {!loading && profiles.length === 0 && !error && (
        <p className="settings-error">
          {readOnly ? '当前没有任何可用模型，请联系管理员配置。' : '当前没有任何可用模型，请先在下方添加一个模型再发起对话。'}
        </p>
      )}

      {!loading && !readOnly && editingId && (
        <section className="settings-form">
          <ModelProfileForm
            form={editForm}
            setForm={setEditForm}
            busy={busy}
            isEdit
            title={`编辑 ${profiles.find((p) => p.id === editingId)?.name ?? ''}`}
            submitLabel="保存"
            onSubmit={(e) => void onSaveEdit(e)}
            onCancel={() => {
              setEditingId(null)
              setEditForm(EMPTY_PROFILE_FORM)
            }}
          />
        </section>
      )}

      {!loading && readOnly && (
        <section>
          <h2 className="settings-subheading">模型列表</h2>
          <ModelProfileList
            profiles={profiles}
            busy={false}
            readOnly
            onEdit={() => undefined}
            onDelete={() => undefined}
          />
        </section>
      )}
      {!loading && !readOnly && (
        <section className="settings-form">
          <h2 className="settings-subheading">模型列表</h2>
          <ModelProfileList
            profiles={profiles}
            busy={busy || editingId != null}
            onEdit={startEdit}
            onDelete={(target) => void onDelete(target)}
          />

          <ModelProfileForm
            form={createForm}
            setForm={setCreateForm}
            busy={busy}
            isEdit={false}
            title="新建模型"
            submitLabel="创建模型"
            onSubmit={(e) => void onCreate(e)}
          />
        </section>
      )}
    </div>
  )
}
