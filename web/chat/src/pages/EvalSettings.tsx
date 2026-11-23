import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { FlaskConical } from 'lucide-react'
import {
  checkEvalCaseState,
  deleteEvalCase,
  exportEvalCase,
  getCalibrationSummary,
  ingestEvalCase,
  listEvalCases,
  listEvalScores,
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
  type EvalArgContains,
  type EvalChainLink,
  type EvalExpectState,
  type EvalScoreRecord,
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

/** Parse the expect_state JSON textarea; empty means no state expectations. */
function parseExpectState(raw: string): { value?: EvalExpectState[]; error?: string } {
  const trimmed = raw.trim()
  if (!trimmed) return { value: [] }
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    return { error: EVAL.expectStateInvalid }
  }
  if (!Array.isArray(parsed)) return { error: EVAL.expectStateShape }
  const out: EvalExpectState[] = []
  for (const item of parsed) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) {
      return { error: EVAL.expectStateShape }
    }
    const fields = (item as { fields?: unknown }).fields
    if (!fields || typeof fields !== 'object' || Array.isArray(fields)) {
      return { error: EVAL.expectStateShape }
    }
    out.push({ fields: fields as EvalExpectState['fields'] })
  }
  return { value: out }
}

/** Turn score notes into plain sentences; unknown notes fall through unchanged. */
function describeScoreNote(note: string): string {
  const n = note.trim()
  let m: RegExpMatchArray | null
  if ((m = n.match(/^missing_reads:(.*)$/))) return EVAL.reasonMissingReads(m[1])
  if ((m = n.match(/^acted_before_reads:(.*)$/))) return EVAL.reasonActedBeforeReads(m[1])
  if ((m = n.match(/^chain_break:(.+?)->(.+)$/))) return EVAL.reasonChainBreak(m[1], m[2])
  if ((m = n.match(/^forbid:(.*)$/))) return EVAL.reasonForbiddenTool(m[1])
  if ((m = n.match(/^arg_contains:([^:]+):(.*)$/))) return EVAL.reasonArgMissing(m[1], m[2])
  if (n === 'state_mismatch') return EVAL.reasonStateMismatch
  if (n === 'missing_state_snapshot') return EVAL.reasonStateMissingSnapshot
  if (n === 'no_expected_consequential') return EVAL.reasonNoExpectedAction
  if ((m = n.match(/^consequential_count=(\d+)$/))) return EVAL.reasonMultipleActions(m[1])
  if ((m = n.match(/^unexpected_consequential:(.*)$/))) return EVAL.reasonUnexpectedActions(m[1])
  if ((m = n.match(/^unknown_protocol:(.*)$/))) return EVAL.reasonBadProtocol(m[1])
  if (n === 'timeout') return EVAL.reasonTimeout
  return n
}

function scoreCodeLabel(code: string): string {
  switch (code) {
    case 'BSR':
      return EVAL.codeBSR
    case 'PAR':
      return EVAL.codePAR
    case 'NO_ACTION':
      return EVAL.codeNoAction
    case 'MULTI':
      return EVAL.codeMulti
    case 'GAP':
      return EVAL.codeGap
    case 'FORBID':
      return EVAL.codeForbid
    case 'ARG':
      return EVAL.codeArg
    case 'STATE':
      return EVAL.codeState
    case 'TIMEOUT':
      return EVAL.codeTimeout
    case 'BAD_PROTOCOL':
      return EVAL.codeBadProtocol
    default:
      return code
  }
}

function actualToolCalls(events: Event[]): string[] {
  const out: string[] = []
  for (const ev of events) {
    if (ev.type !== 'llm.tool_call') continue
    const name = ev.data?.name
    if (typeof name === 'string' && name.trim()) out.push(name)
  }
  return out
}

function scoreNoteLines(notes?: string): string[] {
  if (!notes) return []
  return notes
    .split(';')
    .map((s) => s.trim())
    .filter(Boolean)
    .map(describeScoreNote)
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
  const [expectAny, setExpectAny] = useState('')
  const [forbidTools, setForbidTools] = useState('')
  const [chainLinks, setChainLinks] = useState<{ after: string; before: string }[]>([])
  const [argRules, setArgRules] = useState<{ tool: string; substr: string }[]>([])
  const [expectStateText, setExpectStateText] = useState('[]')
  const [caseStatus, setCaseStatus] = useState<'draft' | 'confirmed'>('draft')
  const [formError, setFormError] = useState<string | null>(null)
  const [compareModal, setCompareModal] = useState<{ item: EvalCase; events: Event[] | null } | null>(
    null,
  )
  const [scoreHistory, setScoreHistory] = useState<Record<string, EvalScoreRecord[]>>({})
  const [scoreLoading, setScoreLoading] = useState<Record<string, boolean>>({})
  const [selectedCases, setSelectedCases] = useState<Record<string, boolean>>({})
  const [pendingDelete, setPendingDelete] = useState<EvalCase | null>(null)
  const [eventsModal, setEventsModal] = useState<{ runId: string; events: Event[] | null } | null>(
    null,
  )
  const [calibrationSummary, setCalibrationSummary] = useState<EvalCalibrationSummary | null>(null)
  const [calibrationNotes, setCalibrationNotes] = useState<Record<string, string>>({})
  const [stateUrls, setStateUrls] = useState<Record<string, string>>({})
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
    setExpectAny('')
    setForbidTools('')
    setChainLinks([])
    setArgRules([])
    setExpectStateText('[]')
    setCaseStatus('draft')
    setFormError(null)
    setExpectationModal({ mode: 'ingest', runId: c.id, inputPreview: c.input_preview })
  }

  const openEditDraft = (c: EvalCase) => {
    setProtocol((c.protocol || 'V1').trim())
    setRequiredReads(textFromLines(c.required_reads))
    setConsequential(textFromLines(c.consequential))
    setExpectAny(textFromLines(c.expect_consequential_any))
    setForbidTools(textFromLines(c.forbid_tools))
    setChainLinks((c.chain ?? []).map((l) => ({ after: l.after, before: l.before })))
    setArgRules((c.arg_contains ?? []).map((r) => ({ tool: r.tool, substr: r.substr })))
    setExpectStateText(c.expect_state?.length ? JSON.stringify(c.expect_state, null, 2) : '[]')
    setCaseStatus(c.status)
    setFormError(null)
    setExpectationModal({ mode: 'edit', evalCase: c })
  }

  const submitExpectations = async () => {
    if (!expectationModal) return
    const proto = protocol.trim()
    if (!proto) {
      push({ tone: 'error', title: EVAL.needProtocol })
      return
    }
    const chain: EvalChainLink[] = []
    for (const row of chainLinks) {
      const after = row.after.trim()
      const before = row.before.trim()
      if (!after && !before) continue
      if (!after || !before) {
        setFormError(EVAL.rowIncomplete)
        return
      }
      chain.push({ after, before })
    }
    const argContains: EvalArgContains[] = []
    for (const row of argRules) {
      const tool = row.tool.trim()
      const substr = row.substr.trim()
      if (!tool && !substr) continue
      if (!tool || !substr) {
        setFormError(EVAL.rowIncomplete)
        return
      }
      argContains.push({ tool, substr })
    }
    const state = parseExpectState(expectStateText)
    if (state.error) {
      setFormError(state.error)
      return
    }
    setFormError(null)
    const common = {
      protocol: proto,
      required_reads: linesFromText(requiredReads),
      consequential: linesFromText(consequential),
      expect_consequential_any: linesFromText(expectAny),
      chain,
      forbid_tools: linesFromText(forbidTools),
      arg_contains: argContains,
      expect_state: state.value ?? [],
    }
    setBusy(true)
    try {
      if (expectationModal.mode === 'ingest') {
        await ingestEvalCase({ run_id: expectationModal.runId, ...common })
        push({ tone: 'success', title: EVAL.toastIngested })
      } else {
        await patchEvalCase(expectationModal.evalCase.id, { ...common, status: caseStatus })
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

  const openFailureCompare = async (c: EvalCase) => {
    setCompareModal({ item: c, events: null })
    try {
      const events = await listEvents(c.source_run_id)
      setCompareModal({ item: c, events })
    } catch (err) {
      setCompareModal({ item: c, events: [] })
      const f = friendlyError(err)
      push({ tone: 'error', title: EVAL.compareLoadFailed, detail: f.detail ?? f.title })
    }
  }

  const runCheckState = async (c: EvalCase) => {
    const stateUrl = (stateUrls[c.id] ?? '').trim()
    if (!stateUrl) {
      push({ tone: 'error', title: EVAL.checkStateNeedUrl })
      return
    }
    setBusy(true)
    try {
      const updated = await checkEvalCaseState(c.id, stateUrl)
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

  const loadScores = async (c: EvalCase) => {
    if (scoreHistory[c.id] || scoreLoading[c.id]) return
    setScoreLoading((prev) => ({ ...prev, [c.id]: true }))
    try {
      const body = await listEvalScores(c.id)
      setScoreHistory((prev) => ({ ...prev, [c.id]: [...body.items].reverse() }))
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setScoreLoading((prev) => {
        const next = { ...prev }
        delete next[c.id]
        return next
      })
    }
  }

  const selectedIDs = (): string[] =>
    (cases ?? []).filter((c) => selectedCases[c.id]).map((c) => c.id)

  const toggleSelected = (id: string) => {
    setSelectedCases((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const runBatchScore = async () => {
    const ids = selectedIDs()
    if (!ids.length) return
    setBusy(true)
    let passed = 0
    try {
      for (const id of ids) {
        const res = await scoreEvalCase(id)
        if (res.pass) passed++
      }
      await refreshCasesAndSummary()
      setSelectedCases({})
      push({ tone: 'success', title: EVAL.toastBatchScored(passed, ids.length) })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const runBatchConfirm = async () => {
    const drafts = (cases ?? []).filter((c) => selectedCases[c.id] && c.status === 'draft')
    if (!drafts.length) return
    setBusy(true)
    try {
      for (const c of drafts) {
        await patchEvalCase(c.id, { status: 'confirmed' })
      }
      await refreshCasesAndSummary()
      setSelectedCases({})
      push({ tone: 'success', title: EVAL.toastBatchConfirmed(drafts.length) })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const runBatchExport = async () => {
    const ids = selectedIDs()
    if (!ids.length) return
    setBusy(true)
    try {
      const exported: unknown[] = []
      for (const id of ids) {
        exported.push(JSON.parse(await exportEvalCase(id)))
      }
      const blob = new Blob(
        [JSON.stringify({ exported_at: new Date().toISOString(), cases: exported }, null, 2)],
        { type: 'application/json' },
      )
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'eval-cases-selected.json'
      a.click()
      URL.revokeObjectURL(url)
      push({ tone: 'success', title: EVAL.toastBatchExported(ids.length) })
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
      {cases && cases.length > 0 ? (
        <div className="eval-batch-bar" data-testid="eval-batch-bar">
          <span className="settings-muted">
            {EVAL.selectedCount(cases.filter((c) => selectedCases[c.id]).length)}
          </span>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            disabled={busy || !cases.some((c) => selectedCases[c.id])}
            onClick={() => void runBatchScore()}
          >
            {EVAL.batchScore}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            disabled={
              busy ||
              !cases.some((c) => selectedCases[c.id] && c.status === 'draft')
            }
            onClick={() => void runBatchConfirm()}
          >
            {EVAL.batchConfirm}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            disabled={busy || !cases.some((c) => selectedCases[c.id])}
            onClick={() => void runBatchExport()}
          >
            {EVAL.batchExport}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={busy || !cases.some((c) => selectedCases[c.id])}
            onClick={() => setSelectedCases({})}
          >
            {EVAL.clearSelection}
          </Button>
        </div>
      ) : null}
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
                  <label className="eval-select">
                    <input
                      type="checkbox"
                      checked={!!selectedCases[c.id]}
                      onChange={() => toggleSelected(c.id)}
                      title={EVAL.selectCase}
                      aria-label={EVAL.selectCase}
                    />
                  </label>
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
                    {c.last_score_pass === false &&
                    (c.last_score_codes?.length || c.last_score_notes) ? (
                      <div className="eval-score-reasons" data-testid={`eval-reasons-${c.id}`}>
                        <p className="settings-tool-desc">{EVAL.scoreReasonsLabel}</p>
                        <ul className="eval-score-reason-list">
                          {(c.last_score_codes ?? []).map((code) => (
                            <li key={code}>
                              <strong>{scoreCodeLabel(code)}</strong>{' '}
                              <span className="settings-muted">{code}</span>
                            </li>
                          ))}
                          {scoreNoteLines(c.last_score_notes).map((line, i) => (
                            <li key={`${c.id}-note-${i}`}>{line}</li>
                          ))}
                        </ul>
                        <Button
                          type="button"
                          size="sm"
                          variant="secondary"
                          onClick={() => void openFailureCompare(c)}
                        >
                          {EVAL.viewFailureCompare}
                        </Button>
                      </div>
                    ) : null}
                    {scoreHistory[c.id] ? (
                      <div className="eval-trend" data-testid={`eval-trend-${c.id}`}>
                        <span className="settings-muted">
                          {scoreHistory[c.id].length
                            ? EVAL.trendSummary(
                                scoreHistory[c.id].filter((r) => r.pass).length,
                                scoreHistory[c.id].length,
                              )
                            : EVAL.trendEmpty}
                        </span>
                        <span className="eval-trend-dots">
                          {scoreHistory[c.id].map((r, i) => (
                            <span
                              key={`${c.id}-score-${i}`}
                              className={`eval-trend-dot ${r.pass ? 'pass' : 'fail'}`}
                              title={`${r.scored_at} ${r.pass ? '✓' : '✗'}`}
                            />
                          ))}
                        </span>
                      </div>
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
                        <Field label={EVAL.checkStateUrlLabel}>
                          <Input
                            value={stateUrls[c.id] ?? ''}
                            onChange={(e) =>
                              setStateUrls((prev) => ({ ...prev, [c.id]: e.target.value }))
                            }
                            disabled={busy}
                            placeholder="http://127.0.0.1:18080/records"
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
                      disabled={busy || !!scoreLoading[c.id]}
                      onClick={() => void loadScores(c)}
                    >
                      {EVAL.trend}
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
            <Field label={EVAL.expectAnyLabel} hint={EVAL.listHint}>
              <Textarea
                value={expectAny}
                onChange={(e) => setExpectAny(e.target.value)}
                disabled={busy}
                rows={2}
              />
            </Field>
            <Field label={EVAL.forbidToolsLabel} hint={EVAL.listHint}>
              <Textarea
                value={forbidTools}
                onChange={(e) => setForbidTools(e.target.value)}
                disabled={busy}
                rows={2}
              />
            </Field>
            <Field label={EVAL.chainLabel} hint={EVAL.chainHint}>
              {chainLinks.map((row, i) => (
                <div className="eval-expect-row" key={`chain-${i}`}>
                  <Input
                    value={row.after}
                    placeholder={EVAL.chainAfterPlaceholder}
                    disabled={busy}
                    onChange={(e) =>
                      setChainLinks((prev) =>
                        prev.map((r, j) => (j === i ? { ...r, after: e.target.value } : r)),
                      )
                    }
                  />
                  <span className="settings-muted">→</span>
                  <Input
                    value={row.before}
                    placeholder={EVAL.chainBeforePlaceholder}
                    disabled={busy}
                    onChange={(e) =>
                      setChainLinks((prev) =>
                        prev.map((r, j) => (j === i ? { ...r, before: e.target.value } : r)),
                      )
                    }
                  />
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    onClick={() => setChainLinks((prev) => prev.filter((_, j) => j !== i))}
                  >
                    {EVAL.removeRow}
                  </Button>
                </div>
              ))}
              <Button
                type="button"
                size="sm"
                variant="secondary"
                disabled={busy}
                onClick={() => setChainLinks((prev) => [...prev, { after: '', before: '' }])}
              >
                {EVAL.addRow}
              </Button>
            </Field>
            <Field label={EVAL.argContainsLabel}>
              {argRules.map((row, i) => (
                <div className="eval-expect-row" key={`arg-${i}`}>
                  <Input
                    value={row.tool}
                    placeholder={EVAL.argToolPlaceholder}
                    disabled={busy}
                    onChange={(e) =>
                      setArgRules((prev) =>
                        prev.map((r, j) => (j === i ? { ...r, tool: e.target.value } : r)),
                      )
                    }
                  />
                  <Input
                    value={row.substr}
                    placeholder={EVAL.argSubstrPlaceholder}
                    disabled={busy}
                    onChange={(e) =>
                      setArgRules((prev) =>
                        prev.map((r, j) => (j === i ? { ...r, substr: e.target.value } : r)),
                      )
                    }
                  />
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    onClick={() => setArgRules((prev) => prev.filter((_, j) => j !== i))}
                  >
                    {EVAL.removeRow}
                  </Button>
                </div>
              ))}
              <Button
                type="button"
                size="sm"
                variant="secondary"
                disabled={busy}
                onClick={() => setArgRules((prev) => [...prev, { tool: '', substr: '' }])}
              >
                {EVAL.addRow}
              </Button>
            </Field>
            <Field label={EVAL.expectStateLabel} hint={EVAL.expectStateHint}>
              <Textarea
                value={expectStateText}
                onChange={(e) => setExpectStateText(e.target.value)}
                disabled={busy}
                rows={6}
                spellCheck={false}
              />
            </Field>
            {expectationModal.mode === 'edit' ? (
              <Field label={EVAL.caseStatusLabel}>
                <Select
                  value={caseStatus}
                  disabled={busy}
                  onChange={(e) => setCaseStatus(e.target.value as 'draft' | 'confirmed')}
                >
                  <option value="draft">{EVAL.statusDraft}</option>
                  <option value="confirmed">{EVAL.statusConfirmed}</option>
                </Select>
              </Field>
            ) : null}
            {formError ? <p className="settings-error">{formError}</p> : null}
            <p className="settings-muted">
              {expectationModal.mode === 'ingest' ? EVAL.ingestDraftHint : EVAL.editDraftHint}
            </p>
          </>
        ) : null}
      </Modal>

      <Modal
        open={compareModal != null}
        title={
          compareModal ? EVAL.compareTitle(compareModal.item.id) : EVAL.viewFailureCompare
        }
        onClose={() => setCompareModal(null)}
        footer={
          <>
            {compareModal ? (
              <Button
                type="button"
                variant="secondary"
                onClick={() => {
                  const runId = compareModal.item.source_run_id
                  setCompareModal(null)
                  void viewRunEvents(runId)
                }}
              >
                {EVAL.compareOpenEvents}
              </Button>
            ) : null}
            <Button type="button" onClick={() => setCompareModal(null)}>
              {EVAL.cancel}
            </Button>
          </>
        }
      >
        {compareModal ? (
          compareModal.events == null ? (
            <p className="settings-muted" data-testid="eval-compare-loading">
              {EVAL.compareLoading}
            </p>
          ) : (
            <>
              <div className="eval-compare-grid">
                <div>
                  <p className="settings-tool-name">{EVAL.compareExpected}</p>
                  <ul className="eval-compare-list">
                    <li>
                      {EVAL.compareExpectedReads(
                        compareModal.item.required_reads?.join(', ') || EVAL.none,
                      )}
                    </li>
                    <li>
                      {EVAL.compareExpectedActions(
                        [
                          ...(compareModal.item.expect_consequential_any ?? []),
                          ...(compareModal.item.consequential ?? []),
                        ].join(', ') || EVAL.none,
                      )}
                    </li>
                    {compareModal.item.chain?.map((l, i) => (
                      <li key={`chain-${i}`}>{EVAL.compareExpectedChain(l.after, l.before)}</li>
                    ))}
                    {compareModal.item.forbid_tools?.length ? (
                      <li>
                        {EVAL.compareExpectedForbid(compareModal.item.forbid_tools.join(', '))}
                      </li>
                    ) : null}
                    {compareModal.item.arg_contains?.length ? (
                      <li>
                        {EVAL.compareExpectedArgs(
                          compareModal.item.arg_contains
                            .map((r) => `${r.tool}: ${r.substr}`)
                            .join('; '),
                        )}
                      </li>
                    ) : null}
                  </ul>
                </div>
                <div>
                  <p className="settings-tool-name">{EVAL.compareActual}</p>
                  {actualToolCalls(compareModal.events).length ? (
                    <ol className="eval-compare-list" data-testid="eval-compare-tools">
                      {actualToolCalls(compareModal.events).map((name, i) => (
                        <li key={`${name}-${i}`}>{name}</li>
                      ))}
                    </ol>
                  ) : (
                    <p className="settings-muted">{EVAL.compareNoTools}</p>
                  )}
                </div>
              </div>
              <p className="settings-tool-desc">{EVAL.scoreReasonsLabel}</p>
              <ul className="eval-score-reason-list">
                {(compareModal.item.last_score_codes ?? []).map((code) => (
                  <li key={code}>
                    <strong>{scoreCodeLabel(code)}</strong>{' '}
                    <span className="settings-muted">{code}</span>
                  </li>
                ))}
                {scoreNoteLines(compareModal.item.last_score_notes).map((line, i) => (
                  <li key={`compare-note-${i}`}>{line}</li>
                ))}
              </ul>
            </>
          )
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
