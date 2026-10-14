import { type FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import {
  createHostedLoginTestSession,
  fetchHostedLoginCandidates,
  getConnector,
  getRuntimeSettings,
  listConnectors,
  listMCPExportIdentities,
  listMCPExportKeys,
  patchMCPExportKey,
  patchRuntimeSettings,
  putConnector,
  suggestHostedLogin,
  type ConnectorInfo,
  type HostedLoginMethod,
  type MCPExportIdentity,
  type MCPExportKey,
} from '../api'
import {
  Button,
  Field,
  Input,
  PageHeader,
  Select,
  ToastRegion,
  useToast,
} from '../components/ui'
import { EXTERNAL_LOGIN, friendlyError } from '../strings'
import {
  buildHostedLoginFromDraft,
  hostedLoginToDraft,
  methodDraftFromCandidate,
  type HostedLoginMethodDraft,
} from './hostedLoginForm'

const WIZARD_STEPS = 4

function connectorEligible(c: ConnectorInfo): boolean {
  return c.type === 'openapi' || c.type === 'http'
}

function keyHasConnectorSlot(key: MCPExportKey, connectorId: string): boolean {
  return (key.identity_slots ?? []).some((s) => s.connector_id === connectorId && s.identity_id)
}

function findKeyForConnector(keys: MCPExportKey[], connectorId: string): MCPExportKey | undefined {
  return keys.find((k) => !k.revoked_at && keyHasConnectorSlot(k, connectorId))
}

export function ExternalLoginSettings() {
  const { toasts, push, dismiss } = useToast()
  const [searchParams] = useSearchParams()
  const preselectConnector = searchParams.get('connector')?.trim() ?? ''

  const [step, setStep] = useState(1)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [connectors, setConnectors] = useState<ConnectorInfo[]>([])
  const [connectorId, setConnectorId] = useState('')
  const [methodDrafts, setMethodDrafts] = useState<HostedLoginMethodDraft[]>([])
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set())
  const [defaultMethodId, setDefaultMethodId] = useState('')
  const [hostedEnabled, setHostedEnabled] = useState(true)
  const [publicBaseUrl, setPublicBaseUrl] = useState('')
  const [identities, setIdentities] = useState<MCPExportIdentity[]>([])
  const [keys, setKeys] = useState<MCPExportKey[]>([])
  const [exportKeyId, setExportKeyId] = useState('')
  const [identityId, setIdentityId] = useState('')
  const [testUrl, setTestUrl] = useState<string | null>(null)
  const [stepError, setStepError] = useState<string | null>(null)

  const loadCatalog = useCallback(async () => {
    setLoading(true)
    try {
      const [openapi, http, runtime, ids, keyList] = await Promise.all([
        listConnectors('openapi'),
        listConnectors('http'),
        getRuntimeSettings(),
        listMCPExportIdentities(),
        listMCPExportKeys(),
      ])
      const merged = [...openapi, ...http].filter(connectorEligible)
      setConnectors(merged)
      setPublicBaseUrl(runtime.public_base_url ?? '')
      setIdentities(ids)
      setKeys(keyList.filter((k) => !k.revoked_at))
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setLoading(false)
    }
  }, [push])

  useEffect(() => {
    void loadCatalog()
  }, [loadCatalog])

  useEffect(() => {
    if (!preselectConnector || loading || connectors.length === 0) return
    if (!connectors.some((c) => c.id === preselectConnector)) return
    setConnectorId(preselectConnector)
  }, [preselectConnector, loading, connectors])

  const resetMethodsFromConnector = useCallback(async (id: string) => {
    setBusy(true)
    setStepError(null)
    try {
      const conn = await getConnector(id)
      const existing = hostedLoginToDraft(conn.auth?.hosted_login)
      let drafts: HostedLoginMethodDraft[] = existing.methods
      if (drafts.length === 0) {
        const { candidates } = await fetchHostedLoginCandidates(id)
        drafts = candidates.map(methodDraftFromCandidate)
      }
      setMethodDrafts(drafts)
      const selected = new Set<string>()
      if (existing.enabled && existing.methods.length > 0) {
        for (const m of existing.methods) selected.add(m.id)
      } else if (drafts.length > 0) {
        for (const m of drafts) selected.add(m.id)
      }
      setSelectedIds(selected)
      const def =
        existing.defaultMethodId ||
        (selected.size > 0 ? [...selected][0] : drafts[0]?.id ?? '')
      setDefaultMethodId(def)
      setHostedEnabled(existing.enabled || selected.size > 0)

      const boundKey = findKeyForConnector(keys, id)
      if (boundKey) {
        setExportKeyId(boundKey.id)
        const slot = boundKey.identity_slots?.find((s) => s.connector_id === id)
        setIdentityId(slot?.identity_id ?? boundKey.identity_id)
      }
    } catch (err) {
      const f = friendlyError(err)
      setStepError(f.detail ?? f.title)
    } finally {
      setBusy(false)
    }
  }, [keys])

  useEffect(() => {
    if (!connectorId) return
    void resetMethodsFromConnector(connectorId)
  }, [connectorId, resetMethodsFromConnector])

  const selectedMethods = useMemo(
    () => methodDrafts.filter((m) => selectedIds.has(m.id)),
    [methodDrafts, selectedIds],
  )

  const onToggleMethod = (id: string, on: boolean) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (on) next.add(id)
      else next.delete(id)
      return next
    })
    if (!defaultMethodId && on) setDefaultMethodId(id)
  }

  const onApplySuggest = async () => {
    if (!connectorId.trim()) return
    setBusy(true)
    setStepError(null)
    try {
      const candidates: HostedLoginMethod[] = methodDrafts
        .map((d) => buildHostedLoginFromDraft({
          enabled: true,
          defaultMethodId: d.id,
          methods: [d],
        }).methods?.[0])
        .filter((m): m is HostedLoginMethod => Boolean(m))
      const { suggestions } = await suggestHostedLogin(connectorId, { candidates })
      if (suggestions.length === 0) {
        push({ tone: 'info', title: EXTERNAL_LOGIN.suggestEmpty })
        return
      }
      setMethodDrafts((drafts) =>
        drafts.map((d) => {
          const s = suggestions.find((x) => x.id === d.id)
          if (!s) return d
          return {
            ...d,
            label: s.label?.trim() ? s.label : d.label,
          }
        }),
      )
      const recommend = suggestions.find((s) => s.recommend_default)
      if (recommend?.id && selectedIds.has(recommend.id)) {
        setDefaultMethodId(recommend.id)
      }
      push({ tone: 'success', title: EXTERNAL_LOGIN.suggestApplied })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: EXTERNAL_LOGIN.suggestFailed, detail: f.detail ?? f.title })
    } finally {
      setBusy(false)
    }
  }

  const saveConnectorConfig = async (): Promise<boolean> => {
    if (!connectorId.trim()) return false
    const draft = {
      enabled: hostedEnabled && selectedMethods.length > 0,
      defaultMethodId: defaultMethodId || selectedMethods[0]?.id || '',
      methods: selectedMethods,
    }
    const config = buildHostedLoginFromDraft(draft)
    if (config.enabled && (!config.methods?.length || !config.default_method_id)) {
      setStepError(EXTERNAL_LOGIN.errMethodsRequired)
      return false
    }
    const conn = await getConnector(connectorId)
    await putConnector(connectorId, {
      type: conn.type,
      base_url: conn.base_url,
      spec: conn.spec,
      execution_callback_url: conn.execution_callback_url,
      auth: {
        ...conn.auth,
        mode: conn.auth?.mode ?? 'static',
        hosted_login: config,
      },
      mcp: conn.mcp,
      require_approval: conn.require_approval,
      require_login: conn.require_login,
    })
    return true
  }

  const bindExportKey = async (): Promise<boolean> => {
    if (!connectorId.trim() || !exportKeyId.trim() || !identityId.trim()) {
      setStepError(EXTERNAL_LOGIN.errKeyRequired)
      return false
    }
    const key = keys.find((k) => k.id === exportKeyId)
    if (!key) {
      setStepError(EXTERNAL_LOGIN.errKeyRequired)
      return false
    }
    const slots = [...(key.identity_slots ?? [])]
    const idx = slots.findIndex((s) => s.connector_id === connectorId)
    const slot = { connector_id: connectorId, identity_id: identityId }
    if (idx >= 0) slots[idx] = slot
    else slots.push(slot)
    await patchMCPExportKey(exportKeyId, { identity_slots: slots })
    return true
  }

  const goStep2 = async () => {
    if (!connectorId.trim()) {
      setStepError(EXTERNAL_LOGIN.errPickConnector)
      return
    }
    setStepError(null)
    setStep(2)
  }

  const goStep3 = () => {
    if (selectedMethods.length === 0) {
      setStepError(EXTERNAL_LOGIN.errPickMethod)
      return
    }
    if (!defaultMethodId || !selectedIds.has(defaultMethodId)) {
      setStepError(EXTERNAL_LOGIN.errDefaultMethod)
      return
    }
    setStepError(null)
    setStep(3)
  }

  const goStep4 = async (e?: FormEvent) => {
    e?.preventDefault()
    setStepError(null)
    setBusy(true)
    try {
      const trimmedBase = publicBaseUrl.trim()
      if (!trimmedBase) {
        setStepError(EXTERNAL_LOGIN.errPublicBase)
        setBusy(false)
        return
      }
      const runtime = await getRuntimeSettings()
      if (trimmedBase !== (runtime.public_base_url ?? '')) {
        const v = await patchRuntimeSettings({ public_base_url: trimmedBase })
        setPublicBaseUrl(v.public_base_url ?? trimmedBase)
      }
      if (!(await saveConnectorConfig())) {
        setBusy(false)
        return
      }
      if (!(await bindExportKey())) {
        setBusy(false)
        return
      }
      setStep(4)
      setTestUrl(null)
    } catch (err) {
      const f = friendlyError(err)
      setStepError(f.detail ?? f.title)
    } finally {
      setBusy(false)
    }
  }

  const issueTestLink = async () => {
    if (!connectorId.trim()) return
    setBusy(true)
    setStepError(null)
    try {
      const res = await createHostedLoginTestSession(connectorId, {
        export_key_id: exportKeyId || undefined,
        method_id: defaultMethodId || undefined,
      })
      setTestUrl(res.url)
    } catch (err) {
      const f = friendlyError(err)
      setStepError(f.detail ?? f.title)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-panel" data-testid="external-login-settings">
      <PageHeader title={EXTERNAL_LOGIN.title} description={EXTERNAL_LOGIN.intro} />
      <ol className="settings-wizard-steps" aria-label={EXTERNAL_LOGIN.stepsAria}>
        {[1, 2, 3, 4].map((n) => (
          <li
            key={n}
            className={step === n ? 'settings-wizard-step is-active' : 'settings-wizard-step'}
            aria-current={step === n ? 'step' : undefined}
          >
            {[EXTERNAL_LOGIN.step1, EXTERNAL_LOGIN.step2, EXTERNAL_LOGIN.step3, EXTERNAL_LOGIN.step4][
              n - 1
            ]}
          </li>
        ))}
      </ol>

      {loading && <p className="settings-muted">{EXTERNAL_LOGIN.loading}</p>}
      {stepError && (
        <p className="ui-inline-error" role="alert">
          {stepError}
        </p>
      )}

      {!loading && step === 1 && (
        <section className="settings-form">
          <Field label={EXTERNAL_LOGIN.connectorLabel} hint={EXTERNAL_LOGIN.connectorHint}>
            <Select
              value={connectorId}
              data-testid="external-login-connector"
              onChange={(e) => setConnectorId(e.target.value)}
            >
              <option value="">{EXTERNAL_LOGIN.connectorPlaceholder}</option>
              {connectors.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.id} ({c.type})
                </option>
              ))}
            </Select>
          </Field>
          <p className="settings-meta">
            {EXTERNAL_LOGIN.connectorLinkPrefix}{' '}
            <Link to="/settings/openapi">{EXTERNAL_LOGIN.connectorLinkOpenApi}</Link>
            {' · '}
            <Link to="/settings/plugins">{EXTERNAL_LOGIN.connectorLinkPlugins}</Link>
          </p>
          <div className="settings-toolbar">
            <Button type="button" disabled={busy} onClick={() => void goStep2()}>
              {EXTERNAL_LOGIN.next}
            </Button>
          </div>
        </section>
      )}

      {!loading && step === 2 && (
        <section className="settings-form">
          <p className="settings-meta">{EXTERNAL_LOGIN.methodsIntro}</p>
          {methodDrafts.length === 0 && (
            <p className="settings-muted">{EXTERNAL_LOGIN.noCandidates}</p>
          )}
          <ul className="settings-card-list">
            {methodDrafts.map((m) => (
              <li key={m.id} className="settings-card">
                <label className="settings-check">
                  <input
                    type="checkbox"
                    checked={selectedIds.has(m.id)}
                    onChange={(e) => onToggleMethod(m.id, e.target.checked)}
                  />{' '}
                  {m.label || m.id}
                </label>
                <label className="settings-check">
                  <input
                    type="radio"
                    name="default-method"
                    checked={defaultMethodId === m.id}
                    disabled={!selectedIds.has(m.id)}
                    onChange={() => setDefaultMethodId(m.id)}
                  />{' '}
                  {EXTERNAL_LOGIN.defaultMethod}
                </label>
                <Field label={EXTERNAL_LOGIN.methodLabel}>
                  <Input
                    value={m.label}
                    onChange={(e) =>
                      setMethodDrafts((drafts) =>
                        drafts.map((d) => (d.id === m.id ? { ...d, label: e.target.value } : d)),
                      )
                    }
                  />
                </Field>
                <p className="settings-muted">
                  {m.mode}
                  {m.loginTool ? ` · ${m.loginTool}` : ''}
                </p>
              </li>
            ))}
          </ul>
          <div className="settings-toolbar">
            <Button type="button" variant="secondary" size="sm" disabled={busy} onClick={() => void onApplySuggest()}>
              {EXTERNAL_LOGIN.suggest}
            </Button>
          </div>
          <label className="settings-check">
            <input
              type="checkbox"
              checked={hostedEnabled}
              onChange={(e) => setHostedEnabled(e.target.checked)}
            />{' '}
            {EXTERNAL_LOGIN.enableHosted}
          </label>
          <div className="settings-toolbar">
            <Button type="button" variant="secondary" disabled={busy} onClick={() => setStep(1)}>
              {EXTERNAL_LOGIN.back}
            </Button>
            <Button type="button" disabled={busy} onClick={goStep3}>
              {EXTERNAL_LOGIN.next}
            </Button>
          </div>
        </section>
      )}

      {!loading && step === 3 && (
        <form className="settings-form" onSubmit={(e) => void goStep4(e)}>
          <Field label={EXTERNAL_LOGIN.publicBaseLabel} hint={EXTERNAL_LOGIN.publicBaseHint}>
            <Input
              value={publicBaseUrl}
              data-testid="external-login-public-base"
              onChange={(e) => setPublicBaseUrl(e.target.value)}
              placeholder="https://baize.example.com"
            />
          </Field>
          <Field label={EXTERNAL_LOGIN.exportKeyLabel} hint={EXTERNAL_LOGIN.exportKeyHint}>
            <Select value={exportKeyId} onChange={(e) => setExportKeyId(e.target.value)}>
              <option value="">{EXTERNAL_LOGIN.exportKeyPlaceholder}</option>
              {keys.map((k) => (
                <option key={k.id} value={k.id}>
                  {k.name} ({k.id})
                </option>
              ))}
            </Select>
          </Field>
          <Field label={EXTERNAL_LOGIN.identityLabel} hint={EXTERNAL_LOGIN.identityHint}>
            <Select value={identityId} onChange={(e) => setIdentityId(e.target.value)}>
              <option value="">{EXTERNAL_LOGIN.identityPlaceholder}</option>
              {identities.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.name} ({i.id})
                </option>
              ))}
            </Select>
          </Field>
          <div className="settings-toolbar">
            <Button type="button" variant="secondary" disabled={busy} onClick={() => setStep(2)}>
              {EXTERNAL_LOGIN.back}
            </Button>
            <Button type="submit" disabled={busy}>
              {EXTERNAL_LOGIN.saveAndContinue}
            </Button>
          </div>
        </form>
      )}

      {!loading && step === 4 && (
        <section className="settings-form">
          <p className="settings-meta">{EXTERNAL_LOGIN.doneIntro}</p>
          <div className="settings-toolbar">
            <Button type="button" disabled={busy} onClick={() => void issueTestLink()}>
              {EXTERNAL_LOGIN.issueTestLink}
            </Button>
          </div>
          {testUrl && (
            <p className="settings-meta">
              <a href={testUrl} target="_blank" rel="noreferrer">
                {EXTERNAL_LOGIN.openTestLogin}
              </a>
            </p>
          )}
          <div className="settings-toolbar">
            <Button type="button" variant="secondary" onClick={() => setStep(1)}>
              {EXTERNAL_LOGIN.configureAnother}
            </Button>
          </div>
        </section>
      )}

      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </div>
  )
}

export { WIZARD_STEPS as externalLoginWizardStepCount }
