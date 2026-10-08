import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  cleanupToolRetrieval,
  disableToolRetrieval,
  enableToolRetrieval,
  enableToolRetrievalAPI,
  getToolRetrieval,
  setToolRetrievalModelsDir,
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
  if (id === 'cn') return RUNTIME.toolMatchMirrorCN
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
  return code
}

function modeTitle(st: ToolRetrievalStatus | null, busy: boolean, enhanced: boolean): string {
  if (enhanced) return RUNTIME.toolMatchModeEnhanced
  if (!st) return RUNTIME.toolMatchModeStandard
  if (busy) return RUNTIME.toolMatchModeBusy
  if (st.phase === 'failed') return RUNTIME.toolMatchModeFailed
  return RUNTIME.toolMatchModeStandard
}

function providerLabel(st: ToolRetrievalStatus | null): string {
  if (!st || st.mode !== 'enhanced') return ''
  return st.provider === 'api' ? RUNTIME.toolMatchProviderAPI : RUNTIME.toolMatchProviderLocal
}

function localEnableLabel(st: ToolRetrievalStatus | null, busy: boolean): string {
  if (busy) return RUNTIME.toolMatchModeBusy
  if (st?.phase === 'failed' && st.provider !== 'api') return RUNTIME.toolMatchRetry
  if (st?.ollama_running && st.model_present) return RUNTIME.toolMatchEnableReady
  if (st?.ollama_running && !st.model_present) return RUNTIME.toolMatchPullModel
  if (st?.ollama_installed && !st.ollama_running) {
    return st.model_present ? RUNTIME.toolMatchStartAndEnable : RUNTIME.toolMatchStartAndPull
  }
  return RUNTIME.toolMatchEnable
}

function localEnableToastTitle(st: ToolRetrievalStatus): string {
  if (st.ollama_running && st.model_present) return RUNTIME.toolMatchEnableReady
  if (st.ollama_installed && !st.ollama_running) {
    return st.model_present ? RUNTIME.toolMatchStartAndEnable : RUNTIME.toolMatchStartAndPull
  }
  if (st.ollama_running && !st.model_present) return RUNTIME.toolMatchPullModel
  return RUNTIME.toolMatchToastStarted
}

function pathSourceLabel(source?: string): string {
  if (source === 'baize') return RUNTIME.toolMatchPathSourceBaize
  if (source === 'env') return RUNTIME.toolMatchPathSourceEnv
  return RUNTIME.toolMatchPathSourceDefault
}

function syncModelsDraft(st: ToolRetrievalStatus | null): string {
  return st?.paths?.models_dir ?? ''
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

  const applyStatus = useCallback((next: ToolRetrievalStatus, syncModelsPath = false) => {
    setSt(next)
    if (syncModelsPath) {
      setModelsDirDraft(syncModelsDraft(next))
    }
    if (next.provider === 'api' && next.base_url) {
      setApiBase(next.base_url)
    }
    if (next.provider === 'api' && next.model) {
      setApiModel(next.model)
    }
  }, [])

  const load = useCallback(async () => {
    setLoadError(null)
    try {
      applyStatus(await getToolRetrieval(), true)
    } catch (err) {
      const f = friendlyError(err)
      setLoadError(f.detail ?? f.title)
      setSt(null)
    } finally {
      setLoading(false)
    }
  }, [applyStatus])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!st?.busy && !BUSY_PHASES.includes(st?.phase ?? 'idle')) return
    const id = window.setInterval(() => {
      void getToolRetrieval()
        .then((next) => {
          applyStatus(next, false)
          setLoadError(null)
        })
        .catch(() => {
          /* keep last */
        })
    }, 1500)
    return () => window.clearInterval(id)
  }, [st?.busy, st?.phase, applyStatus])

  const onEnableLocal = async () => {
    try {
      const next = await enableToolRetrieval()
      applyStatus(next, true)
      setLoadError(null)
      push({
        tone: 'info',
        title: localEnableToastTitle(next),
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }

  const onEnableAPI = async (e: FormEvent) => {
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
      applyStatus(next, true)
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

  const busy = !!st && (st.busy || BUSY_PHASES.includes(st?.phase ?? 'idle'))
  const enhanced = !!st && st.mode === 'enhanced' && st.phase === 'ready'
  const anyBusy = busy || apiBusy || cleanupBusy || pathsBusy
  const showProgress =
    !!st?.download &&
    (st.phase === 'downloading_installer' || st.phase === 'pulling_model')
  const paths = st?.paths

  const onDisable = async () => {
    const wasBusy = busy
    try {
      applyStatus(await disableToolRetrieval(), true)
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
      applyStatus(await setToolRetrievalModelsDir(dir), true)
      push({ tone: 'success', title: RUNTIME.toolMatchToastPathsSaved })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setPathsBusy(false)
    }
  }

  const onCleanup = async () => {
    setCleanupBusy(true)
    try {
      const res = await cleanupToolRetrieval({ remove_ollama: removeOllama })
      applyStatus(res.status, true)
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

  return (
    <div className="settings-section">
      <PageHeader title={RUNTIME.sectionToolMatch} description={RUNTIME.toolMatchHint} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      <section className="settings-form tool-matching-hero" data-testid="tool-matching-panel">
        {loading ? <p className="settings-muted">{RUNTIME.loading}</p> : null}

        {!loading ? (
          <div className="tool-matching-status">
            <p className="tool-matching-status-title">
              <strong>{modeTitle(st, busy, enhanced)}</strong>
              {providerLabel(st) ? (
                <span className="settings-muted"> · {providerLabel(st)}</span>
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
                {st.download?.note && st.phase === 'downloading_installer' ? (
                  <p className="settings-muted" data-testid="tool-matching-download-note">
                    {st.download.note}
                  </p>
                ) : null}
                {showProgress ? (
                  <div className="tool-matching-progress" data-testid="tool-matching-progress">
                    <div className="tool-matching-progress-meta">
                      <span>{mirrorLabel(st.download!.mirror)}</span>
                      <span>
                        {st.download!.total > 0
                          ? RUNTIME.toolMatchDownloadProgress(
                              formatBytes(st.download!.bytes),
                              formatBytes(st.download!.total),
                              st.download!.percent,
                            )
                          : RUNTIME.toolMatchDownloadProgressUnknown(
                              formatBytes(st.download!.bytes),
                            )}
                      </span>
                    </div>
                    <div
                      className="tool-matching-progress-track"
                      role="progressbar"
                      aria-valuemin={0}
                      aria-valuemax={100}
                      aria-valuenow={st.download!.total > 0 ? st.download!.percent : undefined}
                    >
                      <div
                        className="tool-matching-progress-fill"
                        style={{
                          width:
                            st.download!.total > 0
                              ? `${Math.min(100, Math.max(0, st.download!.percent))}%`
                              : '30%',
                          opacity: st.download!.total > 0 ? 1 : 0.45,
                        }}
                      />
                    </div>
                  </div>
                ) : null}
                {st.error ? <p className="settings-muted">{errorLabel(st.error)}</p> : null}
                <p className="settings-muted">
                  {RUNTIME.toolMatchModel}: {st.model || '—'}
                  {st.base_url ? ` · ${st.base_url}` : ''}
                </p>
                {(st.installer_cache_bytes ?? 0) > 0 ? (
                  <p className="settings-muted">
                    {RUNTIME.toolMatchCacheSize(formatBytes(st.installer_cache_bytes ?? 0))}
                  </p>
                ) : null}
              </>
            ) : null}
            {loadError ? (
              <p className="settings-muted" data-testid="tool-matching-load-error">
                {loadError}
              </p>
            ) : null}
          </div>
        ) : null}

        {!readOnly && (enhanced || busy) ? (
          <div className="settings-actions tool-matching-actions">
            <Button
              type="button"
              variant="secondary"
              data-testid="tool-matching-disable"
              onClick={() => void onDisable()}
            >
              {busy ? RUNTIME.toolMatchCancel : RUNTIME.toolMatchDisable}
            </Button>
          </div>
        ) : null}

        {readOnly ? <p className="settings-muted">{RUNTIME.descriptionOperator}</p> : null}
      </section>

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
                <span className="settings-muted"> · {pathSourceLabel(paths.models_dir_source)}</span>
              </dt>
              <dd data-testid="tool-matching-path-models">
                <code>{paths.models_dir}</code>
              </dd>
            </div>
            {paths.config_dir ? (
              <div className="tool-matching-path-row">
                <dt>{RUNTIME.toolMatchPathConfig}</dt>
                <dd data-testid="tool-matching-path-config">
                  <code>{paths.config_dir}</code>
                </dd>
              </div>
            ) : null}
            <div className="tool-matching-path-row">
              <dt>{RUNTIME.toolMatchPathCache}</dt>
              <dd data-testid="tool-matching-path-cache">
                <code>{paths.installer_cache_dir}</code>
              </dd>
            </div>
          </dl>
          {!readOnly ? (
            <>
              <Field label={RUNTIME.toolMatchPathModels}>
                <Input
                  value={modelsDirDraft}
                  onChange={(e) => setModelsDirDraft(e.target.value)}
                  disabled={anyBusy}
                  placeholder={paths.models_dir}
                  data-testid="tool-matching-models-dir"
                />
              </Field>
              <div className="settings-actions tool-matching-actions">
                <Button
                  type="button"
                  variant="primary"
                  disabled={
                    anyBusy ||
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
                  disabled={anyBusy || loading || paths.models_dir_source !== 'baize'}
                  data-testid="tool-matching-models-dir-clear"
                  onClick={() => void onSaveModelsDir('')}
                >
                  {RUNTIME.toolMatchPathModelsClear}
                </Button>
              </div>
            </>
          ) : null}
        </section>
      ) : null}

      {!readOnly && !enhanced ? (
        <>
          <section className="settings-form tool-matching-hero" data-testid="tool-matching-local">
            <h2 className="settings-subheading">{RUNTIME.toolMatchLocalTitle}</h2>
            <p className="settings-muted">{RUNTIME.toolMatchLocalHint}</p>
            <div className="settings-actions tool-matching-actions">
              <Button
                type="button"
                variant="primary"
                disabled={anyBusy || loading}
                data-testid="tool-matching-enable"
                onClick={() => void onEnableLocal()}
              >
                {localEnableLabel(st, busy)}
              </Button>
            </div>
          </section>

          <form
            className="settings-form tool-matching-hero"
            data-testid="tool-matching-api"
            onSubmit={(e) => void onEnableAPI(e)}
          >
            <h2 className="settings-subheading">{RUNTIME.toolMatchAPITitle}</h2>
            <p className="settings-muted">{RUNTIME.toolMatchAPIHint}</p>
            <Field label={RUNTIME.toolMatchAPIBase} hint={RUNTIME.toolMatchAPIBaseHint}>
              <Input
                type="url"
                placeholder="https://api.openai.com/v1"
                value={apiBase}
                onChange={(e) => setApiBase(e.target.value)}
                disabled={anyBusy}
                data-testid="tool-matching-api-base"
              />
            </Field>
            <Field label={RUNTIME.toolMatchAPIModel} hint={RUNTIME.toolMatchAPIModelHint}>
              <Input
                value={apiModel}
                onChange={(e) => setApiModel(e.target.value)}
                disabled={anyBusy}
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
                disabled={anyBusy}
                data-testid="tool-matching-api-key"
              />
            </Field>
            <div className="settings-actions tool-matching-actions">
              <Button
                type="submit"
                variant="primary"
                disabled={anyBusy || loading}
                data-testid="tool-matching-api-enable"
              >
                {apiBusy ? RUNTIME.toolMatchModeBusy : RUNTIME.toolMatchAPIEnable}
              </Button>
            </div>
          </form>
        </>
      ) : null}

      {!readOnly ? (
        <section className="settings-form tool-matching-hero" data-testid="tool-matching-cleanup">
          <h2 className="settings-subheading">{RUNTIME.toolMatchCleanupTitle}</h2>
          <p className="settings-muted">{RUNTIME.toolMatchCleanupHint}</p>
          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={removeOllama}
              disabled={anyBusy || loading}
              data-testid="tool-matching-cleanup-remove-app"
              onChange={(e) => setRemoveOllama(e.target.checked)}
            />
            <span>{RUNTIME.toolMatchCleanupRemoveApp}</span>
          </label>
          <div className="settings-actions tool-matching-actions">
            <Button
              type="button"
              variant="danger"
              disabled={anyBusy || loading}
              data-testid="tool-matching-cleanup"
              onClick={() => setCleanupOpen(true)}
            >
              {RUNTIME.toolMatchCleanupButton}
            </Button>
          </div>
        </section>
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
        onConfirm={() => void onCleanup()}
        onCancel={() => setCleanupOpen(false)}
      />
    </div>
  )
}
