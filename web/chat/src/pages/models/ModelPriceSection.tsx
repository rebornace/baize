import { useEffect, useState } from 'react'
import { Button, Input, Modal } from '../../components/ui'
import { MODELS } from '../../strings'
import {
  getModelPrices,
  putModelPrices,
  type ModelPriceEntry,
  type ModelPriceTable,
} from '../../api/settings/models'

function parseExternal(raw: string): ModelPriceEntry[] | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== 'object') return null
  const obj = parsed as Record<string, unknown>
  const out: ModelPriceEntry[] = []
  if (Array.isArray(obj.entries)) {
    for (const item of obj.entries) {
      if (!item || typeof item !== 'object') continue
      const e = item as Record<string, unknown>
      if (typeof e.model !== 'string') continue
      out.push({
        model: e.model,
        input_per_mtok: Number(e.input_per_mtok ?? 0),
        output_per_mtok: Number(e.output_per_mtok ?? 0),
        cached_input_per_mtok: Number(e.cached_input_per_mtok ?? 0),
      })
    }
    return out
  }
  for (const [model, value] of Object.entries(obj)) {
    if (!value || typeof value !== 'object') continue
    const v = value as Record<string, unknown>
    const input = Number(v.input_cost_per_token)
    const output = Number(v.output_cost_per_token)
    if (!Number.isFinite(input) || !Number.isFinite(output)) continue
    const cached = Number(v.cache_read_input_token_cost)
    out.push({
      model,
      input_per_mtok: input * 1_000_000,
      output_per_mtok: output * 1_000_000,
      cached_input_per_mtok: Number.isFinite(cached) ? cached * 1_000_000 : 0,
    })
  }
  return out
}

export function ModelPriceSection({ readOnly }: { readOnly: boolean }) {
  const [table, setTable] = useState<ModelPriceTable | null>(null)
  const [defaults, setDefaults] = useState<ModelPriceTable | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [importText, setImportText] = useState('')

  useEffect(() => {
    let alive = true
    void (async () => {
      try {
        const body = await getModelPrices()
        if (!alive) return
        setTable({ currency: body.currency, source: body.source, entries: body.entries })
        setDefaults(body.defaults)
      } catch (e) {
        if (alive) setError(e instanceof Error ? e.message : String(e))
      }
    })()
    return () => {
      alive = false
    }
  }, [])

  const update = (index: number, patch: Partial<ModelPriceEntry>) => {
    setTable((prev) => {
      if (!prev) return prev
      const entries = [...prev.entries]
      entries[index] = { ...entries[index], ...patch }
      return { ...prev, entries }
    })
    setSaved(false)
  }

  const save = async () => {
    if (!table || readOnly) return
    if (!table.entries.length) {
      setError(MODELS.priceNeedEntry)
      return
    }
    setBusy(true)
    setError(null)
    try {
      const next = await putModelPrices({ currency: table.currency || 'USD', entries: table.entries })
      setTable(next)
      setSaved(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const reset = async () => {
    if (!defaults || readOnly) return
    setBusy(true)
    setError(null)
    try {
      const next = await putModelPrices({ currency: defaults.currency, entries: defaults.entries })
      setTable(next)
      setSaved(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const applyImport = () => {
    const entries = parseExternal(importText)
    if (!entries) {
      setError(MODELS.priceImportInvalid)
      return
    }
    if (!entries.length) {
      setError(MODELS.priceImportEmpty)
      return
    }
    setTable((prev) => ({ currency: prev?.currency || 'USD', entries }))
    setImportOpen(false)
    setImportText('')
    setError(null)
    setSaved(false)
  }

  return (
    <section className="settings-model-prices">
      <h2 className="settings-subheading">{MODELS.priceTitle}</h2>
      <p className="settings-muted">{MODELS.priceHint}</p>
      {error ? <p className="settings-error">{error}</p> : null}
      {saved ? <p className="settings-muted">{MODELS.priceSaved}</p> : null}
      {table ? (
        <>
          <div className="model-price-head">
            <span>{MODELS.priceModel}</span>
            <span>{MODELS.priceInput}</span>
            <span>{MODELS.priceOutput}</span>
            <span>{MODELS.priceCached}</span>
            <span />
          </div>
          {table.entries.map((entry, i) => (
            <div className="model-price-row" key={`${entry.model}-${i}`}>
              <Input
                value={entry.model}
                disabled={readOnly || busy}
                onChange={(e) => update(i, { model: e.target.value })}
              />
              <Input
                type="number"
                min={0}
                value={String(entry.input_per_mtok)}
                disabled={readOnly || busy}
                onChange={(e) => update(i, { input_per_mtok: Number(e.target.value) || 0 })}
              />
              <Input
                type="number"
                min={0}
                value={String(entry.output_per_mtok)}
                disabled={readOnly || busy}
                onChange={(e) => update(i, { output_per_mtok: Number(e.target.value) || 0 })}
              />
              <Input
                type="number"
                min={0}
                value={String(entry.cached_input_per_mtok)}
                disabled={readOnly || busy}
                onChange={(e) => update(i, { cached_input_per_mtok: Number(e.target.value) || 0 })}
              />
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={readOnly || busy}
                onClick={() =>
                  setTable((prev) =>
                    prev ? { ...prev, entries: prev.entries.filter((_, j) => j !== i) } : prev,
                  )
                }
              >
                {MODELS.delete}
              </Button>
            </div>
          ))}
          {!readOnly ? (
            <div className="settings-form-actions">
              <Button
                type="button"
                size="sm"
                variant="secondary"
                disabled={busy}
                onClick={() =>
                  setTable((prev) =>
                    prev
                      ? {
                          ...prev,
                          entries: [
                            ...prev.entries,
                            {
                              model: '',
                              input_per_mtok: 0,
                              output_per_mtok: 0,
                              cached_input_per_mtok: 0,
                            },
                          ],
                        }
                      : prev,
                  )
                }
              >
                {MODELS.priceAdd}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                disabled={busy}
                onClick={() => setImportOpen(true)}
              >
                {MODELS.priceImport}
              </Button>
              <Button type="button" size="sm" variant="secondary" disabled={busy} onClick={() => void reset()}>
                {MODELS.priceReset}
              </Button>
              <Button type="button" size="sm" variant="primary" disabled={busy} onClick={() => void save()}>
                {MODELS.priceSave}
              </Button>
            </div>
          ) : null}
        </>
      ) : null}

      <Modal
        open={importOpen}
        title={MODELS.priceImportTitle}
        onClose={() => setImportOpen(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setImportOpen(false)}>
              {MODELS.cancel}
            </Button>
            <Button variant="primary" onClick={applyImport}>
              {MODELS.priceImportApply}
            </Button>
          </>
        }
      >
        <p className="settings-muted">{MODELS.priceImportHint}</p>
        <textarea
          className="settings-input"
          rows={12}
          value={importText}
          onChange={(e) => setImportText(e.target.value)}
        />
      </Modal>
    </section>
  )
}
