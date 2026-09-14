import { useEffect, useState } from 'react'
import { Check, ChevronDown, Loader2, X } from 'lucide-react'
import { parseAnalysisPageResult } from '../analysisPage'
import { resumeRun } from '../api'
import { friendlyToolName, toolPhrase, type ToolCatalog } from '../friendlyTool'
import type { ChatBlock } from '../foldEvents'
import { isLoginRequiredContent, loginSkillID, resolveConnectorId } from '../loginEntry'
import { HITL, LOGIN_AT } from '../strings'
import { AnalysisPagePreview } from './AnalysisPagePreview'
import { Button } from './ui'

type ToolBlock = Extract<ChatBlock, { kind: 'tool' }>

export interface ToolCardProps {
  block: ToolBlock
  catalog?: ToolCatalog
  /** 历史回看：只渲染结果态，不出现审批操作。 */
  readOnly?: boolean
  /** 决议失败时回调父级弹 toast；卡片自身只显通用提示。 */
  onError?: (e: unknown) => void
  /** login_required 时「去登录」；传入 `login-<connector_id>` skill id。 */
  onGoLoginSkill?: (skillId: string) => void
}

export function ToolCard({
  block,
  catalog = [],
  readOnly = false,
  onError,
  onGoLoginSkill,
}: ToolCardProps) {
  const waiting = block.status === 'waiting_human' && !readOnly
  const loginRequired = !readOnly && isLoginRequiredContent(block.result)
  const [expanded, setExpanded] = useState(waiting || loginRequired)
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const [showComment, setShowComment] = useState(false)
  const [comment, setComment] = useState('')

  const analysisPage =
    block.result !== undefined ? parseAnalysisPageResult(block.result) : null
  const label = friendlyToolName(block.name, catalog)
  const description = catalog.find((t) => t.name === block.name)?.description?.trim() || ''
  const loginMessage =
    loginRequired &&
    block.result != null &&
    typeof block.result === 'object' &&
    typeof (block.result as { message?: unknown }).message === 'string'
      ? String((block.result as { message: string }).message).trim()
      : ''

  // running → waiting_human: auto-expand arguments (user may still collapse).
  useEffect(() => {
    if (block.status === 'waiting_human' && !readOnly) setExpanded(true)
  }, [block.status, readOnly])

  const decide = async (decision: 'approve' | 'reject') => {
    setBusy(true)
    setFailed(false)
    try {
      await resumeRun(block.runId, decision, decision === 'reject' ? comment.trim() : '')
    } catch (err) {
      // 技术细节交给父级 toast（friendlyError），卡片只保留通用提示。
      setFailed(true)
      onError?.(err)
    } finally {
      setBusy(false)
    }
  }

  const goLoginSkill = () => {
    const connectorId = resolveConnectorId(
      catalog.find((t) => t.name === block.name)?.connector_id,
    )
    if (!connectorId) return
    const skillId = loginSkillID(connectorId)
    if (!skillId) return
    onGoLoginSkill?.(skillId)
  }

  const icon =
    block.status === 'running' || block.status === 'approved' ? (
      <Loader2 size={15} className="icon-spin" aria-hidden />
    ) : block.status === 'failed' || block.status === 'rejected' ? (
      <X size={15} aria-hidden />
    ) : (
      <Check size={15} aria-hidden />
    )

  return (
    <div className={`tool-card${waiting ? ' tool-card-waiting' : ''}`}>
      {waiting ? (
        <div className="hitl-head">
          <p className="hitl-title">{HITL.title}</p>
          <p className="hitl-desc">
            {label}
            {description ? ` · ${description}` : ''}
          </p>
        </div>
      ) : (
        <button
          type="button"
          className="tool-card-header"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
        >
          <span className="tool-card-phrase">
            {icon}
            {toolPhrase(block.name, block.status, catalog)}
          </span>
          <ChevronDown
            size={14}
            className={`tool-card-chevron${expanded ? ' expanded' : ''}`}
            aria-hidden
          />
        </button>
      )}

      {/* Analysis pages stay visible even when the tool card is collapsed. */}
      {analysisPage && (
        <div className="tool-card-preview">
          <AnalysisPagePreview artifactUrl={analysisPage.artifactUrl} />
        </div>
      )}

      {expanded && (
        <div className="tool-card-body">
          <p className="tool-card-techname">工具：{block.name}</p>
          {block.arguments !== undefined && (
            <pre className="tool-card-json">{formatJSON(block.arguments)}</pre>
          )}
          {block.result !== undefined &&
            (analysisPage ? (
              <details className="tool-card-details">
                <summary>详情</summary>
                <pre className="tool-card-json">{formatJSON(block.result)}</pre>
              </details>
            ) : (
              <pre className="tool-card-json">{formatJSON(block.result)}</pre>
            ))}
        </div>
      )}

      {waiting && (
        <div className="tool-card-actions">
          {showComment && (
            <input
              className="hitl-comment"
              type="text"
              value={comment}
              disabled={busy}
              placeholder={HITL.commentPlaceholder}
              onChange={(e) => setComment(e.target.value)}
            />
          )}
          <Button size="sm" variant="primary" disabled={busy} onClick={() => void decide('approve')}>
            {HITL.approve}
          </Button>
          {!showComment ? (
            <Button size="sm" variant="danger" disabled={busy} onClick={() => setShowComment(true)}>
              {HITL.reject}
            </Button>
          ) : (
            <Button size="sm" variant="danger" disabled={busy} onClick={() => void decide('reject')}>
              {HITL.confirmReject}
            </Button>
          )}
          <button
            type="button"
            className="tool-card-detailbtn"
            disabled={busy}
            onClick={() => setExpanded((v) => !v)}
          >
            {HITL.viewParams}
          </button>
        </div>
      )}

      {loginRequired && (
        <div className="tool-card-actions tool-card-login-actions">
          {loginMessage ? <p className="tool-card-login-msg">{loginMessage}</p> : null}
          <Button size="sm" variant="primary" onClick={goLoginSkill}>
            {LOGIN_AT.goLogin}
          </Button>
        </div>
      )}

      {failed && <p className="tool-card-error">{HITL.failed}</p>}
    </div>
  )
}

function formatJSON(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}
