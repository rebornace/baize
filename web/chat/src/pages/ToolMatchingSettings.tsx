import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  cleanupSystemOne,
  cleanupToolRetrieval,
  disableSystemOne,
  disableToolRetrieval,
  enableSystemOneAPI,
  enableSystemOneLocal,
  enableToolRetrieval,
  enableToolRetrievalAPI,
  getSystemOne,
  getToolRetrieval,
  setToolRetrievalModelsDir,
  type SystemOneStatus,
  type ToolRetrievalPhase,
  type ToolRetrievalStatus,
} from '../api'
import {
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

const BUSY_PHASES: ToolRetrievalPhase[] = [
  'checking',
  'downloading_installer',
  'launching_installer',
  'waiting_ollama',
  'pulling_model',
  'probing',
]

function mirrorLabel(id?: string): string {
  if (id === 'modelscope') return RUNTIME.toolMatchMirrorModelScope
  if (id === 'cn') return RUNTIME.toolMatchMirrorCN
  if (id === 'ghfast') return RUNTIME.toolMatchMirrorGHFast
  if (id === 'cnb') return RUNTIME.toolMatchMirrorCNB
  if (id === 'official') return RUNTIME.toolMatchMirrorOfficial
  if (id === 'github') return RUNTIME.toolMatchMirrorGitHub
  if (id === 'model') return RUNTIME.toolMatchModel
  return id || ''
}

function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '0 MB'
  const mb = n / (1024 * 1024)
  if (mb >= 1024) return `${(mb / 1024).toFixed(2)} GB`
  if (mb >= 10) return `${Math.round(mb)} MB`
  return `${mb.toFixed(1)} MB`
}

function phaseLabel(phase: ToolRetrievalPhase, detail?: string): string {
  const map = RUNTIME.toolMatchPhase as Record<string, string>
  const base = map[phase] ?? phase
  if (!detail) return base
  if (detail === 'switch_mirror') return RUNTIME.toolMatchSwitchMirror
  if (detail === 'open_download_page') return RUNTIME.toolMatchOpenCNPage
  if (detail === 'starting_local_ollama') return RUNTIME.toolMatchInstalledNotRunning
  if (phase === 'pulling_model' && detail) return `${base} — ${detail}`
  return base
}

function errorLabel(code?: string): string {
  if (!code) return ''
  const map = RUNTIME.toolMatchPhase as Record<string, string>
  if (map[code]) return map[code]
  if (code === 'ollama_installed_but_not_running') return RUNTIME.toolMatchInstalledNotRunning
  // Pull errors may arrive as "pull: ollama_version_too_old" or raw Ollama text.
  if (code.includes('ollama_version_too_old') || /newer version of ollama/i.test(code)) {
    return map.ollama_version_too_old || code
  }
  return code
}

function matchModeTitle(st: ToolRetrievalStatus | null, busy: boolean, enhanced: boolean): string {
  if (enhanced) return RUNTIME.toolMatchModeEnhanced
  if (!st) return RUNTIME.toolMatchModeStandard
  if (busy) return RUNTIME.toolMatchModeBusy
  if (st.phase === 'failed') return RUNTIME.toolMatchModeFailed
  return RUNTIME.toolMatchModeStandard
}

function decideModeTitle(st: SystemOneStatus | null, busy: boolean, on: boolean): string {
  if (on) return RUNTIME.toolMatchDecideModeOn
  if (!st) return RUNTIME.toolMatchDecideModeOff
  if (busy) return RUNTIME.toolMatchDecideModeBusy
  if (st.phase === 'failed') return RUNTIME.toolMatchDecideModeFailed
  return RUNTIME.toolMatchDecideModeOff
}

function providerLabel(provider?: string): string {
  if (provider === 'api') return RUNTIME.toolMatchProviderAPI
  if (provider === 'local') return RUNTIME.toolMatchProviderLocal
  return ''
}

function matchEnableLabel(st: ToolRetrievalStatus | null, busy: boolean): string {
  if (busy) return RUNTIME.toolMatchModeBusy
  if (st?.phase === 'failed' && st.provider !== 'api') return RUNTIME.toolMatchRetry
  if (st?.ollama_running && st.model_present) return RUNTIME.toolMatchEnableReady
  if (st?.ollama_running && !st.model_present) return RUNTIME.toolMatchPullModel
  if (st?.ollama_installed && !st.ollama_running) {
    return st.model_present ? RUNTIME.toolMatchStartAndEnable : RUNTIME.toolMatchStartAndPull
  }
  return RUNTIME.toolMatchEnable
}

function decideEnableLabel(st: SystemOneStatus | null, busy: boolean): string {
  if (busy) return RUNTIME.toolMatchDecideModeBusy
  if (st?.phase === 'failed' && st.provider !== 'api') return RUNTIME.toolMatchDecideRetry
  if (st?.ollama_running && st.model_present) return RUNTIME.toolMatchDecideEnableReady
  if (st?.ollama_running && !st.model_present) return RUNTIME.toolMatchDecidePullModel
  if (st?.ollama_installed && !st.ollama_running) {
    return st.model_present
      ? RUNTIME.toolMatchDecideStartAndEnable
      : RUNTIME.toolMatchDecideStartAndPull
  }
  return RUNTIME.toolMatchDecideEnable
}

function pathSourceLabel(source?: string): string {
  if (source === 'baize') return RUNTIME.toolMatchPathSourceBaize
  if (source === 'env') return RUNTIME.toolMatchPathSourceEnv
  return RUNTIME.toolMatchPathSourceDefault
}

function ProgressBlock({
  st,
}: {
  st: { phase: string; download?: ToolRetrievalStatus['download'] }
}) {
  const show =
    !!st.download &&
    (st.phase === 'downloading_installer' || st.phase === 'pulling_model')
  if (!show || !st.download) return null
  const d = st.download
  return (
    <div className="tool-matching-progress" data-testid="tool-matching-progress">
      <div className="tool-matching-progress-meta">
        <span>{mirrorLabel(d.mirror)}</span>
        <span>
          {d.total > 0
            ? RUNTIME.toolMatchDownloadProgress(
                formatBytes(d.bytes),
                formatBytes(d.total),
                d.percent,
              )
            : RUNTIME.toolMatchDownloadProgressUnknown(formatBytes(d.bytes))}
        </span>
      </div>
      <div
        className="tool-matching-progress-track"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={d.total > 0 ? d.percent : undefined}
      >
        <div
          className="tool-matching-progress-fill"
          style={{
            width: d.total > 0 ? `${Math.min(100, Math.max(0, d.percent))}%` : '30%',
            opacity: d.total > 0 ? 1 : 0.45,
          }}
        />
      </div>
    </div>
  )
}

export function ToolMatchingSettings() {
  const { role } = useGate()
  const readOnly = role !== 'admin'
  const { toasts, push, dismiss } = useToast()

  const [st, setSt] = useState<ToolRetrievalStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [apiBase, setApiBase] = useState('')
  const [apiModel, setApiModel] = useState('text-embedding-3-small')
  const [apiKey, setApiKey] = useState('')
  const [apiBusy, setApiBusy] = useState(false)
  const [cleanupOpen, setCleanupOpen] = useState(false)
  const [cleanupBusy, setCleanupBusy] = useState(false)
  const [removeOllama, setRemoveOllama] = useState(false)
  const [modelsDirDraft, setModelsDirDraft] = useState('')
  const [pathsBusy, setPathsBusy] = useState(false)

  const [dec, setDec] = useState<SystemOneStatus | null>(null)
  const [decLoading, setDecLoading] = useState(true)
  const [decApiBase, setDecApiBase] = useState('')
  const [decApiModel, setDecApiModel] = useState('tev1')
  const [decApiKey, setDecApiKey] = useState('')
  const [decApiBusy, setDecApiBusy] = useState(false)
  const [decCleanupOpen, setDecCleanupOpen] = useState(false)
  const [decCleanupBusy, setDecCleanupBusy] = useState(false)

  const applyMatch = useCallback((next: ToolRetrievalStatus, syncPath = false) => {
    setSt(next)
    if (syncPath) setModelsDirDraft(next.paths?.models_dir ?? '')
    if (next.provider === 'api' && next.base_url) setApiBase(next.base_url)
    if (next.provider === 'api' && next.model) setApiModel(next.model)
  }, [])

  const applyDecide = useCallback((next: SystemOneStatus) => {
    setDec(next)
    if (next.provider === 'api' && next.base_url) setDecApiBase(next.base_url)
    if (next.provider === 'api' && next.model) setDecApiModel(next.model)
  }, [])

  const loadMatch = useCallback(async () => {
    setLoadError(null)
    try {
      applyMatch(await getToolRetrieval(), true)
    } catch (err) {
      const f = friendlyError(err)
      setLoadError(f.detail ?? f.title)
      setSt(null)
    } finally {
      setLoading(false)
    }
  }, [applyMatch])

  const loadDecide = useCallback(async () => {
    try {
      applyDecide(await getSystemOne())
    } catch {
      setDec(null)
    } finally {
      setDecLoading(false)
    }
  }, [applyDecide])

  useEffect(() => {
    void loadMatch()
    void loadDecide()
  }, [loadMatch, loadDecide])

  const matchBusy = !!st && (st.busy || BUSY_PHASES.includes(st.phase))
  const decideBusy = !!dec && (dec.busy || BUSY_PHASES.includes(dec.phase))

  useEffect(() => {
    if (!matchBusy) return
    const id = window.setInterval(() => {
      void getToolRetrieval()
        .then((next) => {
          applyMatch(next, false)
          setLoadError(null)
        })
        .catch(() => {})
    }, 1500)
    return () => window.clearInterval(id)
  }, [matchBusy, applyMatch])

  useEffect(() => {
    if (!decideBusy) return
    const id = window.setInterval(() => {
      void getSystemOne()
        .then((next) => applyDecide(next))
        .catch(() => {})
    }, 1500)
    return () => window.clearInterval(id)
  }, [decideBusy, applyDecide])

  const enhanced = !!st && st.mode === 'enhanced' && st.phase === 'ready'
  const decideOn = !!dec && dec.mode === 'on' && dec.phase === 'ready'
  const anyMatchBusy = matchBusy || apiBusy || cleanupBusy || pathsBusy
  const anyDecideBusy = decideBusy || decApiBusy || decCleanupBusy
  const paths = st?.paths

  const onEnableMatchLocal = async () => {
    try {
      const next = await enableToolRetrieval()
      applyMatch(next, true)
      push({ tone: 'info', title: matchEnableLabel(next, false) })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }

  const onEnableMatchAPI = async (e: FormEvent) => {
    e.preventDefault()
    if (!apiBase.trim()) {
      push({ tone: 'error', title: RUNTIME.toolMatchAPIBaseRequired })
      return
    }
    setApiBusy(true)
    try {
      const next = await enableToolRetrievalAPI({
        base_url: apiBase.trim(),
        model: apiModel.trim() || 'text-embedding-3-small',
        api_key: apiKey.trim() || undefined,
      })
      applyMatch(next, true)
      setApiKey('')
      if (next.phase === 'failed' || next.mode !== 'enhanced') {
        push({
          tone: 'error',
          title: RUNTIME.toolMatchModeFailed,
          detail: next.error || next.detail,
        })
      } else {
        push({ tone: 'success', title: RUNTIME.toolMatchToastEnabled })
      }
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setApiBusy(false)
    }
  }

  const onDisableMatch = async () => {
    const wasBusy = matchBusy
    try {
      applyMatch(await disableToolRetrieval(), true)
      push({
        tone: 'success',
        title: wasBusy ? RUNTIME.toolMatchToastCancelled : RUNTIME.toolMatchToastDisabled,
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }

  const onSaveModelsDir = async (dir: string) => {
    setPathsBusy(true)
    try {
      applyMatch(await setToolRetrievalModelsDir(dir), true)
      push({ tone: 'success', title: RUNTIME.toolMatchToastPathsSaved })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setPathsBusy(false)
    }
  }

  const onCleanupMatch = async () => {
    setCleanupBusy(true)
    try {
      const res = await cleanupToolRetrieval({ remove_ollama: removeOllama })
      applyMatch(res.status, true)
      setCleanupOpen(false)
      setRemoveOllama(false)
      const freed = res.cleanup.installer_cache_bytes_removed
      push({
        tone: 'success',
        title: RUNTIME.toolMatchToastCleanup,
        detail: [
          res.cleanup.removed_ollama ? RUNTIME.toolMatchCleanupRemoveApp : null,
          res.cleanup.removed_model && !res.cleanup.removed_ollama
            ? RUNTIME.toolMatchModel
            : null,
          freed > 0 ? RUNTIME.toolMatchCacheSize(formatBytes(freed)) : null,
        ]
          .filter(Boolean)
          .join(' · '),
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setCleanupBusy(false)
    }
  }

  const onEnableDecideLocal = async () => {
    try {
      const next = await enableSystemOneLocal()
      applyDecide(next)
      push({ tone: 'info', title: decideEnableLabel(next, false) })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }

  const onEnableDecideAPI = async (e: FormEvent) => {
    e.preventDefault()
    if (!decApiBase.trim()) {
      push({ tone: 'error', title: RUNTIME.toolMatchAPIBaseRequired })
      return
    }
    setDecApiBusy(true)
    try {
      const next = await enableSystemOneAPI({
        base_url: decApiBase.trim(),
        model: decApiModel.trim() || 'tev1',
        api_key: decApiKey.trim() || undefined,
      })
      applyDecide(next)
      setDecApiKey('')
      if (next.phase === 'failed' || next.mode !== 'on') {
        push({
          tone: 'error',
          title: RUNTIME.toolMatchDecideModeFailed,
          detail: next.error || next.detail,
        })
      } else {
        push({ tone: 'success', title: RUNTIME.toolMatchDecideToastEnabled })
      }
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setDecApiBusy(false)
    }
  }

  const onDisableDecide = async () => {
    const wasBusy = decideBusy
    try {
      applyDecide(await disableSystemOne())
      push({
        tone: 'success',
        title: wasBusy
          ? RUNTIME.toolMatchDecideToastCancelled
          : RUNTIME.toolMatchDecideToastDisabled,
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }

  const onCleanupDecide = async () => {
    setDecCleanupBusy(true)
    try {
      const res = await cleanupSystemOne()
      applyDecide(res.status)
      setDecCleanupOpen(false)
      push({
        tone: 'success',
        title: RUNTIME.toolMatchDecideToastCleanup,
        detail: res.cleanup.removed_model ? 'tev1' : undefined,
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setDecCleanupBusy(false)
    }
  }

  return (
    <div className="settings-section">
      <PageHeader title={RUNTIME.sectionToolMatch} description={RUNTIME.toolMatchHint} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {/* —— Matching —— */}
      <h2 className="settings-subheading">{RUNTIME.toolMatchSectionEmbed}</h2>
      <p className="settings-muted">{RUNTIME.toolMatchSectionEmbedHint}</p>

      <section className="settings-form tool-matching-hero" data-testid="tool-matching-panel">
        {loading ? <p className="settings-muted">{RUNTIME.loading}</p> : null}
        {!loading ? (
          <div className="tool-matching-status">
            <p className="tool-matching-status-title">
              <strong>{matchModeTitle(st, matchBusy, enhanced)}</strong>
              {providerLabel(st?.provider) ? (
                <span className="settings-muted"> · {providerLabel(st?.provider)}</span>
              ) : null}
            </p>
            {st ? (
              <>
                <p className="settings-muted">{phaseLabel(st.phase, st.detail)}</p>
                {st.ollama_running ? (
                  <p className="settings-muted">
                    {st.model_present
                      ? RUNTIME.toolMatchModelReady
                      : RUNTIME.toolMatchModelMissing}
                  </p>
                ) : st.ollama_installed ? (
                  <p className="settings-muted">{RUNTIME.toolMatchInstalledNotRunning}</p>
                ) : null}
                <ProgressBlock st={st} />
                {st.error ? <p className="settings-muted">{errorLabel(st.error)}</p> : null}
                <p className="settings-muted">
                  {RUNTIME.toolMatchModel}: {st.model || '—'}
                  {st.base_url ? ` · ${st.base_url}` : ''}
                </p>
              </>
            ) : null}
            {loadError ? (
              <p className="settings-muted" data-testid="tool-matching-load-error">
                {loadError}
              </p>
            ) : null}
          </div>
        ) : null}

        {!readOnly && (enhanced || matchBusy) ? (
          <div className="settings-actions tool-matching-actions">
            <Button
              type="button"
              variant="secondary"
              data-testid="tool-matching-disable"
              onClick={() => void onDisableMatch()}
            >
              {matchBusy ? RUNTIME.toolMatchCancel : RUNTIME.toolMatchDisable}
            </Button>
          </div>
        ) : null}
        {readOnly ? <p className="settings-muted">{RUNTIME.descriptionOperator}</p> : null}
      </section>

      {!readOnly && !enhanced ? (
        <>
          <section className="settings-form tool-matching-hero" data-testid="tool-matching-local">
            <h2 className="settings-subheading">{RUNTIME.toolMatchLocalTitle}</h2>
            <p className="settings-muted">{RUNTIME.toolMatchLocalHint}</p>
            <div className="settings-actions tool-matching-actions">
              <Button
                type="button"
                variant="primary"
                disabled={anyMatchBusy || loading}
                data-testid="tool-matching-enable"
                onClick={() => void onEnableMatchLocal()}
              >
                {matchEnableLabel(st, matchBusy)}
              </Button>
            </div>
          </section>

          <form
            className="settings-form tool-matching-hero"
            data-testid="tool-matching-api"
            onSubmit={(e) => void onEnableMatchAPI(e)}
          >
            <h2 className="settings-subheading">{RUNTIME.toolMatchAPITitle}</h2>
            <p className="settings-muted">{RUNTIME.toolMatchAPIHint}</p>
            <Field label={RUNTIME.toolMatchAPIBase} hint={RUNTIME.toolMatchAPIBaseHint}>
              <Input
                type="url"
                placeholder="https://api.openai.com/v1"
                value={apiBase}
                onChange={(e) => setApiBase(e.target.value)}
                disabled={anyMatchBusy}
                data-testid="tool-matching-api-base"
              />
            </Field>
            <Field label={RUNTIME.toolMatchAPIModel} hint={RUNTIME.toolMatchAPIModelHint}>
              <Input
                value={apiModel}
                onChange={(e) => setApiModel(e.target.value)}
                disabled={anyMatchBusy}
                data-testid="tool-matching-api-model"
              />
            </Field>
            <Field
              label={RUNTIME.toolMatchAPIKey}
              hint={
                st?.api_key_set
                  ? RUNTIME.toolMatchAPIKeyHintSet
                  : RUNTIME.toolMatchAPIKeyHint
              }
            >
              <Input
                type="password"
                autoComplete="off"
                placeholder={st?.api_key_set ? '••••••••' : ''}
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                disabled={anyMatchBusy}
                data-testid="tool-matching-api-key"
              />
            </Field>
            <div className="settings-actions tool-matching-actions">
              <Button
                type="submit"
                variant="primary"
                disabled={anyMatchBusy || loading}
                data-testid="tool-matching-api-enable"
              >
                {apiBusy ? RUNTIME.toolMatchModeBusy : RUNTIME.toolMatchAPIEnable}
              </Button>
            </div>
          </form>
        </>
      ) : null}

      {/* —— Decision —— */}
      <h2 className="settings-subheading">{RUNTIME.toolMatchSectionDecide}</h2>
      <p className="settings-muted">{RUNTIME.toolMatchSectionDecideHint}</p>

      <section className="settings-form tool-matching-hero" data-testid="decide-systemone-panel">
        {decLoading ? <p className="settings-muted">{RUNTIME.loading}</p> : null}
        {!decLoading ? (
          <div className="tool-matching-status">
            <p className="tool-matching-status-title">
              <strong>{decideModeTitle(dec, decideBusy, decideOn)}</strong>
              {providerLabel(dec?.provider) ? (
                <span className="settings-muted"> · {providerLabel(dec?.provider)}</span>
              ) : null}
            </p>
            {dec ? (
              <>
                <p className="settings-muted">{phaseLabel(dec.phase, dec.detail)}</p>
                {dec.provider !== 'api' && dec.ollama_running ? (
                  <p className="settings-muted">
                    {dec.model_present
                      ? RUNTIME.toolMatchDecideModelReady
                      : RUNTIME.toolMatchDecideModelMissing}
                  </p>
                ) : null}
                <ProgressBlock st={dec} />
                {dec.error ? <p className="settings-muted">{errorLabel(dec.error)}</p> : null}
                <p className="settings-muted">
                  {RUNTIME.fieldDecideSystemOneModel}: {dec.model || 'tev1'}
                  {dec.base_url ? ` · ${dec.base_url}` : ''}
                </p>
              </>
            ) : null}
            <p className="settings-muted">
              {RUNTIME.toolMatchDecideNeedEnable}{' '}
              <Link to="/settings/runtime">{RUNTIME.toolMatchDecideOpenRuntime}</Link>
            </p>
          </div>
        ) : null}

        {!readOnly && (decideOn || decideBusy) ? (
          <div className="settings-actions tool-matching-actions">
            <Button
              type="button"
              variant="secondary"
              data-testid="decide-systemone-disable"
              onClick={() => void onDisableDecide()}
            >
              {decideBusy ? RUNTIME.toolMatchCancel : RUNTIME.toolMatchDecideDisable}
            </Button>
          </div>
        ) : null}
      </section>

      {!readOnly && !decideOn ? (
        <>
          <section className="settings-form tool-matching-hero" data-testid="decide-systemone-local">
            <h2 className="settings-subheading">{RUNTIME.toolMatchDecideLocalTitle}</h2>
            <p className="settings-muted">{RUNTIME.toolMatchDecideLocalHint}</p>
            <div className="settings-actions tool-matching-actions">
              <Button
                type="button"
                variant="primary"
                disabled={anyDecideBusy || decLoading}
                data-testid="decide-systemone-enable"
                onClick={() => void onEnableDecideLocal()}
              >
                {decideEnableLabel(dec, decideBusy)}
              </Button>
            </div>
          </section>

          <form
            className="settings-form tool-matching-hero"
            data-testid="decide-systemone-api"
            onSubmit={(e) => void onEnableDecideAPI(e)}
          >
            <h2 className="settings-subheading">{RUNTIME.toolMatchDecideAPITitle}</h2>
            <p className="settings-muted">{RUNTIME.toolMatchDecideAPIHint}</p>
            <Field
              label={RUNTIME.fieldDecideSystemOneURL}
              hint={RUNTIME.toolMatchDecideAPIBaseHint}
            >
              <Input
                type="url"
                placeholder="http://127.0.0.1:11434"
                value={decApiBase}
                onChange={(e) => setDecApiBase(e.target.value)}
                disabled={anyDecideBusy}
                data-testid="decide-systemone-api-base"
              />
            </Field>
            <Field
              label={RUNTIME.fieldDecideSystemOneModel}
              hint={RUNTIME.toolMatchDecideAPIModelHint}
            >
              <Input
                value={decApiModel}
                onChange={(e) => setDecApiModel(e.target.value)}
                disabled={anyDecideBusy}
                placeholder="tev1"
                data-testid="decide-systemone-api-model"
              />
            </Field>
            <Field
              label={RUNTIME.fieldDecideSystemOneKey}
              hint={
                dec?.api_key_set
                  ? RUNTIME.toolMatchAPIKeyHintSet
                  : RUNTIME.toolMatchAPIKeyHint
              }
            >
              <Input
                type="password"
                autoComplete="off"
                placeholder={dec?.api_key_set ? '••••••••' : ''}
                value={decApiKey}
                onChange={(e) => setDecApiKey(e.target.value)}
                disabled={anyDecideBusy}
                data-testid="decide-systemone-api-key"
              />
            </Field>
            <div className="settings-actions tool-matching-actions">
              <Button
                type="submit"
                variant="primary"
                disabled={anyDecideBusy || decLoading}
                data-testid="decide-systemone-api-enable"
              >
                {decApiBusy
                  ? RUNTIME.toolMatchDecideModeBusy
                  : RUNTIME.toolMatchDecideAPIEnable}
              </Button>
            </div>
          </form>
        </>
      ) : null}

      {!readOnly && decideOn ? (
        <section className="settings-form tool-matching-hero" data-testid="decide-systemone-cleanup">
          <h2 className="settings-subheading">{RUNTIME.toolMatchDecideCleanupTitle}</h2>
          <p className="settings-muted">{RUNTIME.toolMatchDecideCleanupHint}</p>
          <div className="settings-actions tool-matching-actions">
            <Button
              type="button"
              variant="danger"
              disabled={anyDecideBusy}
              onClick={() => setDecCleanupOpen(true)}
            >
              {RUNTIME.toolMatchDecideCleanupButton}
            </Button>
          </div>
        </section>
      ) : null}

      {/* —— Advanced (paths + match cleanup) —— */}
      {!readOnly ? (
        <details className="tool-matching-advanced" data-testid="tool-matching-advanced">
          <summary className="settings-subheading">{RUNTIME.toolMatchAdvancedTitle}</summary>
          <p className="settings-muted">{RUNTIME.toolMatchAdvancedHint}</p>

          {!loading && paths ? (
            <section className="settings-form tool-matching-hero" data-testid="tool-matching-paths">
              <h2 className="settings-subheading">{RUNTIME.toolMatchPathsTitle}</h2>
              <p className="settings-muted">{RUNTIME.toolMatchPathsHint}</p>
              <dl className="tool-matching-paths">
                {paths.app_dir ? (
                  <div className="tool-matching-path-row">
                    <dt>{RUNTIME.toolMatchPathApp}</dt>
                    <dd data-testid="tool-matching-path-app">
                      <code>{paths.app_dir}</code>
                    </dd>
                  </div>
                ) : null}
                <div className="tool-matching-path-row">
                  <dt>
                    {RUNTIME.toolMatchPathModels}
                    <span className="settings-muted">
                      {' '}
                      · {pathSourceLabel(paths.models_dir_source)}
                    </span>
                  </dt>
                  <dd data-testid="tool-matching-path-models">
                    <code>{paths.models_dir}</code>
                  </dd>
                </div>
                {paths.config_dir ? (
                  <div className="tool-matching-path-row">
                    <dt>{RUNTIME.toolMatchPathConfig}</dt>
                    <dd>
                      <code>{paths.config_dir}</code>
                    </dd>
                  </div>
                ) : null}
                <div className="tool-matching-path-row">
                  <dt>{RUNTIME.toolMatchPathCache}</dt>
                  <dd>
                    <code>{paths.installer_cache_dir}</code>
                  </dd>
                </div>
              </dl>
              <Field label={RUNTIME.toolMatchPathModels}>
                <Input
                  value={modelsDirDraft}
                  onChange={(e) => setModelsDirDraft(e.target.value)}
                  disabled={anyMatchBusy}
                  placeholder={paths.models_dir}
                  data-testid="tool-matching-models-dir"
                />
              </Field>
              <div className="settings-actions tool-matching-actions">
                <Button
                  type="button"
                  variant="primary"
                  disabled={
                    anyMatchBusy ||
                    loading ||
                    modelsDirDraft.trim() === '' ||
                    modelsDirDraft.trim() === (paths.models_dir ?? '')
                  }
                  data-testid="tool-matching-models-dir-save"
                  onClick={() => void onSaveModelsDir(modelsDirDraft.trim())}
                >
                  {RUNTIME.toolMatchPathModelsSave}
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  disabled={anyMatchBusy || loading || paths.models_dir_source !== 'baize'}
                  data-testid="tool-matching-models-dir-clear"
                  onClick={() => void onSaveModelsDir('')}
                >
                  {RUNTIME.toolMatchPathModelsClear}
                </Button>
              </div>
            </section>
          ) : null}

          <section className="settings-form tool-matching-hero" data-testid="tool-matching-cleanup">
            <h2 className="settings-subheading">{RUNTIME.toolMatchCleanupTitle}</h2>
            <p className="settings-muted">{RUNTIME.toolMatchCleanupHint}</p>
            <label className="settings-checkbox">
              <input
                type="checkbox"
                checked={removeOllama}
                disabled={anyMatchBusy || loading}
                data-testid="tool-matching-cleanup-remove-app"
                onChange={(e) => setRemoveOllama(e.target.checked)}
              />
              <span>{RUNTIME.toolMatchCleanupRemoveApp}</span>
            </label>
            <div className="settings-actions tool-matching-actions">
              <Button
                type="button"
                variant="danger"
                disabled={anyMatchBusy || loading}
                data-testid="tool-matching-cleanup"
                onClick={() => setCleanupOpen(true)}
              >
                {RUNTIME.toolMatchCleanupButton}
              </Button>
            </div>
          </section>
        </details>
      ) : null}

      <ConfirmDialog
        open={cleanupOpen}
        title={RUNTIME.toolMatchCleanupConfirmTitle}
        body={
          removeOllama
            ? RUNTIME.toolMatchCleanupConfirmBodyWithApp
            : RUNTIME.toolMatchCleanupConfirmBody
        }
        confirmText={RUNTIME.toolMatchCleanupOk}
        danger
        busy={cleanupBusy}
        onConfirm={() => void onCleanupMatch()}
        onCancel={() => setCleanupOpen(false)}
      />

      <ConfirmDialog
        open={decCleanupOpen}
        title={RUNTIME.toolMatchDecideCleanupConfirmTitle}
        body={RUNTIME.toolMatchDecideCleanupConfirmBody}
        confirmText={RUNTIME.toolMatchDecideCleanupOk}
        danger
        busy={decCleanupBusy}
        onConfirm={() => void onCleanupDecide()}
        onCancel={() => setDecCleanupOpen(false)}
      />
    </div>
  )
}
