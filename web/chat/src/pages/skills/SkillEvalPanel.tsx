import { useCallback, useEffect, useState } from 'react'
import {
  deleteSkillEval,
  getSkillEval,
  putSkillEval,
  runSkillEval,
  type SkillEvalCase,
  type SkillEvalDoc,
  type SkillEvalRunResult,
  type SkillSummary,
} from '../../api/skills'
import { Button, ConfirmDialog } from '../../components/ui'
import { SKILLS, skillErrorText } from '../../strings'

const EVIDENCE_HARNESS = 'evidence-eval'

function evalCasesValidForSave(cases: SkillEvalCase[]): boolean {
  return cases.every((c) => c.id.trim() !== '' && c.input.trim() !== '')
}

function summarizeInput(input: string, max = 72): string {
  const one = input.replace(/\s+/g, ' ').trim()
  if (one.length <= max) return one
  return `${one.slice(0, max - 1)}…`
}

export type SkillEvalPanelProps = {
  skill: SkillSummary
  agentId: string
  role: 'admin' | 'operator'
  onEvalMutated?: () => void
}

type PendingRun = { mode: 'all' } | { mode: 'one'; caseId: string }

export function SkillEvalPanel({ skill, agentId, role, onEvalMutated }: SkillEvalPanelProps) {
  const canEdit = role === 'admin' && skill.source === 'user'
  const canRun = role === 'admin'
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [doc, setDoc] = useState<SkillEvalDoc | null>(null)
  const [draft, setDraft] = useState<SkillEvalCase[]>([])
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [pendingClear, setPendingClear] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [clearError, setClearError] = useState<string | null>(null)
  const [pendingRun, setPendingRun] = useState<PendingRun | null>(null)
  const [running, setRunning] = useState(false)
  const [runError, setRunError] = useState<string | null>(null)
  const [runResults, setRunResults] = useState<SkillEvalRunResult[] | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setLoadError(null)
    try {
      const loaded = await getSkillEval(skill.id)
      setDoc(loaded)
      setDraft(loaded.cases ?? [])
      setRunResults(null)
    } catch (err) {
      setDoc(null)
      setDraft([])
      const f = skillErrorText(err)
      setLoadError(f.detail ? `${f.title} ${f.detail}` : f.title)
    } finally {
      setLoading(false)
    }
  }, [skill.id])

  useEffect(() => {
    void load()
  }, [load])

  const onSave = async () => {
    if (!canEdit) return
    if (!evalCasesValidForSave(draft)) {
      setSaveError(SKILLS.evalValidationBeforeSave)
      return
    }
    setSaving(true)
    setSaveError(null)
    try {
      const next = await putSkillEval(skill.id, { harness: EVIDENCE_HARNESS, cases: draft })
      setDoc(next)
      setDraft(next.cases ?? [])
      onEvalMutated?.()
    } catch (err) {
      const f = skillErrorText(err)
      setSaveError(f.detail ? `${f.title} ${f.detail}` : f.title)
    } finally {
      setSaving(false)
    }
  }

  const confirmClear = async () => {
    if (!canEdit) return
    setClearing(true)
    setClearError(null)
    try {
      await deleteSkillEval(skill.id)
      const empty = { harness: EVIDENCE_HARNESS, cases: [] as SkillEvalCase[] }
      setDoc(empty)
      setDraft([])
      setPendingClear(false)
      onEvalMutated?.()
    } catch (err) {
      const f = skillErrorText(err)
      setClearError(f.detail ? `${f.title} ${f.detail}` : f.title)
    } finally {
      setClearing(false)
    }
  }

  const confirmRun = async () => {
    if (!pendingRun || !canRun) return
    setRunning(true)
    setRunError(null)
    try {
      const case_ids = pendingRun.mode === 'one' ? [pendingRun.caseId] : undefined
      const { results } = await runSkillEval(skill.id, { agent_id: agentId, case_ids })
      setRunResults(results)
      setPendingRun(null)
    } catch (err) {
      const f = skillErrorText(err)
      setRunError(f.detail ? `${f.title} ${f.detail}` : f.title)
    } finally {
      setRunning(false)
    }
  }

  const busy = saving || clearing || running
  const cases = canEdit ? draft : (doc?.cases ?? [])
  const readOnlyHint =
    !canEdit && role === 'admin' && skill.source !== 'user' ? SKILLS.evalReadonlyBuiltin : null

  return (
    <div className="settings-skill-eval" data-testid="skill-eval-panel">
      {loading && <p className="settings-muted">{SKILLS.evalLoading}</p>}
      {loadError && <p className="settings-error">{loadError}</p>}
      {!loading && !loadError && (
        <>
          {readOnlyHint && <p className="settings-muted">{readOnlyHint}</p>}
          {cases.length === 0 ? (
            <p className="settings-muted">{SKILLS.evalEmpty}</p>
          ) : (
            <ul className="settings-list settings-skill-eval-cases">
              {cases.map((c, idx) => (
                <li key={`${c.id}-${idx}`} className="settings-list-item settings-skill-eval-case">
                  {canEdit ? (
                    <div className="settings-skill-eval-fields">
                      <label className="settings-field">
                        <span className="settings-muted">{SKILLS.evalCaseId}</span>
                        <input
                          className="settings-input"
                          value={c.id}
                          disabled={busy}
                          onChange={(e) => {
                            const id = e.target.value
                            setDraft((prev) => prev.map((row, i) => (i === idx ? { ...row, id } : row)))
                          }}
                        />
                      </label>
                      <label className="settings-field">
                        <span className="settings-muted">{SKILLS.evalProtocol}</span>
                        <select
                          className="settings-input"
                          value={c.protocol}
                          disabled={busy}
                          onChange={(e) => {
                            const protocol = e.target.value
                            setDraft((prev) =>
                              prev.map((row, i) => (i === idx ? { ...row, protocol } : row)),
                            )
                          }}
                        >
                          <option value="V0">V0</option>
                          <option value="V1">V1</option>
                          <option value="V2">V2</option>
                        </select>
                      </label>
                      <label className="settings-field settings-skill-eval-input">
                        <span className="settings-muted">{SKILLS.evalInput}</span>
                        <textarea
                          className="settings-textarea"
                          rows={2}
                          value={c.input}
                          disabled={busy}
                          onChange={(e) => {
                            const input = e.target.value
                            setDraft((prev) =>
                              prev.map((row, i) => (i === idx ? { ...row, input } : row)),
                            )
                          }}
                        />
                      </label>
                      <Button
                        size="sm"
                        variant="danger"
                        disabled={busy}
                        onClick={() => setDraft((prev) => prev.filter((_, i) => i !== idx))}
                      >
                        {SKILLS.evalRemoveCase}
                      </Button>
                    </div>
                  ) : (
                    <div className="settings-skill-eval-readonly">
                      <span className="settings-tool-title" data-testid="skill-eval-case-id">
                        {c.id}
                      </span>
                      <span className="settings-muted">{c.protocol}</span>
                      <span className="settings-muted">{summarizeInput(c.input)}</span>
                    </div>
                  )}
                  {canRun && c.id.trim() && (
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={busy || !c.input.trim()}
                      onClick={() => setPendingRun({ mode: 'one', caseId: c.id })}
                    >
                      {SKILLS.evalRunOne}
                    </Button>
                  )}
                  {runResults?.find((r) => r.id === c.id) && (
                    <p className="settings-muted settings-skill-eval-result">
                      {formatRunResult(runResults.find((r) => r.id === c.id)!)}
                    </p>
                  )}
                </li>
              ))}
            </ul>
          )}

          <div className="settings-skill-eval-toolbar">
            {canEdit && (
              <>
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={busy}
                  onClick={() =>
                    setDraft((prev) => [
                      ...prev,
                      { id: `case-${prev.length + 1}`, protocol: 'V0', input: '' },
                    ])
                  }
                >
                  {SKILLS.evalAddCase}
                </Button>
                <Button size="sm" variant="primary" disabled={busy} onClick={() => void onSave()}>
                  {saving ? SKILLS.evalSaving : SKILLS.evalSave}
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={busy || (doc?.cases?.length ?? 0) === 0 && draft.length === 0}
                  onClick={() => setPendingClear(true)}
                >
                  {SKILLS.evalClear}
                </Button>
              </>
            )}
            {canRun && cases.length > 0 && (
              <Button
                size="sm"
                variant="primary"
                disabled={busy}
                onClick={() => setPendingRun({ mode: 'all' })}
              >
                {running ? SKILLS.evalRunBusy : SKILLS.evalRunAll}
              </Button>
            )}
          </div>
          {saveError && <p className="settings-error">{saveError}</p>}
        </>
      )}

      {canEdit && (
        <ConfirmDialog
          open={pendingClear}
          danger
          title={SKILLS.evalClearConfirmTitle}
          body={SKILLS.evalClearConfirmBody}
          confirmText={SKILLS.evalClear}
          busy={clearing}
          error={clearError}
          onCancel={() => {
            if (clearing) return
            setPendingClear(false)
            setClearError(null)
          }}
          onConfirm={() => void confirmClear()}
        />
      )}

      {canRun && (
        <ConfirmDialog
          open={pendingRun !== null}
          title={SKILLS.evalConfirmRunTitle}
          body={SKILLS.evalConfirmRunBody}
          confirmText={SKILLS.evalConfirmRunOk}
          busy={running}
          error={runError}
          onCancel={() => {
            if (running) return
            setPendingRun(null)
            setRunError(null)
          }}
          onConfirm={() => void confirmRun()}
        />
      )}
    </div>
  )
}

function formatRunResult(r: SkillEvalRunResult): string {
  const codes = r.codes?.length ? r.codes.join(', ') : '—'
  return r.pass ? SKILLS.evalResultPass(codes) : SKILLS.evalResultFail(codes, r.notes)
}

/** Label for skill row from list summary count. */
export function skillEvalCountLabel(count?: number): string {
  const n = count ?? 0
  return n > 0 ? SKILLS.evalCount(n) : SKILLS.evalNone
}
