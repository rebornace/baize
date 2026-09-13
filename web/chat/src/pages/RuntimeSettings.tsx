import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  getCredentials,
  getRuntimeSettings,
  patchCredentials,
  patchRuntimeSettings,
  type CredentialsView,
  type RuntimeKnobsView,
} from '../api'
import {
  Badge,
  Button,
  ConfirmDialog,
  Field,
  Input,
  PageHeader,
  ToastRegion,
  useToast,
} from '../components/ui'
import { useGate } from '../gateContext'
import { RUNTIME, friendlyError } from '../strings'
import {
  MAIN_KNOB_FIELDS,
  COMPACT_ADV_FIELDS,
  allKnobFieldSpecs,
  buildKnobsPatch,
  knobsToForm,
  validateKnobField,
  type KnobsForm,
} from './runtimeSettingsHelpers'

function CredentialsSection() {
  const { role } = useGate()
  const [view, setView] = useState<CredentialsView | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [operatorToken, setOperatorToken] = useState('')
  const [adminToken, setAdminToken] = useState('')
  const [newOpId, setNewOpId] = useState('')
  const [newOpToken, setNewOpToken] = useState('')
  const [pendingReset, setPendingReset] = useState(false)
  const [resetError, setResetError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (role !== 'admin') {
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      setView(await getCredentials())
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [role])

  useEffect(() => {
    void load()
  }, [load])

  const apply = useCallback((v: CredentialsView) => {
    setView(v)
    setOperatorToken('')
    setAdminToken('')
    setNewOpId('')
    setNewOpToken('')
  }, [])

  const rotateTokens = async (e: FormEvent) => {
    e.preventDefault()
    if (!operatorToken && !adminToken) {
      setError('请至少填写一个要轮换的口令')
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const v = await patchCredentials({
        operator_token: operatorToken || undefined,
        admin_token: adminToken || undefined,
      })
      apply(v)
      setStatus('口令已轮换；若改的是当前登录口令，请用新口令重新解锁。')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const addOperator = async (e: FormEvent) => {
    e.preventDefault()
    const id = newOpId.trim()
    if (!id || !newOpToken) {
      setError('新增 operator 需要 id 与 token')
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      apply(await patchCredentials({ add_operators: [{ id, token: newOpToken }] }))
      setStatus(`已新增 operator：${id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const removeOperator = async (id: string) => {
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      apply(await patchCredentials({ remove_operators: [id] }))
      setStatus(`已移除 operator：${id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const resetAll = async () => {
    setBusy(true)
    setError(null)
    setResetError(null)
    setStatus(null)
    try {
      apply(await patchCredentials({ reset: true }))
      setStatus('已重置：凭据回落至配置基线（引擎参数不受影响）。')
      setPendingReset(false)
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      setError(msg)
      setResetError(msg)
    } finally {
      setBusy(false)
    }
  }

  // 凭据（含 operator 列表）仅 admin 可见；hooks 必须保持无条件执行，故早退放在全部 hooks 之后。
  if (role !== 'admin') return null

  return (
    <section className="settings-form">
      <h2 className="settings-subheading">{RUNTIME.sectionCreds}</h2>
      {loading && <p className="settings-muted">加载中…</p>}
      {error && <p className="settings-error">{error}</p>}
      {status && <p className="settings-muted">{status}</p>}
      {view && (
        <>
          <p className="settings-muted">
            来源：{view.source === 'override' ? '热更新覆盖' : '配置基线（config）'} ·
            operator 口令 {view.operator_set ? '已设置' : '未设置'} ·
            admin 口令 {view.admin_set ? '已设置' : '未设置'}
          </p>

          <div className="settings-meta">
            <p>主口令轮换（留空表示不修改；明文经 HTTPS 提交，GET 永不返回口令）。</p>
          </div>
          <form onSubmit={(e) => void rotateTokens(e)}>
            <label className="settings-field">
              <span className="settings-field-label">operator token（留空不修改）</span>
              <input
                className="settings-input"
                type="password"
                autoComplete="off"
                value={operatorToken}
                onChange={(e) => setOperatorToken(e.target.value)}
                disabled={busy}
                placeholder="••••••"
              />
            </label>
            <label className="settings-field">
              <span className="settings-field-label">admin token（留空不修改）</span>
              <input
                className="settings-input"
                type="password"
                autoComplete="off"
                value={adminToken}
                onChange={(e) => setAdminToken(e.target.value)}
                disabled={busy}
                placeholder="••••••"
              />
            </label>
            <button type="submit" className="btn primary" disabled={busy}>
              {busy ? '提交中…' : '轮换主口令'}
            </button>
          </form>

          <h3 className="settings-subheading" style={{ marginTop: '1.25rem' }}>命名 operator</h3>
          {view.operators.length === 0 ? (
            <p className="settings-muted">暂无 operator。</p>
          ) : (
            <ul className="cred-operator-list">
              {view.operators.map((op) => (
                <li key={op.id} className="cred-operator-item">
                  <span className="cred-operator-id">{op.id}</span>
                  <span className="settings-badge">{op.source === 'runtime' ? 'runtime' : 'config'}</span>
                  {op.source === 'runtime' && (
                    <button
                      type="button"
                      className="btn ghost"
                      disabled={busy}
                      onClick={() => void removeOperator(op.id)}
                    >
                      移除
                    </button>
                  )}
                </li>
              ))}
            </ul>
          )}
          <form onSubmit={(e) => void addOperator(e)}>
            <label className="settings-field">
              <span className="settings-field-label">新增 operator id</span>
              <input
                className="settings-input"
                value={newOpId}
                onChange={(e) => setNewOpId(e.target.value)}
                disabled={busy}
                placeholder="bob"
              />
            </label>
            <label className="settings-field">
              <span className="settings-field-label">token</span>
              <input
                className="settings-input"
                type="password"
                autoComplete="off"
                value={newOpToken}
                onChange={(e) => setNewOpToken(e.target.value)}
                disabled={busy}
                placeholder="••••••"
              />
            </label>
            <button type="submit" className="btn primary" disabled={busy}>
              {busy ? '提交中…' : '新增 operator'}
            </button>
          </form>

          <div style={{ marginTop: '1.25rem' }}>
            <button
              type="button"
              className="btn danger"
              disabled={busy}
              onClick={() => {
                setResetError(null)
                setPendingReset(true)
              }}
            >
              {RUNTIME.resetButton}
            </button>
          </div>
        </>
      )}
      <ConfirmDialog
        open={pendingReset}
        danger
        title={RUNTIME.confirmResetTitle}
        body={RUNTIME.confirmResetBody}
        confirmText={RUNTIME.confirmResetOk}
        busy={busy}
        error={resetError}
        onCancel={() => {
          if (busy) return
          setPendingReset(false)
          setResetError(null)
        }}
        onConfirm={() => void resetAll()}
      />
    </section>
  )
}

function knobFieldLabel(label: string, overridden: boolean) {
  return (
    <>
      {label}
      {overridden && <Badge>{RUNTIME.badgeOverridden}</Badge>}
    </>
  )
}

export function RuntimeSettings() {
  const { role } = useGate()
  const readOnly = role !== 'admin'
  const { toasts, push, dismiss } = useToast()
  const [knobView, setKnobView] = useState<RuntimeKnobsView | null>(null)
  const [form, setForm] = useState<KnobsForm | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const v = await getRuntimeSettings()
      setKnobView(v)
      setForm(knobsToForm(v.effective))
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: RUNTIME.loadFailed, detail: f.detail ?? f.title })
    } finally {
      setLoading(false)
    }
  }, [push])

  useEffect(() => {
    void load()
  }, [load])

  const setField = (key: keyof KnobsForm, value: string | boolean) => {
    setForm((f) => (f ? { ...f, [key]: value } : f))
  }

  const onSubmitKnobs = async (e: FormEvent) => {
    e.preventDefault()
    if (!knobView || !form) return
    for (const spec of allKnobFieldSpecs()) {
      const msg = validateKnobField(spec, form[spec.key])
      if (msg) {
        push({ tone: 'error', title: msg })
        return
      }
    }
    const patch = buildKnobsPatch(form, knobView.effective)
    if (Object.keys(patch).length === 0) {
      push({ tone: 'info', title: RUNTIME.toastNoChange })
      return
    }
    setBusy(true)
    try {
      const v = await patchRuntimeSettings(patch)
      setKnobView(v)
      setForm(knobsToForm(v.effective))
      push({ tone: 'success', title: RUNTIME.toastKnobsSaved })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-section">
      <PageHeader
        title={RUNTIME.title}
        description={readOnly ? RUNTIME.descriptionOperator : RUNTIME.descriptionAdmin}
      />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {loading && <p className="settings-muted">加载中…</p>}

      {!loading && knobView && form && (
        <form className="settings-form" onSubmit={(e) => void onSubmitKnobs(e)}>
          <h2 className="settings-subheading">{RUNTIME.sectionBehavior}</h2>
          {MAIN_KNOB_FIELDS.map((spec) => (
            <Field
              key={spec.key}
              label={knobFieldLabel(spec.label, knobView.overridden[spec.key])}
              hint={spec.hint}
            >
              <Input
                type="number"
                step={spec.integer ? 1 : 'any'}
                min={spec.min}
                max={spec.max}
                value={form[spec.key]}
                onChange={(e) => setField(spec.key, e.target.value)}
                disabled={busy || readOnly}
              />
            </Field>
          ))}

          <h2 className="settings-subheading">{RUNTIME.sectionCompact}</h2>
          <p className="settings-muted">{RUNTIME.compactHint}</p>
          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={form.compaction_enabled}
              onChange={(e) => setField('compaction_enabled', e.target.checked)}
              disabled={busy || readOnly}
            />
            {RUNTIME.compactEnabled}
            {knobView.overridden.compaction_enabled && (
              <Badge>{RUNTIME.badgeOverridden}</Badge>
            )}
          </label>
          <details>
            <summary>{RUNTIME.compactAdvanced}</summary>
            {COMPACT_ADV_FIELDS.map((spec) => (
              <Field
                key={spec.key}
                label={knobFieldLabel(spec.label, knobView.overridden[spec.key])}
                hint={spec.hint}
              >
                <Input
                  type="number"
                  step={spec.integer ? 1 : 'any'}
                  min={spec.min}
                  max={spec.max}
                  value={form[spec.key]}
                  onChange={(e) => setField(spec.key, e.target.value)}
                  disabled={busy || readOnly}
                />
              </Field>
            ))}
          </details>

          {!readOnly && (
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? RUNTIME.saving : RUNTIME.saveKnobs}
            </Button>
          )}
        </form>
      )}

      <CredentialsSection />
    </div>
  )
}
