import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { FlaskConical } from 'lucide-react'
import {
  deleteEvalCase,
  exportEvalCase,
  ingestEvalCase,
  listEvalCases,
  patchEvalCase,
  queryEvalRuns,
  scoreEvalCase,
  type EvalCandidate,
  type EvalCase,
} from '../api/eval'
import { listEvents } from '../api/runs'
import type { Event } from '../api/types'
import {
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  Modal,
  PageHeader,
  Select,
  Textarea,
  ToastRegion,
  useToast,
} from '../components/ui'
import { EVAL, friendlyError } from '../strings'

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

function toDatetimeLocalValue(d: Date): string {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}T${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

function defaultTimeWindow(): { from: string; to: string } {
  const to = new Date()
  const from = new Date(to.getTime() - 7 * 24 * 60 * 60 * 1000)
  return { from: toDatetimeLocalValue(from), to: toDatetimeLocalValue(to) }
}

function localDatetimeToRFC3339(local: string): string {
  const d = new Date(local)
  if (Number.isNaN(d.getTime())) return ''
  return d.toISOString()
}

function linesFromText(raw: string): string[] {
  return raw
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}

function textFromLines(lines?: string[]): string {
  if (!lines?.length) return ''
  return lines.join('\n')
}

function formatDuration(ms?: number): string {
  if (ms == null) return '—'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(1)} s`
}

function scoreSummary(c: EvalCase): string {
  if (c.last_score_pass == null) return EVAL.scoreNone
  const codes = c.last_score_codes?.length ? c.last_score_codes.join(', ') : '—'
  return c.last_score_pass ? EVAL.scorePass(codes) : EVAL.scoreFail(codes)
}

type ExpectationModal =
  | { mode: 'ingest'; runId: string; inputPreview: string }
  | { mode: 'edit'; evalCase: EvalCase }

export function EvalSettings() {
  const { toasts, push, dismiss } = useToast()
  const [fromLocal, setFromLocal] = useState(() => defaultTimeWindow().from)
  const [toLocal, setToLocal] = useState(() => defaultTimeWindow().to)
  const [status, setStatus] = useState('')
  const [minDurationMs, setMinDurationMs] = useState('')
  const [eventType, setEventType] = useState('')
  const [candidates, setCandidates] = useState<EvalCandidate[] | null>(null)
  const [cases, setCases] = useState<EvalCase[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [expectationModal, setExpectationModal] = useState<ExpectationModal | null>(null)
  const [protocol, setProtocol] = useState('V1')
  const [requiredReads, setRequiredReads] = useState('')
  const [consequential, setConsequential] = useState('')
  const [pendingDelete, setPendingDelete] = useState<EvalCase | null>(null)
  const [eventsModal, setEventsModal] = useState<{ runId: string; events: Event[] | null } | null>(
    null,
  )

  const refreshCases = useCallback(async () => {
    try {
      const body = await listEvalCases()
      setCases(body.items)
    } catch (e) {
      setCases(null)
      const f = friendlyError(e)
      push({ tone: 'error', title: EVAL.loadCasesFailed, detail: f.detail ?? f.title })
    }
  }, [push])

  useEffect(() => {
    void refreshCases()
  }, [refreshCases])

  const runQuery = async (e?: FormEvent) => {
    e?.preventDefault()
    const from = localDatetimeToRFC3339(fromLocal)
    const to = localDatetimeToRFC3339(toLocal)
    if (!from || !to) {
      push({ tone: 'error', title: EVAL.needTimeWindow })
      return
    }
    setBusy(true)
    try {
      const minRaw = minDurationMs.trim()
      let minMs: number | undefined
      if (minRaw) {
        const n = Number(minRaw)
        if (!Number.isFinite(n) || n < 0) {
          push({ tone: 'error', title: EVAL.badMinDuration })
          return
        }
        minMs = n
      }
      const body = await queryEvalRuns({
        from,
        to,
        status: status.trim() || undefined,
        min_duration_ms: minMs,
        event_type: eventType.trim() || undefined,
      })
      setCandidates(body.items)
    } catch (err) {
      setCandidates(null)
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const openIngest = (c: EvalCandidate) => {
    setProtocol('V1')
    setRequiredReads('')
    setConsequential('')
    setExpectationModal({ mode: 'ingest', runId: c.id, inputPreview: c.input_preview })
  }

  const openEditDraft = (c: EvalCase) => {
    setProtocol((c.protocol || 'V1').trim())
    setRequiredReads(textFromLines(c.required_reads))
    setConsequential(textFromLines(c.consequential))
    setExpectationModal({ mode: 'edit', evalCase: c })
  }

  const submitExpectations = async () => {
    if (!expectationModal) return
    const proto = protocol.trim()
    if (!proto) {
      push({ tone: 'error', title: EVAL.needProtocol })
      return
    }
    setBusy(true)
    try {
      if (expectationModal.mode === 'ingest') {
        const body: Parameters<typeof ingestEvalCase>[0] = {
          run_id: expectationModal.runId,
          protocol: proto,
        }
        if (requiredReads.trim()) {
          body.required_reads = linesFromText(requiredReads)
        }
        if (consequential.trim()) {
          body.consequential = linesFromText(consequential)
        }
        await ingestEvalCase(body)
        push({ tone: 'success', title: EVAL.toastIngested })
      } else {
        await patchEvalCase(expectationModal.evalCase.id, {
          protocol: proto,
          required_reads: linesFromText(requiredReads),
          consequential: linesFromText(consequential),
        })
        push({ tone: 'success', title: EVAL.toastExpectationsSaved })
      }
      setExpectationModal(null)
      await refreshCases()
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const viewRunEvents = async (runId: string) => {
    setEventsModal({ runId, events: null })
    setBusy(true)
    try {
      const events = await listEvents(runId)
      setEventsModal({ runId, events })
    } catch (err) {
      setEventsModal(null)
      const f = friendlyError(err)
      push({ tone: 'error', title: EVAL.loadEventsFailed, detail: f.detail ?? f.title })
    } finally {
      setBusy(false)
    }
  }

  const rescore = async (c: EvalCase) => {
    setBusy(true)
    try {
      const res = await scoreEvalCase(c.id)
      await refreshCases()
      push({
        tone: res.pass ? 'success' : 'error',
        title: res.pass ? EVAL.toastScorePass : EVAL.toastScoreFail,
        detail: res.codes?.join(', ') ?? res.notes ?? '',
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const doExport = async (c: EvalCase) => {
    setBusy(true)
    try {
      const json = await exportEvalCase(c.id)
      const blob = new Blob([json], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `eval-case-${c.id}.json`
      a.click()
      URL.revokeObjectURL(url)
      push({ tone: 'success', title: EVAL.toastExported })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const confirmDeleteCase = async () => {
    if (!pendingDelete) return
    setBusy(true)
    try {
      await deleteEvalCase(pendingDelete.id)
      setPendingDelete(null)
      await refreshCases()
      push({ tone: 'success', title: EVAL.toastDeleted })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-panel">
      <PageHeader title={EVAL.title} description={EVAL.description} />
      <p className="settings-muted">{EVAL.harnessHint}</p>

      <h2 className="settings-subheading">{EVAL.candidatesTitle}</h2>
      <form className="settings-form" onSubmit={(e) => void runQuery(e)}>
        <Field label={EVAL.fromLabel}>
          <Input
            type="datetime-local"
            value={fromLocal}
            onChange={(e) => setFromLocal(e.target.value)}
            disabled={busy}
            data-testid="eval-from"
          />
        </Field>
        <Field label={EVAL.toLabel}>
          <Input
            type="datetime-local"
            value={toLocal}
            onChange={(e) => setToLocal(e.target.value)}
            disabled={busy}
            data-testid="eval-to"
          />
        </Field>
        <Field label={EVAL.statusLabel} hint={EVAL.statusHint}>
          <Input
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            disabled={busy}
            placeholder="failed"
          />
        </Field>
        <Field label={EVAL.minDurationLabel}>
          <Input
            value={minDurationMs}
            onChange={(e) => setMinDurationMs(e.target.value)}
            disabled={busy}
            inputMode="numeric"
            placeholder="60000"
          />
        </Field>
        <Field label={EVAL.eventTypeLabel}>
          <Input
            value={eventType}
            onChange={(e) => setEventType(e.target.value)}
            disabled={busy}
          />
        </Field>
        <Button type="submit" disabled={busy} data-testid="eval-query">
          {busy ? EVAL.querying : EVAL.query}
        </Button>
      </form>

      <div className="settings-table-wrap" data-testid="eval-candidates">
        {candidates === null ? (
          <p className="settings-muted">{EVAL.candidatesIdle}</p>
        ) : candidates.length === 0 ? (
          <EmptyState
            icon={<FlaskConical size={28} aria-hidden="true" />}
            title={EVAL.candidatesEmpty}
          />
        ) : (
          <table className="settings-table">
            <thead>
              <tr>
                <th>{EVAL.colRunId}</th>
                <th>{EVAL.colStatus}</th>
                <th>{EVAL.colDuration}</th>
                <th>{EVAL.colInput}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {candidates.map((c) => (
                <tr key={c.id}>
                  <td>
                    <code className="settings-code-sample">{c.id}</code>
                  </td>
                  <td>{c.status}</td>
                  <td>{formatDuration(c.duration_ms)}</td>
                  <td className="settings-truncate">{c.input_preview}</td>
                  <td>
                    <div className="accounts-actions">
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        disabled={busy}
                        data-testid={`eval-view-events-${c.id}`}
                        onClick={() => void viewRunEvents(c.id)}
                      >
                        {EVAL.viewEvents}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        disabled={busy}
                        onClick={() => openIngest(c)}
                      >
                        {EVAL.addCorpus}
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <h2 className="settings-subheading">{EVAL.casesTitle}</h2>
      <div data-testid="eval-cases">
        {cases === null ? (
          <p className="settings-muted">{EVAL.casesLoading}</p>
        ) : cases.length === 0 ? (
          <EmptyState
            icon={<FlaskConical size={28} aria-hidden="true" />}
            title={EVAL.casesEmpty}
            description={EVAL.casesEmptyDesc}
          />
        ) : (
          <ul className="settings-list">
            {cases.map((c) => (
              <li key={c.id} className="settings-list-item">
                <div className="settings-tool-row">
                  <div>
                    <p className="settings-tool-name">{c.input || c.source_run_id}</p>
                    <p className="settings-muted">
                      {EVAL.caseStatus(c.status)}
                      {c.protocol ? ` · ${c.protocol}` : ''}
                      {' · '}
                      {scoreSummary(c)}
                    </p>
                    {c.last_score_notes ? (
                      <p className="settings-muted">{c.last_score_notes}</p>
                    ) : null}
                  </div>
                  <div className="accounts-actions">
                    {c.status === 'draft' && (
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        disabled={busy}
                        onClick={() => openEditDraft(c)}
                      >
                        {EVAL.editExpectations}
                      </Button>
                    )}
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={busy || c.status !== 'confirmed'}
                      onClick={() => void rescore(c)}
                    >
                      {EVAL.rescore}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={busy}
                      onClick={() => void doExport(c)}
                    >
                      {EVAL.export}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="danger"
                      disabled={busy}
                      onClick={() => setPendingDelete(c)}
                    >
                      {EVAL.delete}
                    </Button>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      <Modal
        open={expectationModal != null}
        title={expectationModal?.mode === 'edit' ? EVAL.editExpectationsTitle : EVAL.ingestTitle}
        onClose={() => setExpectationModal(null)}
        footer={
          <>
            <Button
              type="button"
              variant="secondary"
              onClick={() => setExpectationModal(null)}
              disabled={busy}
            >
              {EVAL.cancel}
            </Button>
            <Button type="button" onClick={() => void submitExpectations()} disabled={busy}>
              {EVAL.ingestSubmit}
            </Button>
          </>
        }
      >
        {expectationModal ? (
          <>
            {expectationModal.mode === 'ingest' ? (
              <>
                <p className="settings-muted">{EVAL.ingestRun(expectationModal.runId)}</p>
                <p className="settings-truncate">{expectationModal.inputPreview}</p>
              </>
            ) : (
              <p className="settings-muted">{EVAL.editExpectationsBody(expectationModal.evalCase.id)}</p>
            )}
            <Field label={EVAL.protocolLabel}>
              <Select value={protocol} onChange={(e) => setProtocol(e.target.value)} disabled={busy}>
                <option value="V0">V0</option>
                <option value="V1">V1</option>
                <option value="V2">V2</option>
              </Select>
            </Field>
            <Field label={EVAL.requiredReadsLabel} hint={EVAL.listHint}>
              <Textarea
                value={requiredReads}
                onChange={(e) => setRequiredReads(e.target.value)}
                disabled={busy}
                rows={3}
              />
            </Field>
            <Field label={EVAL.consequentialLabel} hint={EVAL.listHint}>
              <Textarea
                value={consequential}
                onChange={(e) => setConsequential(e.target.value)}
                disabled={busy}
                rows={3}
              />
            </Field>
            <p className="settings-muted">
              {expectationModal.mode === 'ingest' ? EVAL.ingestDraftHint : EVAL.editDraftHint}
            </p>
          </>
        ) : null}
      </Modal>

      <Modal
        open={eventsModal != null}
        title={eventsModal ? EVAL.eventsTitle(eventsModal.runId) : EVAL.viewEvents}
        onClose={() => setEventsModal(null)}
        footer={
          <Button type="button" variant="secondary" onClick={() => setEventsModal(null)}>
            {EVAL.cancel}
          </Button>
        }
      >
        {eventsModal?.events == null ? (
          <p className="settings-muted">{EVAL.eventsLoading}</p>
        ) : eventsModal.events.length === 0 ? (
          <p className="settings-muted">{EVAL.eventsEmpty}</p>
        ) : (
          <pre className="settings-code-sample" style={{ maxHeight: '24rem', overflow: 'auto' }}>
            {JSON.stringify(eventsModal.events, null, 2)}
          </pre>
        )}
      </Modal>

      <ConfirmDialog
        open={pendingDelete != null}
        title={EVAL.deleteTitle}
        body={EVAL.deleteBody}
        confirmText={EVAL.delete}
        cancelText={EVAL.cancel}
        danger
        busy={busy}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => void confirmDeleteCase()}
      />

      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </div>
  )
}
