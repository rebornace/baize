import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  getCredentials,
  getRuntimeSettings,
  listModelProfiles,
  patchCredentials,
  patchRuntimeSettings,
  type CredentialsView,
  type ModelProfile,
  type RuntimeKnobsView,
} from '../api'
import {
  Badge,
  Button,
  ConfirmDialog,
  Field,
  Input,
  PageHeader,
  Select,
  ToastRegion,
  useToast,
  type ToastApi,
} from '../components/ui'
import { useLocale } from '../locale/LocaleContext'
import { useGate } from '../gateContext'
import { RUNTIME, friendlyError } from '../strings'
import {
  mainKnobFields,
  compactAdvFields,
  allKnobFieldSpecs,
  decideToolFields,
  decideMiscFields,
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
  const { locale, setLocale, strings } = useLocale()
  const readOnly = role !== 'admin'
  const { toasts, push, dismiss } = useToast()
  const [knobView, setKnobView] = useState<RuntimeKnobsView | null>(null)
  const [form, setForm] = useState<KnobsForm | null>(null)
  const [profiles, setProfiles] = useState<ModelProfile[]>([])
  const [publicBase, setPublicBase] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const L = strings.LOCALE

  useEffect(() => {
    let alive = true
    void listModelProfiles()
      .then((ps) => {
        if (alive) setProfiles(ps)
      })
      .catch(() => {
        // 模型下拉加载失败时退化为自由输入，不阻塞页面。
      })
    return () => {
      alive = false
    }
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const v = await getRuntimeSettings()
      setKnobView(v)
      setForm(knobsToForm(v.effective))
      setPublicBase(v.public_base_url ?? '')
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

  const onSubmitPublicBase = async (e: FormEvent) => {
    e.preventDefault()
    if (!knobView) return
    const trimmed = publicBase.trim()
    if (trimmed && !/^https?:\/\//i.test(trimmed)) {
      push({ tone: 'error', title: RUNTIME.errPublicBaseInvalid })
      return
    }
    if (trimmed === (knobView.public_base_url ?? '')) {
      push({ tone: 'info', title: RUNTIME.toastNoChange })
      return
    }
    setBusy(true)
    try {
      const v = await patchRuntimeSettings({ public_base_url: trimmed })
      setKnobView(v)
      setPublicBase(v.public_base_url ?? '')
      push({ tone: 'success', title: RUNTIME.toastPublicBaseSaved })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
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
      setPublicBase(v.public_base_url ?? '')
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

      <section className="settings-form" aria-labelledby="locale-section-title">
        <h2 id="locale-section-title" className="settings-subheading">
          {L.sectionTitle}
        </h2>
        <p className="settings-muted">{L.sectionHint}</p>
        <div className="locale-choice" role="group" aria-label={L.sectionTitle} data-testid="locale-choice">
          <button
            type="button"
            className="locale-choice-option"
            aria-pressed={locale === 'zh-CN'}
            data-testid="locale-zh"
            onClick={() => setLocale('zh-CN')}
          >
            {L.optionZh}
          </button>
          <button
            type="button"
            className="locale-choice-option"
            aria-pressed={locale === 'en'}
            data-testid="locale-en"
            onClick={() => setLocale('en')}
          >
            {L.optionEn}
          </button>
        </div>
      </section>

      {loading && <p className="settings-muted">加载中…</p>}

      {!loading && knobView && (
        <form className="settings-form" onSubmit={(e) => void onSubmitPublicBase(e)}>
          <h2 className="settings-subheading">{RUNTIME.sectionPublicBase}</h2>
          {!knobView.public_base_url && (
            <p className="settings-muted">{RUNTIME.publicBaseRequiredHint}</p>
          )}
          <Field
            label={knobFieldLabel(RUNTIME.fieldPublicBase, knobView.public_base_url_overridden)}
            hint={RUNTIME.hintPublicBase}
          >
            <Input
              type="url"
              placeholder="http://127.0.0.1:8080"
              value={publicBase}
              onChange={(e) => setPublicBase(e.target.value)}
              disabled={busy || readOnly}
            />
          </Field>
          {!readOnly && (
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? RUNTIME.saving : RUNTIME.savePublicBase}
            </Button>
          )}
        </form>
      )}

      {!loading && knobView && form && (
        <form className="settings-form" onSubmit={(e) => void onSubmitKnobs(e)}>
          <h2 className="settings-subheading">{RUNTIME.sectionBehavior}</h2>
          {mainKnobFields().map((spec) => (
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
            {compactAdvFields().map((spec) => (
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

          <h2 className="settings-subheading">{RUNTIME.sectionMemory}</h2>
          <p className="settings-muted">{RUNTIME.memoryEnabledHint}</p>
          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={form.memory_enabled}
              onChange={(e) => setField('memory_enabled', e.target.checked)}
              disabled={busy || readOnly}
            />
            {RUNTIME.memoryEnabled}
            {knobView.overridden.memory_enabled && (
              <Badge>{RUNTIME.badgeOverridden}</Badge>
            )}
          </label>
          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={form.memory_auto_extract}
              onChange={(e) => setField('memory_auto_extract', e.target.checked)}
              disabled={busy || readOnly || !form.memory_enabled}
            />
            {RUNTIME.memoryAutoExtract}
            {knobView.overridden.memory_auto_extract && (
              <Badge>{RUNTIME.badgeOverridden}</Badge>
            )}
          </label>
          <p className="settings-muted">{RUNTIME.memoryAutoExtractHint}</p>

          <h2 className="settings-subheading">{RUNTIME.sectionDecide}</h2>
          <p className="settings-muted">{RUNTIME.decideHint}</p>
          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={form.decide_enabled}
              onChange={(e) => setField('decide_enabled', e.target.checked)}
              disabled={busy || readOnly}
            />
            {RUNTIME.decideEnabled}
            {knobView.overridden.decide_enabled && <Badge>{RUNTIME.badgeOverridden}</Badge>}
          </label>

          <Field
            label={knobFieldLabel(RUNTIME.fieldDecideProfile, knobView.overridden.decide_profile_id)}
            hint={RUNTIME.hintDecideProfile}
          >
            {profiles.length > 0 ? (
              <Select
                value={form.decide_profile_id}
                onChange={(e) => setField('decide_profile_id', e.target.value)}
                disabled={busy || readOnly || !form.decide_enabled}
              >
                <option value="">{RUNTIME.decideProfileNone}</option>
                {profiles.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}（{p.model}）
                  </option>
                ))}
              </Select>
            ) : (
              <Input
                value={form.decide_profile_id}
                onChange={(e) => setField('decide_profile_id', e.target.value)}
                disabled={busy || readOnly || !form.decide_enabled}
                placeholder={RUNTIME.decideProfileNone}
              />
            )}
          </Field>

          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={form.decide_memory_enabled}
              onChange={(e) => setField('decide_memory_enabled', e.target.checked)}
              disabled={busy || readOnly || !form.decide_enabled}
            />
            {RUNTIME.decideMemoryEnabled}
            {knobView.overridden.decide_memory_enabled && (
              <Badge>{RUNTIME.badgeOverridden}</Badge>
            )}
          </label>
          <p className="settings-muted">{RUNTIME.decideMemoryEnabledHint}</p>

          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={form.decide_tool_routing_enabled}
              onChange={(e) => setField('decide_tool_routing_enabled', e.target.checked)}
              disabled={busy || readOnly || !form.decide_enabled}
            />
            {RUNTIME.decideToolRoutingEnabled}
            {knobView.overridden.decide_tool_routing_enabled && (
              <Badge>{RUNTIME.badgeOverridden}</Badge>
            )}
          </label>
          <p className="settings-muted">{RUNTIME.decideToolRoutingHint}</p>
          <label className="settings-checkbox" style={{ marginLeft: '1.25rem' }}>
            <input
              type="checkbox"
              checked={form.decide_tool_shadow}
              onChange={(e) => setField('decide_tool_shadow', e.target.checked)}
              disabled={busy || readOnly || !form.decide_enabled || !form.decide_tool_routing_enabled}
            />
            {RUNTIME.decideToolShadow}
            {knobView.overridden.decide_tool_shadow && <Badge>{RUNTIME.badgeOverridden}</Badge>}
          </label>
          <p className="settings-muted" style={{ marginLeft: '1.25rem' }}>
            {RUNTIME.decideToolShadowHint}
          </p>
          <label className="settings-checkbox" style={{ marginLeft: '1.25rem' }}>
            <input
              type="checkbox"
              checked={form.decide_tool_choice_enabled}
              onChange={(e) => setField('decide_tool_choice_enabled', e.target.checked)}
              disabled={
                busy ||
                readOnly ||
                !form.decide_enabled ||
                !form.decide_tool_routing_enabled ||
                form.decide_tool_shadow
              }
            />
            {RUNTIME.decideToolChoice}
            {knobView.overridden.decide_tool_choice_enabled && (
              <Badge>{RUNTIME.badgeOverridden}</Badge>
            )}
          </label>
          <p className="settings-muted" style={{ marginLeft: '1.25rem' }}>
            {RUNTIME.decideToolChoiceHint}
          </p>
          <details>
            <summary>{RUNTIME.decideToolRoutingEnabled}</summary>
            {decideToolFields().map((spec) => (
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
                  disabled={busy || readOnly || !form.decide_enabled}
                />
              </Field>
            ))}
          </details>

          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={form.decide_tool_prune_enabled}
              onChange={(e) => setField('decide_tool_prune_enabled', e.target.checked)}
              disabled={busy || readOnly || !form.decide_enabled}
            />
            {RUNTIME.decideToolPruneEnabled}
            {knobView.overridden.decide_tool_prune_enabled && (
              <Badge>{RUNTIME.badgeOverridden}</Badge>
            )}
          </label>
          <p className="settings-muted">{RUNTIME.decideToolPruneHint}</p>
          <details>
            <summary>{RUNTIME.decideToolPruneEnabled}</summary>
            {decideMiscFields()
              .filter((s) => s.key.startsWith('decide_tool_prune'))
              .map((spec) => (
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
                    disabled={busy || readOnly || !form.decide_enabled}
                  />
                </Field>
              ))}
          </details>

          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={form.decide_route_enabled}
              onChange={(e) => setField('decide_route_enabled', e.target.checked)}
              disabled={busy || readOnly || !form.decide_enabled}
            />
            {RUNTIME.decideRouteEnabled}
            {knobView.overridden.decide_route_enabled && (
              <Badge>{RUNTIME.badgeOverridden}</Badge>
            )}
          </label>
          <p className="settings-muted">{RUNTIME.decideRouteHint}</p>
          <details>
            <summary>{RUNTIME.decideRouteEnabled}</summary>
            {decideMiscFields()
              .filter((s) => s.key === 'decide_route_min_runes')
              .map((spec) => (
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
                    disabled={busy || readOnly || !form.decide_enabled}
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
