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
  patchTool,
  putConnector,
  suggestHostedLogin,
  type ConnectorInfo,
  type HostedLoginMethod,
  type MCPExportIdentity,
  type MCPExportIdentitySlot,
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
  methodDraftEffectiveRole,
  methodDraftFromCandidate,
  type HostedLoginMethodDraft,
} from './hostedLoginForm'

const WIZARD_STEPS = 4

function connectorEligible(c: ConnectorInfo): boolean {
  return c.type === 'openapi' || c.type === 'http'
}

function connectorHasHostedLogin(c: ConnectorInfo): boolean {
  const hl = c.auth?.hosted_login
  if (!hl?.enabled) return false
  return (hl.methods?.length ?? 0) > 0
}

function keyHasConnectorSlot(key: MCPExportKey, connectorId: string): boolean {
  return (key.identity_slots ?? []).some((s) => s.connector_id === connectorId && s.identity_id)
}

function keyReferencesIdentities(key: MCPExportKey, identityIds: Set<string>): boolean {
  if (identityIds.has(key.identity_id)) return true
  return (key.identity_slots ?? []).some(
    (s) => Boolean(s.identity_id) && identityIds.has(s.identity_id),
  )
}

/** Keys already tied to the bound export identities (or this connector). */
function resolveLinkedExportKeys(
  keys: MCPExportKey[],
  methods: HostedLoginMethodDraft[],
  connectorId: string,
): MCPExportKey[] {
  const identityIds = new Set(
    methods.flatMap((m) => m.identityIds.map((x) => x.trim()).filter(Boolean)),
  )
  if (identityIds.size === 0) return []
  return keys.filter(
    (k) => keyReferencesIdentities(k, identityIds) || keyHasConnectorSlot(k, connectorId),
  )
}

function upsertIdentitySlot(
  slots: MCPExportIdentitySlot[],
  slot: MCPExportIdentitySlot,
): MCPExportIdentitySlot[] {
  const connector = slot.connector_id ?? ''
  const role = slot.role ?? ''
  const next = [...slots]
  const idx = next.findIndex(
    (s) => (s.connector_id ?? '') === connector && (s.role ?? '') === role,
  )
  if (idx >= 0) next[idx] = slot
  else next.push(slot)
  return next
}

/** Drop slots whose identity was deleted, so PATCH does not 400. */
function slotsWithKnownIdentities(
  slots: MCPExportIdentitySlot[],
  knownIds: Set<string>,
): MCPExportIdentitySlot[] {
  return slots.filter((s) => knownIds.has((s.identity_id ?? '').trim()))
}

function knownIdentityIdSet(identities: MCPExportIdentity[]): Set<string> {
  return new Set(identities.map((i) => i.id.trim()).filter(Boolean))
}

function unionTools(existing: string[] | undefined, add: string[]): string[] {
  const out = new Set((existing ?? []).map((t) => t.trim()).filter(Boolean))
  for (const t of add) {
    const n = t.trim()
    if (n) out.add(n)
  }
  return [...out]
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
    if (loading || connectors.length === 0 || connectorId) return
    if (preselectConnector && connectors.some((c) => c.id === preselectConnector)) {
      setConnectorId(preselectConnector)
      return
    }
    const configured = connectors.find(connectorHasHostedLogin)
    if (configured) setConnectorId(configured.id)
  }, [preselectConnector, loading, connectors, connectorId])

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

      // Prefer identity_ids persisted on methods; else backfill from Key slots.
      const slotSource = keys.find((k) => keyHasConnectorSlot(k, id))
      if (slotSource) {
        drafts = drafts.map((m) => {
          if (m.identityIds.length > 0) return m
          if (!selected.has(m.id)) return m
          const role = methodDraftEffectiveRole(m)
          const primary = slotSource.identity_slots?.find(
            (s) => s.connector_id === id && (s.role ?? '') === role && s.identity_id,
          )?.identity_id
          const extras = (slotSource.identity_slots ?? [])
            .filter(
              (s) =>
                s.connector_id === id &&
                s.identity_id &&
                (s.role ?? '') === s.identity_id &&
                s.identity_id !== primary,
            )
            .map((s) => s.identity_id as string)
          const connectorDefault = slotSource.identity_slots?.find(
            (s) => s.connector_id === id && !(s.role ?? '') && s.identity_id,
          )?.identity_id
          const ids = [
            ...new Set(
              [primary, connectorDefault, ...extras, slotSource.identity_id]
                .map((x) => (x ?? '').trim())
                .filter(Boolean),
            ),
          ]
          return ids.length > 0 ? { ...m, identityIds: ids } : m
        })
        setMethodDrafts(drafts)
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

  const linkedExportKeys = useMemo(
    () => resolveLinkedExportKeys(keys, selectedMethods, connectorId),
    [keys, selectedMethods, connectorId],
  )

  const selectedAlreadyConfigured = useMemo(() => {
    const c = connectors.find((x) => x.id === connectorId)
    return Boolean(c && connectorHasHostedLogin(c))
  }, [connectors, connectorId])

  const onToggleMethod = (id: string, on: boolean) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (on) next.add(id)
      else next.delete(id)
      return next
    })
    if (!defaultMethodId && on) setDefaultMethodId(id)
  }

  /** Identity-first toggle: which login methods an export identity may use. */
  const onToggleIdentityMethod = (identityId: string, methodId: string, on: boolean) => {
    setMethodDrafts((drafts) =>
      drafts.map((d) => {
        if (d.id !== methodId) return d
        if (on) {
          if (d.identityIds.includes(identityId)) return d
          return { ...d, identityIds: [...d.identityIds, identityId] }
        }
        return { ...d, identityIds: d.identityIds.filter((x) => x !== identityId) }
      }),
    )
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

  const bindExportKeysAndLoginTools = async (): Promise<boolean> => {
    if (!connectorId.trim()) return false
    const knownIds = knownIdentityIdSet(identities)
    // Drop deleted / stale ids left on methods or key slots (common after identity cleanup).
    const methodsWithKnown = selectedMethods.map((m) => ({
      ...m,
      identityIds: m.identityIds.map((x) => x.trim()).filter((id) => knownIds.has(id)),
    }))
    const anyBound = methodsWithKnown.some((m) => m.identityIds.length > 0)
    if (!anyBound) {
      setStepError(EXTERNAL_LOGIN.errMethodIdentity)
      return false
    }
    const targetKeys = resolveLinkedExportKeys(keys, methodsWithKnown, connectorId)
    if (targetKeys.length === 0) {
      setStepError(EXTERNAL_LOGIN.errNoLinkedKey)
      return false
    }
    const loginTools = [
      ...new Set(
        methodsWithKnown.map((m) => m.loginTool.trim()).filter(Boolean),
      ),
    ]
    for (const toolName of loginTools) {
      await patchTool(toolName, { export: 'force_allow' })
    }

    const defaultId = defaultMethodId || methodsWithKnown[0]?.id || ''
    const defaultMethod =
      methodsWithKnown.find((m) => m.id === defaultId) ?? methodsWithKnown[0]
    const defaultIdentity = defaultMethod?.identityIds[0]?.trim() ?? ''

    for (const key of targetKeys) {
      let slots = slotsWithKnownIdentities(key.identity_slots ?? [], knownIds)
      for (const m of methodsWithKnown) {
        const ids = m.identityIds
        if (ids.length === 0) continue
        const primary = ids[0]
        const role = methodDraftEffectiveRole(m)
        slots = upsertIdentitySlot(slots, {
          connector_id: connectorId,
          role,
          identity_id: primary,
        })
        for (const identityId of ids.slice(1)) {
          slots = upsertIdentitySlot(slots, {
            connector_id: connectorId,
            role: identityId,
            identity_id: identityId,
          })
        }
      }
      if (defaultIdentity) {
        slots = upsertIdentitySlot(slots, {
          connector_id: connectorId,
          identity_id: defaultIdentity,
        })
      }
      await patchMCPExportKey(key.id, {
        identity_slots: slots,
        allowed_tools: unionTools(key.allowed_tools, loginTools),
      })
    }
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
      if (!(await bindExportKeysAndLoginTools())) {
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

  const issueLoginLink = async (openInNewTab: boolean) => {
    if (!connectorId.trim()) return
    setBusy(true)
    setStepError(null)
    try {
      const firstKey = resolveLinkedExportKeys(keys, selectedMethods, connectorId)[0]?.id
      const res = await createHostedLoginTestSession(connectorId, {
        export_key_id: firstKey || undefined,
        method_id: defaultMethodId || undefined,
      })
      setTestUrl(res.url)
      if (openInNewTab && res.url) {
        window.open(res.url, '_blank', 'noopener,noreferrer')
      }
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
          {connectors.some(connectorHasHostedLogin) && (
            <p className="settings-meta" data-testid="external-login-configured-hint">
              {EXTERNAL_LOGIN.configuredHint}
            </p>
          )}
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
                  {connectorHasHostedLogin(c) ? ` · ${EXTERNAL_LOGIN.configuredSuffix}` : ''}
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
            {selectedAlreadyConfigured ? (
              <>
                <Button
                  type="button"
                  variant="primary"
                  disabled={busy || !connectorId}
                  data-testid="external-login-open-shortcut"
                  onClick={() => void issueLoginLink(true)}
                >
                  {EXTERNAL_LOGIN.openLoginShortcut}
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  disabled={busy || !connectorId}
                  data-testid="external-login-edit-config"
                  onClick={() => void goStep2()}
                >
                  {EXTERNAL_LOGIN.editConfig}
                </Button>
              </>
            ) : (
              <Button type="button" disabled={busy} onClick={() => void goStep2()}>
                {EXTERNAL_LOGIN.next}
              </Button>
            )}
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
                  {EXTERNAL_LOGIN.enableMethod}: {m.label || m.id}
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
                <Field label={EXTERNAL_LOGIN.methodRoleLabel} hint={EXTERNAL_LOGIN.methodRoleHint}>
                  <Input
                    value={m.role}
                    placeholder={m.id}
                    onChange={(e) =>
                      setMethodDrafts((drafts) =>
                        drafts.map((d) => (d.id === m.id ? { ...d, role: e.target.value } : d)),
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
          <Field label={EXTERNAL_LOGIN.identityMethodLabel} hint={EXTERNAL_LOGIN.identityMethodHint}>
            {identities.length === 0 ? (
              <p className="settings-muted">
                {EXTERNAL_LOGIN.noIdentities}{' '}
                <Link to="/settings/mcp-export">{EXTERNAL_LOGIN.goMcpExport}</Link>
              </p>
            ) : (
              <ul className="settings-card-list" data-testid="external-login-identity-methods">
                {identities.map((i) => (
                  <li key={i.id} className="settings-card">
                    <p className="settings-meta">
                      {i.name} ({i.id})
                    </p>
                    <ul className="settings-card-list">
                      {selectedMethods.map((m) => (
                        <li key={m.id}>
                          <label className="settings-check">
                            <input
                              type="checkbox"
                              checked={m.identityIds.includes(i.id)}
                              onChange={(e) =>
                                onToggleIdentityMethod(i.id, m.id, e.target.checked)
                              }
                            />{' '}
                            {m.label || m.id}
                          </label>
                        </li>
                      ))}
                    </ul>
                  </li>
                ))}
              </ul>
            )}
          </Field>
          <p className="settings-meta" data-testid="external-login-keys-auto-hint">
            {linkedExportKeys.length > 0
              ? EXTERNAL_LOGIN.keysAutoHint(linkedExportKeys.length)
              : EXTERNAL_LOGIN.keysAutoHintEmpty}{' '}
            <Link to="/settings/mcp-export">{EXTERNAL_LOGIN.goMcpExport}</Link>
          </p>
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
          <p className="settings-meta" data-testid="external-login-credential-dest">
            {EXTERNAL_LOGIN.credentialDestHint}{' '}
            <Link to="/settings/mcp-export">{EXTERNAL_LOGIN.goMcpExport}</Link>
          </p>
          <div className="settings-toolbar">
            <Button type="button" disabled={busy} onClick={() => void issueLoginLink(true)}>
              {EXTERNAL_LOGIN.issueTestLink}
            </Button>
            {testUrl && (
              <Button
                type="button"
                variant="secondary"
                onClick={() => window.open(testUrl, '_blank', 'noopener,noreferrer')}
              >
                {EXTERNAL_LOGIN.openTestLogin}
              </Button>
            )}
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setStep(1)
                setTestUrl(null)
                setConnectorId('')
              }}
            >
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
