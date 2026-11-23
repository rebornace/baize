import { useEffect, useMemo, useState } from 'react'
import { listTools, type ToolInfo } from '../../api/connectors'
import {
  cloneSkillWorkflow,
  deleteSkillWorkflow,
  getSkillWorkflow,
  previewSkillWorkflow,
  putSkillWorkflow,
  putSkillWorkflowRaw,
  type SkillSummary,
  type SkillWorkflow,
  type SkillWorkflowCondition,
  type SkillWorkflowPreview,
  type SkillWorkflowStep,
  type SkillWorkflowView,
} from '../../api/skills'
import { Button, ConfirmDialog, useToast } from '../../components/ui'
import { SKILLS } from '../../strings'

const OPS = ['eq', 'ne', 'contains', 'in', 'exists', 'not_exists', 'gt', 'gte', 'lt', 'lte']

type Props = {
  skill: SkillSummary
  readOnly: boolean
  onChanged: () => void
  onClose: () => void
}

function cloneWorkflow(w: SkillWorkflow): SkillWorkflow {
  return JSON.parse(JSON.stringify(w)) as SkillWorkflow
}

function argsText(step: SkillWorkflowStep): string {
  return JSON.stringify(step.args ?? {}, null, 2)
}

function parseArgs(raw: string): Record<string, unknown> | null {
  if (!raw.trim()) return {}
  try {
    const v: unknown = JSON.parse(raw)
    if (v && typeof v === 'object' && !Array.isArray(v)) return v as Record<string, unknown>
    return null
  } catch {
    return null
  }
}

function valueText(v: unknown): string {
  if (v === undefined || v === null) return ''
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

function parseLoose(raw: string): unknown {
  if (raw.trim() === '') return ''
  try {
    return JSON.parse(raw)
  } catch {
    return raw
  }
}

function conditionMode(c: SkillWorkflowCondition): 'leaf' | 'all' | 'any' | 'not' {
  if (c.all?.length) return 'all'
  if (c.any?.length) return 'any'
  if (c.not) return 'not'
  return 'leaf'
}

function ConditionEditor({
  value,
  onChange,
  readOnly,
}: {
  value: SkillWorkflowCondition
  onChange: (next: SkillWorkflowCondition) => void
  readOnly: boolean
}) {
  const mode = conditionMode(value)
  const setMode = (next: 'leaf' | 'all' | 'any' | 'not') => {
    if (next === 'leaf') onChange({ path: 'input.flag', op: 'eq', value: '' })
    else if (next === 'all') onChange({ all: [{ path: 'input.flag', op: 'eq', value: '' }] })
    else if (next === 'any') onChange({ any: [{ path: 'input.flag', op: 'eq', value: '' }] })
    else onChange({ not: { path: 'input.flag', op: 'eq', value: '' } })
  }
  if (readOnly) {
    return <code className="workflow-condition-readonly">{JSON.stringify(value)}</code>
  }
  return (
    <div className="workflow-condition">
      <div className="workflow-condition-head">
        <select
          value={mode}
          onChange={(e) => setMode(e.target.value as 'leaf' | 'all' | 'any' | 'not')}
          aria-label={SKILLS.workflowWhen}
        >
          <option value="leaf">{SKILLS.workflowConditionLeaf}</option>
          <option value="all">{SKILLS.workflowConditionAll}</option>
          <option value="any">{SKILLS.workflowConditionAny}</option>
          <option value="not">{SKILLS.workflowConditionNot}</option>
        </select>
      </div>
      {mode === 'leaf' ? (
        <div className="workflow-condition-leaf">
          <input
            value={value.path ?? ''}
            placeholder="input.flag"
            aria-label={SKILLS.workflowConditionPath}
            onChange={(e) => onChange({ ...value, path: e.target.value, all: undefined, any: undefined, not: undefined })}
          />
          <select
            value={value.op ?? 'eq'}
            aria-label={SKILLS.workflowConditionOp}
            onChange={(e) => onChange({ ...value, op: e.target.value })}
          >
            {OPS.map((op) => (
              <option key={op} value={op}>
                {op}
              </option>
            ))}
          </select>
          <input
            value={valueText(value.value)}
            placeholder="value"
            aria-label={SKILLS.workflowConditionValue}
            onChange={(e) => onChange({ ...value, value: parseLoose(e.target.value) })}
          />
        </div>
      ) : null}
      {mode === 'all' || mode === 'any' ? (
        <div className="workflow-condition-children">
          {(value.all ?? value.any ?? []).map((child, i) => (
            <div key={i} className="workflow-condition-child">
              <ConditionEditor
                value={child}
                readOnly={readOnly}
                onChange={(next) => {
                  const list = [...(value.all ?? value.any ?? [])]
                  list[i] = next
                  onChange(mode === 'all' ? { all: list } : { any: list })
                }}
              />
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  const list = [...(value.all ?? value.any ?? [])]
                  list.splice(i, 1)
                  onChange(
                    list.length === 0
                      ? { path: 'input.flag', op: 'eq', value: '' }
                      : mode === 'all'
                        ? { all: list }
                        : { any: list },
                  )
                }}
              >
                {SKILLS.workflowConditionRemove}
              </Button>
            </div>
          ))}
          <Button
            size="sm"
            variant="secondary"
            onClick={() => {
              const list = [...(value.all ?? value.any ?? []), { path: 'input.flag', op: 'eq', value: '' }]
              onChange(mode === 'all' ? { all: list } : { any: list })
            }}
          >
            {SKILLS.workflowConditionAdd}
          </Button>
        </div>
      ) : null}
      {mode === 'not' && value.not ? (
        <div className="workflow-condition-child">
          <ConditionEditor
            value={value.not}
            readOnly={readOnly}
            onChange={(next) => onChange({ not: next })}
          />
        </div>
      ) : null}
    </div>
  )
}

export function SkillWorkflowEditor({ skill, readOnly, onChanged, onClose }: Props) {
  const { push } = useToast()
  const [view, setView] = useState<SkillWorkflowView | null>(null)
  const [draft, setDraft] = useState<SkillWorkflow | null>(null)
  const [args, setArgs] = useState<Record<number, string>>({})
  const [tools, setTools] = useState<ToolInfo[]>([])
  const [busy, setBusy] = useState(false)
  const [mode, setMode] = useState<'form' | 'yaml'>('form')
  const [yamlText, setYamlText] = useState('')
  const [previewInput, setPreviewInput] = useState('{"text":"sample"}')
  const [preview, setPreview] = useState<SkillWorkflowPreview | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [previewBusy, setPreviewBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)

  useEffect(() => {
    let alive = true
    void (async () => {
      try {
        const [wf, toolList] = await Promise.all([
          getSkillWorkflow(skill.id),
          listTools().catch(() => [] as ToolInfo[]),
        ])
        if (!alive) return
        setTools(toolList)
        setView(wf)
        setYamlText(wf.yaml ?? '')
        if (wf.workflow) {
          const copy = cloneWorkflow(wf.workflow)
          setDraft(copy)
          setArgs(Object.fromEntries(copy.steps.map((s, i) => [i, argsText(s)])))
        } else {
          setDraft(null)
          setArgs({})
        }
      } catch (err) {
        if (alive) setError(err instanceof Error ? err.message : String(err))
      }
    })()
    return () => {
      alive = false
    }
  }, [skill.id])

  const toolNames = useMemo(() => {
    const names = tools.map((t) => t.name)
    for (const step of draft?.steps ?? []) {
      if (step.tool && !names.includes(step.tool)) names.push(step.tool)
    }
    return names
  }, [tools, draft])

  const beginCreate = () => {
    setError(null)
    setDraft({ name: skill.id, steps: [] })
    setArgs({})
  }

  const addStep = () => {
    const steps = draft?.steps ?? []
    let n = steps.length + 1
    const ids = new Set(steps.map((s) => s.id))
    while (ids.has(`step${n}`)) n++
    const tool =
      toolNames.find((name) => tools.find((t) => t.name === name && t.enabled !== false)) ??
      toolNames[0] ??
      ''
    const next = {
      ...(draft ?? { name: skill.id, steps: [] }),
      steps: [...steps, { id: `step${n}`, tool, args: {} }],
    }
    setDraft(next)
    setArgs((old) => ({ ...old, [next.steps.length - 1]: '{}' }))
  }

  const updateStep = (index: number, patch: Partial<SkillWorkflowStep>) => {
    setDraft((d) => {
      if (!d) return d
      const steps = [...d.steps]
      steps[index] = { ...steps[index], ...patch }
      return { ...d, steps }
    })
  }

  const moveStep = (index: number, delta: number) => {
    if (!draft) return
    const to = index + delta
    if (to < 0 || to >= draft.steps.length) return
    const steps = [...draft.steps]
    const [item] = steps.splice(index, 1)
    steps.splice(to, 0, item)
    setDraft({ ...draft, steps })
    setArgs((old) => {
      const next = { ...old }
      const a = next[index]
      next[index] = next[to]
      next[to] = a
      return next
    })
  }

  const removeStep = (index: number) => {
    if (!draft) return
    setDraft({ ...draft, steps: draft.steps.filter((_, j) => j !== index) })
    setArgs((old) => {
      const next: Record<number, string> = {}
      for (let j = 0, k = 0; j < draft.steps.length; j++) {
        if (j !== index) next[k++] = old[j] ?? ''
      }
      return next
    })
  }

  const applyView = (next: SkillWorkflowView) => {
    setView(next)
    setYamlText(next.yaml ?? '')
    if (next.workflow) {
      const copy = cloneWorkflow(next.workflow)
      setDraft(copy)
      setArgs(Object.fromEntries(copy.steps.map((s, i) => [i, argsText(s)])))
    } else {
      setDraft(null)
      setArgs({})
    }
  }

  const save = async () => {
    if (!draft) return
    setError(null)
    if (mode === 'yaml') {
      setBusy(true)
      try {
        const next = await putSkillWorkflowRaw(skill.id, yamlText)
        applyView(next)
        push({ tone: 'success', title: SKILLS.workflowSaved })
        onChanged()
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err))
      } finally {
        setBusy(false)
      }
      return
    }
    if (draft.steps.length === 0) {
      setError(SKILLS.workflowEmpty)
      return
    }
    const steps: SkillWorkflowStep[] = []
    for (let i = 0; i < draft.steps.length; i++) {
      const step = draft.steps[i]
      if (!step.id.trim() || step.id.includes('.')) {
        setError(SKILLS.workflowStepIdRequired)
        return
      }
      if (!step.tool.trim()) {
        setError(SKILLS.workflowToolRequired)
        return
      }
      const parsed = parseArgs(args[i] ?? '')
      if (parsed === null) {
        setError(SKILLS.workflowInvalidArgs(step.id))
        return
      }
      steps.push({ ...step, id: step.id.trim(), args: parsed })
    }
    setBusy(true)
    try {
      const next = await putSkillWorkflow(skill.id, {
        name: skill.id,
        timeout_ms: draft.timeout_ms,
        steps,
      })
      applyView(next)
      push({ tone: 'success', title: SKILLS.workflowSaved })
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const cloneWorkflowToUser = async () => {
    setBusy(true)
    setError(null)
    try {
      const next = await cloneSkillWorkflow(skill.id)
      applyView(next)
      setMode('form')
      push({ tone: 'success', title: SKILLS.workflowCloned })
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const removeWorkflow = async () => {
    setBusy(true)
    setError(null)
    try {
      await deleteSkillWorkflow(skill.id)
      setView((prev) => (prev ? { ...prev, workflow: undefined, yaml: undefined } : prev))
      setDraft(null)
      setArgs({})
      setYamlText('')
      setConfirmDelete(false)
      push({ tone: 'success', title: SKILLS.workflowDeleted })
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setConfirmDelete(false)
    } finally {
      setBusy(false)
    }
  }

  const runPreview = async () => {
    setPreviewError(null)
    let input: Record<string, unknown>
    try {
      const parsed: unknown = JSON.parse(previewInput)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('input must be a JSON object')
      input = parsed as Record<string, unknown>
    } catch (err) {
      setPreviewError(err instanceof Error ? err.message : String(err))
      return
    }
    setPreviewBusy(true)
    try {
      setPreview(await previewSkillWorkflow(skill.id, input))
    } catch (err) {
      setPreviewError(err instanceof Error ? err.message : String(err))
    } finally {
      setPreviewBusy(false)
    }
  }

  const editable = view?.editable && !readOnly
  return (
    <div className="workflow-editor" data-testid={`workflow-editor-${skill.id}`}>
      <div className="workflow-editor-head">
        <span className="settings-tool-title">{SKILLS.workflowTitle}</span>
        <div className="workflow-editor-actions">
          {view?.workflow && editable ? (
            <>
              <Button
                size="sm"
                variant={mode === 'form' ? 'primary' : 'ghost'}
                onClick={() => setMode('form')}
              >
                {SKILLS.workflowForm}
              </Button>
              <Button
                size="sm"
                variant={mode === 'yaml' ? 'primary' : 'ghost'}
                onClick={() => setMode('yaml')}
              >
                {SKILLS.workflowYAML}
              </Button>
            </>
          ) : null}
          <Button size="sm" variant="ghost" onClick={onClose}>
            {SKILLS.workflowClose}
          </Button>
        </div>
      </div>
      {error ? <p className="settings-error">{error}</p> : null}
      {!view ? <p className="settings-muted">{SKILLS.loading}</p> : null}
      {view && !view.workflow && !draft ? (
        <div className="workflow-editor-empty">
          <p className="settings-muted">{SKILLS.workflowEmpty}</p>
          {editable && view.can_create ? (
            <Button size="sm" variant="primary" onClick={beginCreate}>
              {SKILLS.workflowCreate}
            </Button>
          ) : (
            <p className="settings-muted">{SKILLS.workflowReadOnly}</p>
          )}
        </div>
      ) : null}
      {view && draft && !editable ? (
        <>
          <p className="settings-muted">{SKILLS.workflowReadOnly}</p>
          {view.source === 'builtin' && !readOnly ? (
            <>
              <p className="settings-muted">{SKILLS.workflowCloneHint}</p>
              <Button
                size="sm"
                variant="secondary"
                disabled={busy}
                onClick={() => void cloneWorkflowToUser()}
              >
                {SKILLS.workflowClone}
              </Button>
            </>
          ) : null}
          <pre className="workflow-readonly">{yamlText || JSON.stringify(draft, null, 2)}</pre>
        </>
      ) : null}
      {view && draft && editable ? (
        <>
          {mode === 'yaml' ? (
            <label className="workflow-args workflow-yaml-editor">
              <span>{SKILLS.workflowYAML}</span>
              <textarea
                rows={18}
                value={yamlText}
                onChange={(e) => setYamlText(e.target.value)}
              />
            </label>
          ) : (
          <>
          <div className="workflow-global">
            <label>
              <span>{SKILLS.workflowTotalTimeout}</span>
              <input
                type="number"
                min={0}
                max={86400}
                step={1}
                value={draft.timeout_ms ? Math.round(draft.timeout_ms / 1000) : 0}
                onChange={(e) => {
                  const n = Number.parseInt(e.target.value, 10)
                  const timeout_ms = Number.isFinite(n) ? Math.min(86400, Math.max(0, n)) * 1000 : 0
                  setDraft((d) => (d ? { ...d, timeout_ms } : d))
                }}
              />
            </label>
          </div>
          <div className="workflow-steps-editor">
            {draft.steps.map((step, i) => {
              const missing = step.tool && !tools.some((t) => t.name === step.tool)
              return (
                <div className="workflow-step-editor" key={`${step.id}-${i}`}>
                  <div className="workflow-step-editor-head">
                    <strong>{SKILLS.workflowStep(i + 1)}</strong>
                    <div className="workflow-step-editor-actions">
                      <Button size="sm" variant="ghost" disabled={i === 0} onClick={() => moveStep(i, -1)}>
                        {SKILLS.workflowMoveUp}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={i === draft.steps.length - 1}
                        onClick={() => moveStep(i, 1)}
                      >
                        {SKILLS.workflowMoveDown}
                      </Button>
                      <Button
                        size="sm"
                        variant="danger"
                        onClick={() => removeStep(i)}
                      >
                        {SKILLS.workflowRemoveStep}
                      </Button>
                    </div>
                  </div>
                  <div className="workflow-step-grid">
                    <label>
                      <span>{SKILLS.workflowStepId}</span>
                      <input
                        value={step.id}
                        onChange={(e) => updateStep(i, { id: e.target.value })}
                      />
                    </label>
                    <label>
                      <span>{SKILLS.workflowTool}</span>
                      <select value={step.tool} onChange={(e) => updateStep(i, { tool: e.target.value })}>
                        <option value="">—</option>
                        {toolNames.map((name) => (
                          <option key={name} value={name}>
                            {name}
                          </option>
                        ))}
                      </select>
                      {missing ? <span className="settings-error">{SKILLS.workflowMissingTool(step.tool)}</span> : null}
                    </label>
                    <label className="workflow-approve">
                      <input
                        type="checkbox"
                        checked={!!step.approve}
                        onChange={(e) => updateStep(i, { approve: e.target.checked })}
                      />
                      <span>{SKILLS.workflowApprove}</span>
                    </label>
                    <label className="workflow-approve">
                      <input
                        type="checkbox"
                        checked={!!step.continue_on_error}
                        onChange={(e) => updateStep(i, { continue_on_error: e.target.checked })}
                      />
                      <span>{SKILLS.workflowContinueOnError}</span>
                    </label>
                    <label className="workflow-retry">
                      <span>{SKILLS.workflowRetry}</span>
                      <input
                        type="number"
                        min={0}
                        max={5}
                        value={step.retry ?? 0}
                        onChange={(e) => {
                          const n = Number.parseInt(e.target.value, 10)
                          const retry = Number.isFinite(n) ? Math.min(5, Math.max(0, n)) : 0
                          updateStep(i, { retry })
                        }}
                      />
                    </label>
                    <label className="workflow-timeout">
                      <span>{SKILLS.workflowTimeout}</span>
                      <input
                        type="number"
                        min={0}
                        max={600}
                        step={1}
                        value={step.timeout_ms ? Math.round(step.timeout_ms / 1000) : 0}
                        onChange={(e) => {
                          const n = Number.parseInt(e.target.value, 10)
                          const timeout_ms = Number.isFinite(n) ? Math.min(600, Math.max(0, n)) * 1000 : 0
                          updateStep(i, { timeout_ms })
                        }}
                      />
                    </label>
                  </div>
                  <label className="workflow-args">
                    <span>{SKILLS.workflowArgs}</span>
                    <textarea
                      rows={4}
                      value={args[i] ?? '{}'}
                      onChange={(e) => setArgs((old) => ({ ...old, [i]: e.target.value }))}
                    />
                  </label>
                  <div className="workflow-when-row">
                    <label className="workflow-approve">
                      <input
                        type="checkbox"
                        checked={!!step.when}
                        onChange={(e) =>
                          updateStep(i, {
                            when: e.target.checked ? { path: 'input.flag', op: 'eq', value: '' } : undefined,
                          })
                        }
                      />
                      <span>{SKILLS.workflowWhen}</span>
                    </label>
                  </div>
                  {step.when ? (
                    <ConditionEditor
                      value={step.when}
                      readOnly={false}
                      onChange={(next) => updateStep(i, { when: next })}
                    />
                  ) : null}
                </div>
              )
            })}
          </div>
          </>
          )}
          <div className="workflow-editor-actions">
            {mode === 'form' ? (
              <Button size="sm" variant="secondary" onClick={addStep} disabled={busy}>
                {SKILLS.workflowAddStep}
              </Button>
            ) : null}
            <Button size="sm" variant="primary" onClick={() => void save()} disabled={busy}>
              {busy ? SKILLS.workflowSaving : SKILLS.workflowSave}
            </Button>
            <Button size="sm" variant="danger" onClick={() => setConfirmDelete(true)} disabled={busy}>
              {SKILLS.workflowDelete}
            </Button>
          </div>
          <div className="workflow-preview">
            <label className="workflow-args">
              <span>{SKILLS.workflowPreviewInput}</span>
              <textarea
                rows={3}
                value={previewInput}
                onChange={(e) => setPreviewInput(e.target.value)}
              />
            </label>
            <div className="workflow-editor-actions">
              <Button size="sm" variant="secondary" onClick={() => void runPreview()} disabled={previewBusy}>
                {previewBusy ? SKILLS.workflowPreviewBusy : SKILLS.workflowPreviewRun}
              </Button>
            </div>
            {previewError ? <p className="settings-error">{previewError}</p> : null}
            {preview ? (
              <ol className="workflow-preview-steps">
                {preview.steps.map((s) => (
                  <li key={s.id} className={`workflow-preview-step is-${s.status}`}>
                    <strong>{s.id}</strong>
                    <span>{s.tool}</span>
                    <span>{s.status}</span>
                    {s.error ? <span className="settings-error">{s.error}</span> : null}
                    {s.args ? <code>{JSON.stringify(s.args)}</code> : null}
                  </li>
                ))}
              </ol>
            ) : null}
          </div>
        </>
      ) : null}
      <ConfirmDialog
        open={confirmDelete}
        danger
        title={SKILLS.workflowDeleteConfirmTitle}
        body={SKILLS.workflowDeleteConfirmBody}
        confirmText={SKILLS.workflowDelete}
        busy={busy}
        error={error ?? undefined}
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() => void removeWorkflow()}
      />
    </div>
  )
}

