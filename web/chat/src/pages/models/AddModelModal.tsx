import { useEffect, useState } from 'react'
import {
  batchImportModels,
  createModelProfile,
  discoverModels,
  type BatchImportResult,
  type ThinkingDialect,
  type ThinkingLevel,
  type UpstreamModel,
} from '../../api'
import { Button, Field, Input, Modal, Select, Spinner } from '../../components/ui'
import { MODELS } from '../../strings'
import {
  tierOptions,
  thinkingDialectOptions,
  thinkingLevelOptions,
} from './modelSettingsHelpers'

export interface AddModelModalProps {
  open: boolean
  onClose: () => void
  onImported: () => void | Promise<void>
}

type Phase = 'discover' | 'list' | 'result'

export function AddModelModal({ open, onClose, onImported }: AddModelModalProps) {
  // Endpoint + credential
  const [baseUrl, setBaseUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [fetching, setFetching] = useState(false)

  // Catalog + selection
  const [phase, setPhase] = useState<Phase>('discover')
  const [models, setModels] = useState<UpstreamModel[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [error, setError] = useState<string | null>(null)
  const [importing, setImporting] = useState(false)
  const [result, setResult] = useState<BatchImportResult | null>(null)

  // Manual single-model fallback
  const [manual, setManual] = useState(false)
  const [savingManual, setSavingManual] = useState(false)
  const [manualForm, setManualForm] = useState({
    name: '',
    model: '',
    tier: 'auto',
    thinkingLevel: 'medium' as ThinkingLevel,
    thinkingDialect: 'auto' as ThinkingDialect,
    supportsVision: false,
    contextTokens: 128000,
    apiKeyEnv: '',
  })

  // Reset everything whenever the dialog is (re)opened.
  useEffect(() => {
    if (open) {
      setBaseUrl('')
      setApiKey('')
      setFetching(false)
      setPhase('discover')
      setModels([])
      setSelected(new Set())
      setError(null)
      setImporting(false)
      setResult(null)
      setManual(false)
      setSavingManual(false)
      setManualForm({
        name: '', model: '', tier: 'auto',
        thinkingLevel: 'medium', thinkingDialect: 'auto',
        supportsVision: false, contextTokens: 128000, apiKeyEnv: '',
      })
    }
  }, [open])

  const endpointReady = baseUrl.trim() !== ''

  const runDiscover = async () => {
    if (!endpointReady) return
    setFetching(true)
    setError(null)
    try {
      const list = await discoverModels({
        base_url: baseUrl.trim(),
        ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
      })
      setModels(list)
      setSelected(new Set(list.map((m) => m.id)))
      setPhase('list')
    } catch (err) {
      const detail = err instanceof Error ? err.message : String(err)
      setError(detail)
    } finally {
      setFetching(false)
    }
  }

  const allSelected = models.length > 0 && selected.size === models.length
  const toggleAll = () =>
    setSelected(allSelected ? new Set() : new Set(models.map((m) => m.id)))
  const toggleOne = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const runImport = async () => {
    if (selected.size === 0) return
    setImporting(true)
    try {
      const res = await batchImportModels({
        base_url: baseUrl.trim(),
        ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
        models: models.filter((m) => selected.has(m.id)).map((m) => ({ id: m.id })),
        thinking_level: 'medium',
        thinking_dialect: 'auto',
        context_tokens: 128000,
        supports_vision: false,
      })
      setResult(res)
      setPhase('result')
    } finally {
      setImporting(false)
    }
  }

  const saveManual = async () => {
    const model = manualForm.model.trim()
    if (!endpointReady) {
      setError(MODELS.batchNeedEndpoint)
      return
    }
    if (!model) {
      setError(MODELS.errModelRequired)
      return
    }
    setSavingManual(true)
    setError(null)
    try {
      await createModelProfile({
        name: manualForm.name.trim() || model,
        base_url: baseUrl.trim(),
        model,
        ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
        ...(manualForm.apiKeyEnv.trim() ? { api_key_env: manualForm.apiKeyEnv.trim() } : {}),
        auto_tier: manualForm.tier as 'auto' | 'light' | 'standard' | 'power',
        thinking_level: manualForm.thinkingLevel,
        thinking_dialect: manualForm.thinkingDialect,
        supports_vision: manualForm.supportsVision,
        context_tokens: manualForm.contextTokens,
      })
      await onImported()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSavingManual(false)
    }
  }

  const finish = async () => {
    await onImported()
    onClose()
  }

  const busy = fetching || importing || savingManual

  const footer = (
    phase === 'result' ? (
      <Button variant="primary" onClick={() => void finish()}>
        {MODELS.batchDone}
      </Button>
    ) : manual ? (
      <>
        <Button variant="ghost" disabled={busy} onClick={() => setManual(false)}>
          {MODELS.batchBack}
        </Button>
        <Button variant="primary" disabled={busy} onClick={() => void saveManual()}>
          {savingManual ? <Spinner /> : MODELS.save}
        </Button>
      </>
    ) : phase === 'list' ? (
      <>
        <Button variant="ghost" disabled={busy} onClick={() => setPhase('discover')}>
          {MODELS.batchBack}
        </Button>
        <Button
          variant="primary"
          disabled={busy || selected.size === 0}
          onClick={() => void runImport()}
        >
          {importing ? <Spinner /> : MODELS.batchImport(selected.size)}
        </Button>
      </>
    ) : (
      <>
        <Button variant="ghost" disabled={busy} onClick={onClose}>
          {MODELS.cancel}
        </Button>
        <Button
          variant="primary"
          disabled={busy || !endpointReady}
          onClick={() => void runDiscover()}
        >
          {fetching ? <Spinner /> : MODELS.batchFetch}
        </Button>
      </>
    )
  )

  return (
    <Modal
      open={open}
      title={MODELS.add}
      onClose={busy ? undefined : onClose}
      footer={footer}
    >
      <div className="add-model-wizard" data-testid="add-model-wizard">
        <p className="settings-muted">{MODELS.wizardIntro}</p>

        <Field label={MODELS.fieldBaseUrl} required>
          <Input
            value={baseUrl}
            onChange={(e) => setBaseUrl(e.target.value)}
            disabled={busy}
            placeholder={MODELS.phBaseUrl}
          />
        </Field>

        {/* Key is optional up-front: local servers need none; a 401 asks for it. */}
        <Field label={MODELS.fieldApiKey} hint={MODELS.keyOptionalHint}>
          <Input
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            disabled={busy}
            autoComplete="off"
          />
        </Field>

        {error && (
          <p className="ui-inline-error" role="alert">
            {error}
          </p>
        )}

        {!manual && phase === 'discover' && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => setManual(true)}
          >
            {MODELS.manualEntry}
          </Button>
        )}

        {manual && (
          <div className="manual-model-form" data-testid="manual-model-form">
            <Field label={MODELS.fieldModel} required>
              <Input
                value={manualForm.model}
                onChange={(e) =>
                  setManualForm((f) => ({ ...f, model: e.target.value }))
                }
                disabled={busy}
                placeholder={MODELS.phModel}
              />
            </Field>
            <Field label={MODELS.fieldName} hint={MODELS.manualNameHint}>
              <Input
                value={manualForm.name}
                onChange={(e) =>
                  setManualForm((f) => ({ ...f, name: e.target.value }))
                }
                disabled={busy}
                placeholder={MODELS.phName}
              />
            </Field>
            <Field label={MODELS.fieldTier} hint={MODELS.hintTier}>
              <Select
                value={manualForm.tier}
                onChange={(e) =>
                  setManualForm((f) => ({ ...f, tier: e.target.value }))
                }
                disabled={busy}
              >
                {tierOptions().map((o) => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </Select>
            </Field>
            <Field label={MODELS.fieldThinkingLevel}>
              <Select
                value={manualForm.thinkingLevel}
                onChange={(e) =>
                  setManualForm((f) => ({
                    ...f,
                    thinkingLevel: e.target.value as ThinkingLevel,
                  }))
                }
                disabled={busy}
              >
                {thinkingLevelOptions().map((o) => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </Select>
            </Field>

            <details className="settings-advanced">
              <summary>{MODELS.advanced}</summary>
              <label className="ui-checkbox-row">
                <input
                  type="checkbox"
                  checked={manualForm.supportsVision}
                  onChange={(e) =>
                    setManualForm((f) => ({ ...f, supportsVision: e.target.checked }))
                  }
                  disabled={busy}
                />
                <span>{MODELS.fieldVision}</span>
              </label>
              <Field label={MODELS.fieldThinkingDialect}>
                <Select
                  value={manualForm.thinkingDialect}
                  onChange={(e) =>
                    setManualForm((f) => ({
                      ...f,
                      thinkingDialect: e.target.value as ThinkingDialect,
                    }))
                  }
                  disabled={busy}
                >
                  {thinkingDialectOptions().map((o) => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </Select>
              </Field>
              <Field label={MODELS.fieldContextTokens} hint={MODELS.hintContextTokens}>
                <Input
                  type="number" min={1024} step={1000}
                  value={manualForm.contextTokens}
                  onChange={(e) =>
                    setManualForm((f) => ({ ...f, contextTokens: Number(e.target.value) }))
                  }
                  disabled={busy}
                />
              </Field>
              <Field label={MODELS.fieldApiKeyEnv}>
                <Input
                  value={manualForm.apiKeyEnv}
                  onChange={(e) =>
                    setManualForm((f) => ({ ...f, apiKeyEnv: e.target.value }))
                  }
                  disabled={busy}
                  placeholder={MODELS.phApiKeyEnv}
                />
              </Field>
            </details>
          </div>
        )}

        {phase === 'list' && (
          <div className="batch-model-list" data-testid="batch-model-list">
            <label className="batch-select-all">
              <input type="checkbox" checked={allSelected} onChange={toggleAll} />
              <span>{MODELS.batchSelectAll}</span>
            </label>
            {models.map((m) => (
              <label key={m.id} className="batch-model-row">
                <input
                  type="checkbox"
                  checked={selected.has(m.id)}
                  onChange={() => toggleOne(m.id)}
                />
                <span className="batch-model-id">{m.id}</span>
                {m.owned_by ? (
                  <span className="batch-model-owner">{m.owned_by}</span>
                ) : null}
              </label>
            ))}
          </div>
        )}

        {phase === 'result' && result && (
          <div className="batch-result" data-testid="batch-result">
            <p className="batch-result-count">
              {MODELS.batchCreated(result.created.length)}
            </p>
            {result.skipped.length > 0 && (
              <>
                <p className="batch-result-count batch-skip-count">
                  {MODELS.batchSkipped(result.skipped.length)}
                </p>
                <ul className="batch-skip-list">
                  {result.skipped.map((s) => (
                    <li key={s.id}>{s.name || s.id} · {s.reason}</li>
                  ))}
                </ul>
              </>
            )}
          </div>
        )}
      </div>
    </Modal>
  )
}
