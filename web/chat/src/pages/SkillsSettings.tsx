import { useCallback, useEffect, useRef, useState } from 'react'
import {
  deleteSkill,
  getAgent,
  getUIConfig,
  listSkills,
  putAgent,
  uploadSkill,
  type SkillSummary,
} from '../api'

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

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
      return '内置'
    case 'user':
      return '用户'
    default: {
      const _exhaustive: never = source
      return _exhaustive
    }
  }
}

function toolsSummary(tools: string[]): string {
  if (tools.length === 0) return '—'
  return tools.join(', ')
}

export function SkillsSettings() {
  const [skills, setSkills] = useState<SkillSummary[] | null>(null)
  const [agentId, setAgentId] = useState('ticket-agent')
  const [system, setSystem] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)
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

      const [{ skills: list }, agent] = await Promise.all([
        listSkills(),
        getAgent(resolvedAgentId),
      ])
      setSkills(list ?? [])
      setSystem(agent.system ?? '')
      setSelected(new Set(agent.skills ?? []))
      setError(null)
    } catch (err) {
      setSkills(null)
      setError(errorMessage(err))
    }
  }, [])

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
    setStatus(null)
    try {
      await uploadSkill(file)
      await refreshList()
      setError(null)
      setStatus(`已上传 ${file.name}`)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  const onDelete = async (id: string) => {
    setDeleting(id)
    setStatus(null)
    try {
      await deleteSkill(id)
      setSelected((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
      await refreshList()
      setError(null)
      setStatus(`已删除 ${id}`)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setDeleting(null)
    }
  }

  const onSave = async () => {
    if (skills == null) return
    setSaving(true)
    setStatus(null)
    try {
      const agent = await getAgent(agentId)
      const catalogOrder = skills.map((s) => s.id)
      const skillsIds = mergeSkillSelection(agent.skills ?? [], selected, catalogOrder)
      await putAgent(agentId, { system: agent.system ?? system, skills: skillsIds })
      setSystem(agent.system ?? system)
      setError(null)
      setStatus('默认 Skills 已保存')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const loadFailed = skills === null && error !== null
  const busy = uploading || saving || deleting !== null

  return (
    <div className="settings-section settings-skills">
      <h1 className="settings-heading">Skills</h1>
      <p className="settings-meta">默认 Agent：{agentId}</p>
      {loadFailed && <p className="settings-error">无法加载 Skills：{error}</p>}
      {!loadFailed && error && <p className="settings-error">{error}</p>}
      {!loadFailed && status && <p className="settings-muted">{status}</p>}
      {skills === null && !loadFailed && <p className="settings-muted">加载中…</p>}
      {skills !== null && (
        <>
          <div className="settings-toolbar">
            <input
              ref={fileRef}
              type="file"
              accept=".md,.zip"
              disabled={busy}
              aria-label="上传 Skill"
              onChange={(e) => {
                void onUpload(e.target.files?.[0])
              }}
            />
            <button
              type="button"
              className="btn primary sm"
              disabled={busy}
              onClick={() => {
                void onSave()
              }}
            >
              {saving ? '保存中…' : '保存默认勾选'}
            </button>
          </div>
          {skills.length === 0 && <p className="settings-empty">尚未安装 Skill</p>}
          {skills.length > 0 && (
            <ul className="settings-list">
              {skills.map((s) => {
                const isDeleting = deleting === s.id
                return (
                  <li key={s.id} className="settings-list-item settings-skill-row">
                    <label className="settings-login-toggle settings-skill-check">
                      <input
                        type="checkbox"
                        checked={selected.has(s.id)}
                        disabled={busy}
                        onChange={(e) => {
                          setSelected((prev) => toggleSkillSelection(prev, s.id, e.target.checked))
                        }}
                      />
                      <span className="settings-skill-line">
                        <span className="settings-tool-title">{s.id}</span>
                        {s.description ? (
                          <span className="settings-tool-desc">{s.description}</span>
                        ) : null}
                        <span className="settings-tool-sub">{toolsSummary(s.tools)}</span>
                      </span>
                    </label>
                    <span className="settings-tool-actions">
                      <span className="settings-badge">{sourceLabel(s.source)}</span>
                      {s.source === 'user' && (
                        <button
                          type="button"
                          className="btn danger sm"
                          disabled={busy}
                          onClick={() => {
                            void onDelete(s.id)
                          }}
                        >
                          {isDeleting ? '删除中…' : '删除'}
                        </button>
                      )}
                    </span>
                  </li>
                )
              })}
            </ul>
          )}
        </>
      )}
    </div>
  )
}
