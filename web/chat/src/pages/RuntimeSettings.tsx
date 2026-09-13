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
  type ToastApi,
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

function CredentialsSection({ push }: { push: ToastApi['push'] }) {
  const { role } = useGate()
  const [view, setView] = useState<CredentialsView | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
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
    try {
      setView(await getCredentials())
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setLoading(false)
    }
  }, [role, push])

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
      push({ tone: 'error', title: RUNTIME.rotateNeedOne })
      return
    }
    setBusy(true)
    try {
      const v = await patchCredentials({
        operator_token: operatorToken || undefined,
        admin_token: adminToken || undefined,
      })
      apply(v)
      push({ tone: 'success', title: RUNTIME.toastRotated })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const addOperator = async (e: FormEvent) => {
    e.preventDefault()
    const id = newOpId.trim()
    if (!id || !newOpToken) {
      push({ tone: 'error', title: RUNTIME.addNeedBoth })
      return
    }
    setBusy(true)
    try {
      apply(await patchCredentials({ add_operators: [{ id, token: newOpToken }] }))
      push({ tone: 'success', title: RUNTIME.toastAdded, detail: id })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const removeOperator = async (id: string) => {
    setBusy(true)
    try {
      apply(await patchCredentials({ remove_operators: [id] }))
      push({ tone: 'success', title: RUNTIME.toastRemoved, detail: id })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const resetAll = async () => {
    setBusy(true)
    setResetError(null)
    try {
      apply(await patchCredentials({ reset: true }))
      push({ tone: 'success', title: RUNTIME.toastReset })
      setPendingReset(false)
    } catch (err) {
      const f = friendlyError(err)
      const msg = f.detail ?? f.title
      setResetError(msg)
      push({ tone: 'error', title: f.title, detail: f.detail })
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
      {view && (
        <>
          <p className="settings-muted">
            {view.source === 'override' ? RUNTIME.credsSourceOverride : RUNTIME.credsSourceConfig}
            {' · '}
            {view.operator_set ? RUNTIME.credsOperatorSet : RUNTIME.credsOperatorUnset}
            {' · '}
            {view.admin_set ? RUNTIME.credsAdminSet : RUNTIME.credsAdminUnset}
          </p>

          <h3 className="settings-subheading">{RUNTIME.rotateTitle}</h3>
          <form onSubmit={(e) => void rotateTokens(e)}>
            <Field label={RUNTIME.fieldOperatorToken} hint={RUNTIME.hintOperatorToken}>
              <Input
                type="password"
                autoComplete="off"
                value={operatorToken}
                onChange={(e) => setOperatorToken(e.target.value)}
                disabled={busy}
                placeholder="••••••"
              />
            </Field>
            <Field label={RUNTIME.fieldAdminToken} hint={RUNTIME.hintAdminToken}>
              <Input
                type="password"
                autoComplete="off"
                value={adminToken}
                onChange={(e) => setAdminToken(e.target.value)}
                disabled={busy}
                placeholder="••••••"
              />
            </Field>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? RUNTIME.saving : RUNTIME.rotateSubmit}
            </Button>
          </form>

          <h3 className="settings-subheading" style={{ marginTop: '1.25rem' }}>
            {RUNTIME.namedOpsTitle}
          </h3>
          {view.operators.length === 0 ? (
            <p className="settings-muted">{RUNTIME.namedOpsEmpty}</p>
          ) : (
            <ul className="cred-operator-list">
              {view.operators.map((op) => (
                <li key={op.id} className="cred-operator-item">
                  <span className="cred-operator-id">{op.id}</span>
                  <Badge>
                    {op.source === 'runtime' ? RUNTIME.badgeRuntime : RUNTIME.badgeConfig}
                  </Badge>
                  {op.source === 'runtime' && (
                    <Button
                      type="button"
                      variant="ghost"
                      disabled={busy}
                      onClick={() => void removeOperator(op.id)}
                    >
                      {RUNTIME.removeOperator}
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
          <form onSubmit={(e) => void addOperator(e)}>
            <Field label={RUNTIME.fieldNewOpId}>
              <Input
                value={newOpId}
                onChange={(e) => setNewOpId(e.target.value)}
                disabled={busy}
                placeholder="bob"
              />
            </Field>
            <Field label={RUNTIME.fieldNewOpToken}>
              <Input
                type="password"
                autoComplete="off"
                value={newOpToken}
                onChange={(e) => setNewOpToken(e.target.value)}
                disabled={busy}
                placeholder="••••••"
              />
            </Field>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? RUNTIME.saving : RUNTIME.addOperator}
            </Button>
          </form>

          <div style={{ marginTop: '1.25rem' }}>
            <Button
              type="button"
              variant="danger"
              disabled={busy}
              onClick={() => {
                setResetError(null)
                setPendingReset(true)
              }}
            >
              {RUNTIME.resetButton}
            </Button>
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

      <CredentialsSection push={push} />
    </div>
  )
}
