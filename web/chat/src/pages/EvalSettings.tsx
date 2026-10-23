import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { FlaskConical } from 'lucide-react'
import {
  checkEvalCaseState,
  deleteEvalCase,
  exportEvalCase,
  getCalibrationSummary,
  ingestEvalCase,
  listEvalCases,
  patchEvalCase,
  putCalibration,
  queryEvalRuns,
  sampleEvalRuns,
  scoreEvalCase,
  fetchSkillDraft,
  installSkillDraft,
  polishSkillDraft,
  type EvalCalibrationSummary,
  type EvalCandidate,
  type EvalCase,
  type SkillDraftResponse,
} from '../api/eval'
import { SkillDraftModal } from '../components/SkillDraftModal'
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

/** Merge check-state API row; clear stale score codes when omitempty omits them on pass. */
function mergeCaseAfterCheckState(row: EvalCase, updated: EvalCase): EvalCase {
  const merged: EvalCase = { ...row, ...updated }
  if (updated.last_score_codes === undefined) {
    merged.last_score_codes = []
  }
  if (updated.last_score_notes === undefined) {
    merged.last_score_notes = ''
  }
  return merged
}

function isCalibratable(c: EvalCase): boolean {
  return (
    c.status === 'confirmed' &&
    c.last_scored_at != null &&
    c.last_scored_at !== '' &&
    c.last_score_pass != null
  )
}

function isCalibrationStale(c: EvalCase): boolean {
  if (c.human_agree == null || !c.calibrated_at || !c.last_scored_at) return false
  return Date.parse(c.calibrated_at) < Date.parse(c.last_scored_at)
}

function formatAgreementRate(rate: number | null): string {
  if (rate == null) return EVAL.calibrationSummaryRateNone
  const pct = `${(rate * 100).toFixed(1).replace(/\.0$/, '')}%`
  return EVAL.calibrationSummaryRate(pct)
}

function humanCalibrationLabel(c: EvalCase): string | null {
  if (c.human_agree == null) return null
  return c.human_agree ? EVAL.calibrationHumanAgree : EVAL.calibrationHumanDisagree
}

function compactDisplayText(raw: string, max = 160): string {
  const cleaned = raw.replace(/\s+/g, ' ').trim()
  if (cleaned.length <= max) return cleaned
  return `${cleaned.slice(0, Math.max(0, max - 1))}…`
}

function caseDisplayTitle(c: EvalCase): string {
  return compactDisplayText(c.input || c.source_run_id || '')
}

function candidateDisplayTitle(c: EvalCandidate): string {
  return compactDisplayText(c.input_preview || c.id)
}

function shortId(id: string): string {
  if (id.length <= 16) return id
  return `${id.slice(0, 8)}…${id.slice(-4)}`
}

function trajectorySummaryLines(c: EvalCase): string[] {
  const lines: string[] = []
  if (c.forbid_tools?.length) {
    lines.push(`${EVAL.trajectoryForbid}: ${JSON.stringify(c.forbid_tools)}`)
  }
  if (c.arg_contains?.length) {
    lines.push(`${EVAL.trajectoryArg}: ${JSON.stringify(c.arg_contains)}`)
  }
  if (c.expect_state?.length) {
    lines.push(`${EVAL.trajectoryExpect}: ${JSON.stringify(c.expect_state)}`)
  }
  return lines
}

type ExpectationModal =
  | { mode: 'ingest'; runId: string; inputPreview: string }
  | { mode: 'edit'; evalCase: EvalCase }

export function EvalSettings() {
  const { toasts, push, dismiss } = useToast()
  const [fromLocal, setFromLocal] = useState(() => defaultTimeWindow().from)
  const [toLocal, setToLocal] = useState(() => defaultTimeWindow().to)
  const [sampleWithScore, setSampleWithScore] = useState(false)
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
  const [calibrationSummary, setCalibrationSummary] = useState<EvalCalibrationSummary | null>(null)
  const [calibrationNotes, setCalibrationNotes] = useState<Record<string, string>>({})
  const [mockBaseUrls, setMockBaseUrls] = useState<Record<string, string>>({})
  const [skillDraftModal, setSkillDraftModal] = useState<{
    runId: string
    draft: SkillDraftResponse | null
  } | null>(null)
  const [skillDraftMarkdown, setSkillDraftMarkdown] = useState('')
  const [skillDraftEvalStub, setSkillDraftEvalStub] = useState(false)

  const refreshSummary = useCallback(async () => {
    try {
      const body = await getCalibrationSummary()
      setCalibrationSummary(body)
    } catch (e) {
      setCalibrationSummary(null)
      const f = friendlyError(e)
      push({ tone: 'error', title: EVAL.loadCasesFailed, detail: f.detail ?? f.title })
    }
  }, [push])

  const refreshCases = useCallback(async () => {
    try {
      const body = await listEvalCases()
      setCases(body.items)
      setCalibrationNotes((prev) => {
        const next = { ...prev }
        for (const c of body.items) {
          if (next[c.id] === undefined) {
            next[c.id] = c.human_note ?? ''
          }
        }
        return next
      })
    } catch (e) {
      setCases(null)
      const f = friendlyError(e)
      push({ tone: 'error', title: EVAL.loadCasesFailed, detail: f.detail ?? f.title })
    }
  }, [push])

  const refreshCasesAndSummary = useCallback(async () => {
    await Promise.all([refreshCases(), refreshSummary()])
  }, [refreshCases, refreshSummary])

  useEffect(() => {
    void refreshCasesAndSummary()
  }, [refreshCasesAndSummary])

  const submitCalibration = async (c: EvalCase, agree: boolean | null) => {
    setBusy(true)
    try {
      const note = calibrationNotes[c.id] ?? ''
      await putCalibration(c.id, { agree, note: note.trim() || undefined })
      await refreshCasesAndSummary()
      push({
        tone: 'success',
        title: agree == null ? EVAL.toastCalibrationCleared : EVAL.toastCalibrationSaved,
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const parseWindowFilters = (): {
    from: string
    to: string
    minMs?: number
  } | null => {
    const from = localDatetimeToRFC3339(fromLocal)
    const to = localDatetimeToRFC3339(toLocal)
    if (!from || !to) {
      push({ tone: 'error', title: EVAL.needTimeWindow })
      return null
    }
    const minRaw = minDurationMs.trim()
    let minMs: number | undefined
    if (minRaw) {
      const n = Number(minRaw)
      if (!Number.isFinite(n) || n < 0) {
        push({ tone: 'error', title: EVAL.badMinDuration })
        return null
      }
      minMs = n
    }
    return { from, to, minMs }
  }

  const runQuery = async (e?: FormEvent) => {
    e?.preventDefault()
    const win = parseWindowFilters()
    if (!win) return
    setBusy(true)
    try {
      const body = await queryEvalRuns({
        from: win.from,
        to: win.to,
        status: status.trim() || undefined,
        min_duration_ms: win.minMs,
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

  const runSample = async () => {
    const win = parseWindowFilters()
    if (!win) return
    setBusy(true)
    try {
      const res = await sampleEvalRuns({
        from: win.from,
        to: win.to,
        status: status.trim() || undefined,
        min_duration_ms: win.minMs,
        event_type: eventType.trim() || undefined,
        max_n: 10,
        mode: sampleWithScore ? 'score' : 'draft',
        ...(sampleWithScore
          ? { protocol: 'V0', required_reads: [] as string[], consequential: [] as string[] }
          : {}),
      })
      push({
        tone: 'success',
        title: EVAL.toastSampled(res.created, res.skipped, res.scored),
      })
      await refreshCasesAndSummary()
      const listed = await queryEvalRuns({
        from: win.from,
        to: win.to,
        status: status.trim() || undefined,
        min_duration_ms: win.minMs,
        event_type: eventType.trim() || undefined,
      })
      setCandidates(listed.items)
    } catch (err) {
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
      await refreshCasesAndSummary()
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const digSkillVariant = (c: EvalCandidate): 'primary' | 'secondary' => {
    if (c.status === 'failed') return 'primary'
    const hitl = eventType.trim().toLowerCase()
    if (hitl.includes('hitl')) return 'primary'
    return 'secondary'
  }

  const openSkillDraft = async (runId: string) => {
    setSkillDraftModal({ runId, draft: null })
    setSkillDraftMarkdown('')
    setSkillDraftEvalStub(false)
    setBusy(true)
    try {
      const draft = await fetchSkillDraft(runId)
      setSkillDraftModal({ runId, draft })
      setSkillDraftMarkdown(draft.markdown)
    } catch (err) {
      setSkillDraftModal(null)
      const f = friendlyError(err)
      const detail = f.detail ?? f.title
      push({
        tone: 'error',
        title: EVAL.digSkillEmpty,
        detail: detail === EVAL.digSkillEmpty ? undefined : detail,
      })
    } finally {
      setBusy(false)
    }
  }

  const polishSkillDraftMarkdown = async () => {
    if (!skillDraftModal?.draft) return
    const before = skillDraftMarkdown
    setBusy(true)
    try {
      const res = await polishSkillDraft(skillDraftModal.runId, {
        markdown: skillDraftMarkdown,
      })
      setSkillDraftMarkdown(res.markdown)
    } catch (err) {
      setSkillDraftMarkdown(before)
      const f = friendlyError(err)
      push({
        tone: 'error',
        title: EVAL.digSkillPolishFailed,
        detail: f.detail ?? f.title,
      })
    } finally {
      setBusy(false)
    }
  }

  const confirmInstallSkillDraft = async () => {
    if (!skillDraftModal?.draft) return
    const markdown = skillDraftMarkdown.trim()
    if (!markdown) {
      push({ tone: 'error', title: EVAL.digSkillEmpty })
      return
    }
    setBusy(true)
    try {
      const res = await installSkillDraft(skillDraftModal.runId, {
        markdown,
        create_eval_stub: skillDraftEvalStub || undefined,
      })
      setSkillDraftModal(null)
      push({ tone: 'success', title: EVAL.toastSkillInstalled(res.id) })
      if (skillDraftEvalStub) {
        if (res.eval_stub === 'created') {
          push({ tone: 'success', title: EVAL.digSkillEvalStubCreated })
        } else if (res.eval_stub === 'skipped_exists') {
          push({ tone: 'info', title: EVAL.digSkillEvalStubSkipped })
        } else if (res.eval_stub === 'error') {
          push({ tone: 'error', title: EVAL.digSkillEvalStubError })
        }
      }
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

  const runCheckState = async (c: EvalCase) => {
    const mockBaseUrl = (mockBaseUrls[c.id] ?? '').trim()
    if (!mockBaseUrl) {
      push({ tone: 'error', title: EVAL.checkStateNeedUrl })
      return
    }
    setBusy(true)
    try {
      const updated = await checkEvalCaseState(c.id, mockBaseUrl)
      setCases(
        (prev) =>
          prev?.map((row) =>
            row.id === updated.id ? mergeCaseAfterCheckState(row, updated) : row,
          ) ?? null,
      )
      await refreshSummary()
      const codes = updated.last_score_codes?.join(', ')
      push({
        tone: updated.last_score_pass ? 'success' : 'error',
        title: updated.last_score_pass ? EVAL.toastCheckStatePass : EVAL.toastCheckStateFail,
        detail: codes ?? updated.last_score_notes ?? '',
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const rescore = async (c: EvalCase) => {
    setBusy(true)
    try {
      const res = await scoreEvalCase(c.id)
      await refreshCasesAndSummary()
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
      await refreshCasesAndSummary()
      push({ tone: 'success', title: EVAL.toastDeleted })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-panel settings-eval">
      <PageHeader title={EVAL.title} description={EVAL.description} />
      <div className="settings-eval-hints">
        <p className="settings-muted">{EVAL.calibrationHint}</p>
        <p className="settings-muted">{EVAL.harnessHint}</p>
      </div>

      <div className="settings-eval-summary" data-testid="eval-calibration-summary">
        {calibrationSummary == null ? (
          <span>{EVAL.casesLoading}</span>
        ) : (
          <span>
            {EVAL.calibrationSummaryScored(calibrationSummary.scored_count)}
            {' · '}
            {EVAL.calibrationSummaryCalibrated(calibrationSummary.calibrated_count)}
            {' · '}
            {formatAgreementRate(calibrationSummary.agreement_rate)}
          </span>
        )}
      </div>

      <h2 className="settings-subheading">{EVAL.candidatesTitle}</h2>
      <form className="settings-form" onSubmit={(e) => void runQuery(e)}>
        <div className="settings-form-grid">
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
              placeholder={EVAL.statusPlaceholder}
            />
          </Field>
          <Field label={EVAL.minDurationLabel}>
            <Input
              value={minDurationMs}
              onChange={(e) => setMinDurationMs(e.target.value)}
              disabled={busy}
              inputMode="numeric"
              placeholder={EVAL.minDurationPlaceholder}
            />
          </Field>
          <Field label={EVAL.eventTypeLabel}>
            <Input
              value={eventType}
              onChange={(e) => setEventType(e.target.value)}
              disabled={busy}
            />
          </Field>
        </div>
        <label className="settings-check">
          <input
            type="checkbox"
            checked={sampleWithScore}
            onChange={(e) => setSampleWithScore(e.target.checked)}
            disabled={busy}
            data-testid="eval-sample-score"
          />{' '}
          {EVAL.sampleWithScore}
        </label>
        <div className="settings-form-actions">
          <Button type="submit" disabled={busy} data-testid="eval-query">
            {busy ? EVAL.querying : EVAL.query}
          </Button>
          <Button
            type="button"
            variant="secondary"
            disabled={busy}
            data-testid="eval-sample"
            onClick={() => void runSample()}
          >
            {busy ? EVAL.sampling : EVAL.sample}
          </Button>
        </div>
      </form>

      <div data-testid="eval-candidates">
        {candidates === null ? (
          <p className="settings-muted">{EVAL.candidatesIdle}</p>
        ) : candidates.length === 0 ? (
          <EmptyState
            icon={<FlaskConical size={28} aria-hidden="true" />}
            title={EVAL.candidatesEmpty}
          />
        ) : (
          <ul className="settings-list">
            {candidates.map((c) => (
              <li key={c.id} className="settings-list-item">
                <div className="settings-tool-row">
                  <div className="settings-tool-line">
                    <p className="settings-tool-name" title={c.input_preview || c.id}>
                      {candidateDisplayTitle(c)}
                    </p>
                    <p className="settings-tool-sub">
                      {c.status}
                      {' · '}
                      {formatDuration(c.duration_ms)}
                      {' · '}
                      <code className="settings-code-inline" title={c.id}>
                        {shortId(c.id)}
                      </code>
                    </p>
                  </div>
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
                      variant={digSkillVariant(c)}
                      disabled={busy}
                      data-testid={`eval-dig-skill-${c.id}`}
                      onClick={() => void openSkillDraft(c.id)}
                    >
                      {EVAL.digSkill}
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
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      <h2 className="settings-subheading">{EVAL.casesTitle}</h2>
      <p className="settings-muted">{EVAL.checkStateHint}</p>
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
                  <div className="settings-tool-line">
                    <p className="settings-tool-name" title={c.input || c.source_run_id}>
                      {caseDisplayTitle(c)}
                    </p>
                    <p className="settings-tool-sub">
                      {EVAL.caseStatus(c.status)}
                      {c.protocol ? ` · ${c.protocol}` : ''}
                      {' · '}
                      {scoreSummary(c)}
                    </p>
                    {c.last_score_notes ? (
                      <p className="settings-tool-desc" title={c.last_score_notes}>
                        {c.last_score_notes}
                      </p>
                    ) : null}
                    {trajectorySummaryLines(c).map((line, i) => (
                      <p key={`${c.id}-tr-${i}`} className="settings-muted settings-code-sample">
                        {line}
                      </p>
                    ))}
                    {c.has_state_snapshot ? (
                      <p className="settings-tool-sub">{EVAL.hasStateSnapshot}</p>
                    ) : null}
                    {humanCalibrationLabel(c) ? (
                      <p className="settings-tool-sub">{humanCalibrationLabel(c)}</p>
                    ) : null}
                    {isCalibrationStale(c) ? (
                      <p
                        className="settings-tool-sub"
                        data-testid={`eval-calibration-stale-${c.id}`}
                      >
                        {EVAL.calibrationStale}
                      </p>
                    ) : null}
                    {isCalibratable(c) ? (
                      <div className="settings-eval-check">
                        <Field label={EVAL.calibrationNoteLabel}>
                          <Input
                            value={calibrationNotes[c.id] ?? ''}
                            onChange={(e) =>
                              setCalibrationNotes((prev) => ({
                                ...prev,
                                [c.id]: e.target.value,
                              }))
                            }
                            disabled={busy}
                            data-testid={`eval-calibrate-note-${c.id}`}
                          />
                        </Field>
                        <div className="accounts-actions">
                          <Button
                            type="button"
                            size="sm"
                            variant="secondary"
                            disabled={busy}
                            data-testid={`eval-calibrate-agree-${c.id}`}
                            onClick={() => void submitCalibration(c, true)}
                          >
                            {EVAL.calibrationAgree}
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            variant="secondary"
                            disabled={busy}
                            data-testid={`eval-calibrate-disagree-${c.id}`}
                            onClick={() => void submitCalibration(c, false)}
                          >
                            {EVAL.calibrationDisagree}
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            variant="secondary"
                            disabled={busy || c.human_agree == null}
                            data-testid={`eval-calibrate-clear-${c.id}`}
                            onClick={() => void submitCalibration(c, null)}
                          >
                            {EVAL.calibrationClear}
                          </Button>
                        </div>
                      </div>
                    ) : null}
                    {c.status === 'confirmed' ? (
                      <div className="settings-eval-check" data-testid={`eval-check-state-${c.id}`}>
                        <Field label={EVAL.checkStateMockLabel}>
                          <Input
                            value={mockBaseUrls[c.id] ?? ''}
                            onChange={(e) =>
                              setMockBaseUrls((prev) => ({ ...prev, [c.id]: e.target.value }))
                            }
                            disabled={busy}
                            placeholder="http://127.0.0.1:8080"
                            data-testid={`eval-check-state-url-${c.id}`}
                          />
                        </Field>
                        <Button
                          type="button"
                          size="sm"
                          variant="secondary"
                          disabled={busy}
                          data-testid={`eval-check-state-btn-${c.id}`}
                          onClick={() => void runCheckState(c)}
                        >
                          {EVAL.checkStateButton}
                        </Button>
                      </div>
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
          <pre className="settings-log-viewer settings-log-viewer--modal">
            {JSON.stringify(eventsModal.events, null, 2)}
          </pre>
        )}
      </Modal>

      <SkillDraftModal
        open={skillDraftModal != null}
        runId={skillDraftModal?.runId ?? null}
        draft={skillDraftModal?.draft ?? null}
        markdown={skillDraftMarkdown}
        onMarkdownChange={setSkillDraftMarkdown}
        createEvalStub={skillDraftEvalStub}
        onCreateEvalStubChange={setSkillDraftEvalStub}
        busy={busy}
        onClose={() => setSkillDraftModal(null)}
        onPolish={() => void polishSkillDraftMarkdown()}
        onInstall={() => void confirmInstallSkillDraft()}
      />

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
