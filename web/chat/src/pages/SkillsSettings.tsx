import { useCallback, useEffect, useRef, useState } from 'react'
import { Sparkles } from 'lucide-react'
import {
  deleteSkill,
  getAgent,
  getUIConfig,
  listSkills,
  putAgent,
  uploadSkill,
  type SkillSummary,
} from '../api'
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  PageHeader,
  ToastRegion,
  useToast,
} from '../components/ui'
import { useGate } from '../gateContext'
import { SKILLS, skillErrorText } from '../strings'

export function toggleSkillSelection(prev: Set<string>, id: string, checked: boolean): Set<string> {
  const next = new Set(prev)
  if (checked) next.add(id)
  else next.delete(id)
  return next
}

/** Preserve previous Agent.skills order for still-checked ids, then append new picks in catalog order. */
export function mergeSkillSelection(
  previous: string[],
  selected: Set<string>,
  catalogOrder: string[],
): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const id of previous) {
    if (!selected.has(id) || seen.has(id)) continue
    out.push(id)
    seen.add(id)
  }
  for (const id of catalogOrder) {
    if (!selected.has(id) || seen.has(id)) continue
    out.push(id)
    seen.add(id)
  }
  return out
}

function sourceLabel(source: SkillSummary['source']): string {
  switch (source) {
    case 'builtin':
      return SKILLS.sourceBuiltin
    case 'user':
      return SKILLS.sourceUser
    default: {
      const _exhaustive: never = source
      return _exhaustive
    }
  }
}

function skillDisplayName(s: SkillSummary): string {
  const desc = s.description?.trim()
  if (desc) return desc
  const name = s.name?.trim()
  if (name) return name
  return s.id
}

function toolsSummary(tools: string[]): string {
  if (tools.length === 0) return '—'
  return tools.join(', ')
}

export function SkillsSettings() {
  const { role } = useGate()
  const readOnly = role !== 'admin'
  const { toasts, push, dismiss } = useToast()
  const [skills, setSkills] = useState<SkillSummary[] | null>(null)
  const [agentId, setAgentId] = useState('ticket-agent')
  const [system, setSystem] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [loadError, setLoadError] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<SkillSummary | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const load = useCallback(async () => {
    try {
      let resolvedAgentId = 'ticket-agent'
      try {
        const cfg = await getUIConfig()
        if (cfg.agent_id?.trim()) resolvedAgentId = cfg.agent_id.trim()
      } catch {
        /* fallback ticket-agent */
      }
      setAgentId(resolvedAgentId)

      // Skill list is the read-only core payload; its success must not depend on getAgent.
      const { skills: list } = await listSkills()
      setSkills(list ?? [])
      setLoadError(null)

      // Agent config (system prompt + saved selection) is admin-only (GET /v0/agents/{id}
      // returns 403 for operators). Skip it entirely on the read-only path; operators see
      // an unselected, disabled list. For admins, a failure here is non-fatal: the list is
      // already rendered and selection simply stays empty.
      if (readOnly) return
      try {
        const agent = await getAgent(resolvedAgentId)
        setSystem(agent.system ?? '')
        setSelected(new Set(agent.skills ?? []))
      } catch {
        /* non-fatal: skill list remains visible without saved selection/system */
      }
    } catch (err) {
      setSkills(null)
      const f = skillErrorText(err)
      setLoadError(f.detail ? `${f.title} ${f.detail}` : f.title)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }, [readOnly, push])

  useEffect(() => {
    void load()
  }, [load])

  const refreshList = async () => {
    const { skills: list } = await listSkills()
    setSkills(list ?? [])
  }

  const onUpload = async (file: File | undefined) => {
    if (!file) return
    setUploading(true)
    try {
      await uploadSkill(file)
      await refreshList()
      push({ tone: 'success', title: SKILLS.toastUploaded, detail: file.name })
    } catch (err) {
      const f = skillErrorText(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  const beginDelete = (s: SkillSummary) => {
    setDeleteError(null)
    setPendingDelete(s)
  }

  const cancelDelete = () => {
    if (deleting) return
    setPendingDelete(null)
    setDeleteError(null)
  }

  const confirmDelete = async () => {
    if (!pendingDelete) return
    setDeleting(true)
    setDeleteError(null)
    try {
      await deleteSkill(pendingDelete.id)
      setSelected((prev) => {
        const next = new Set(prev)
        next.delete(pendingDelete.id)
        return next
      })
      push({ tone: 'success', title: SKILLS.toastDeleted, detail: skillDisplayName(pendingDelete) })
      setPendingDelete(null)
      await refreshList()
    } catch (err) {
      const f = skillErrorText(err)
      setDeleteError(f.detail ? `${f.title} ${f.detail}` : f.title)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setDeleting(false)
    }
  }

  const onSave = async () => {
    if (skills == null) return
    setSaving(true)
    try {
      const agent = await getAgent(agentId)
      const catalogOrder = skills.map((s) => s.id)
      const skillsIds = mergeSkillSelection(agent.skills ?? [], selected, catalogOrder)
      await putAgent(agentId, { system: agent.system ?? system, skills: skillsIds })
      setSystem(agent.system ?? system)
      push({ tone: 'success', title: SKILLS.toastSaved })
    } catch (err) {
      const f = skillErrorText(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setSaving(false)
    }
  }

  const loadFailed = skills === null && loadError !== null
  const busy = uploading || saving || deleting
  const showEmpty = skills !== null && skills.length === 0 && !loadFailed

  const headerActions =
    readOnly || showEmpty || skills === null ? undefined : (
      <div className="settings-toolbar">
        <input
          ref={fileRef}
          type="file"
          accept=".md,.zip"
          hidden
          disabled={busy}
          aria-label={SKILLS.upload}
          onChange={(e) => {
            void onUpload(e.target.files?.[0])
          }}
        />
        <Button
          variant="secondary"
          size="sm"
          disabled={busy}
          onClick={() => fileRef.current?.click()}
        >
          {SKILLS.upload}
        </Button>
        <Button
          variant="primary"
          size="sm"
          disabled={busy}
          onClick={() => {
            void onSave()
          }}
        >
          {saving ? '保存中…' : SKILLS.saveDefaults}
        </Button>
      </div>
    )

  return (
    <div className="settings-panel settings-skills">
      <PageHeader title={SKILLS.title} description={SKILLS.description} actions={headerActions} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {loadFailed && <p className="settings-error">{loadError}</p>}
      {skills === null && !loadFailed && <p className="settings-muted">加载中…</p>}

      {showEmpty && (
        <EmptyState
          icon={<Sparkles size={28} aria-hidden="true" />}
          title={SKILLS.emptyTitle}
          description={SKILLS.emptyDesc}
          action={
            readOnly ? undefined : (
              <>
                <input
                  ref={fileRef}
                  type="file"
                  accept=".md,.zip"
                  disabled={busy}
                  aria-label={SKILLS.upload}
                  hidden
                  onChange={(e) => {
                    void onUpload(e.target.files?.[0])
                  }}
                />
                <Button
                  variant="primary"
                  disabled={busy}
                  onClick={() => fileRef.current?.click()}
                >
                  {SKILLS.upload}
                </Button>
              </>
            )
          }
        />
      )}

      {skills !== null && skills.length > 0 && (
        <ul className="settings-list">
          {skills.map((s) => (
            <li key={s.id} className="settings-list-item settings-skill-row">
              <label className="settings-login-toggle settings-skill-check">
                <input
                  type="checkbox"
                  checked={selected.has(s.id)}
                  disabled={busy || readOnly}
                  onChange={(e) => {
                    setSelected((prev) => toggleSkillSelection(prev, s.id, e.target.checked))
                  }}
                />
                <span className="settings-skill-line">
                  <span className="settings-tool-title">{skillDisplayName(s)}</span>
                  {s.description?.trim() && s.id !== skillDisplayName(s) ? (
                    <span className="settings-tool-sub">{s.id}</span>
                  ) : null}
                  <span className="settings-tool-sub">{toolsSummary(s.tools)}</span>
                </span>
              </label>
              <span className="settings-tool-actions">
                <Badge tone={s.source === 'builtin' ? 'info' : 'neutral'}>{sourceLabel(s.source)}</Badge>
                {s.source === 'user' && !readOnly && (
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={busy}
                    onClick={() => beginDelete(s)}
                  >
                    {SKILLS.confirmDeleteOk}
                  </Button>
                )}
              </span>
            </li>
          ))}
        </ul>
      )}

      {!readOnly && (
        <ConfirmDialog
          open={!!pendingDelete}
          danger
          title={SKILLS.confirmDeleteTitle}
          body={SKILLS.confirmDeleteBody}
          confirmText={SKILLS.confirmDeleteOk}
          busy={deleting}
          error={deleteError}
          onCancel={cancelDelete}
          onConfirm={() => void confirmDelete()}
        />
      )}
    </div>
  )
}
