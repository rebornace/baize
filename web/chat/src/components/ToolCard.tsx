import { useEffect, useState } from 'react'
import { parseAnalysisPageResult } from '../analysisPage'
import { resumeRun } from '../api'
import { AnalysisPagePreview } from './AnalysisPagePreview'
import type { ChatBlock } from '../foldEvents'

type ToolBlock = Extract<ChatBlock, { kind: 'tool' }>

const STATUS_LABEL: Record<ToolBlock['status'], string> = {
  running: '执行中',
  waiting_human: '等待批准',
  succeeded: '成功',
  failed: '失败',
  approved: '已批准',
  rejected: '已拒绝',
}

export interface ToolCardProps {
  block: ToolBlock
  onResumed?: () => void
}

export function ToolCard({ block, onResumed }: ToolCardProps) {
  const [expanded, setExpanded] = useState(block.status === 'waiting_human')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const waiting = block.status === 'waiting_human'
  const analysisPage =
    block.result !== undefined ? parseAnalysisPageResult(block.result) : null

  // running → waiting_human: auto-expand arguments (user may still collapse).
  useEffect(() => {
    if (block.status === 'waiting_human') {
      setExpanded(true)
    }
  }, [block.status])

  const decide = async (decision: 'approve' | 'reject') => {
    setBusy(true)
    setError(null)
    try {
      await resumeRun(block.runId, decision)
      onResumed?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={`tool-card${waiting ? ' tool-card-waiting' : ''}`}>
      <button
        type="button"
        className="tool-card-header"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
      >
        <span className="tool-card-name">{block.name}</span>
        <span className="tool-card-status">{STATUS_LABEL[block.status]}</span>
      </button>

      {expanded && (
        <div className="tool-card-body">
          {block.arguments !== undefined && (
            <pre className="tool-card-json">{formatJSON(block.arguments)}</pre>
          )}
          {analysisPage && <AnalysisPagePreview artifactUrl={analysisPage.artifactUrl} />}
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
          <button
            type="button"
            className="btn primary sm"
            disabled={busy}
            onClick={() => void decide('approve')}
          >
            批准
          </button>
          <button
            type="button"
            className="btn danger sm"
            disabled={busy}
            onClick={() => void decide('reject')}
          >
            驳回
          </button>
        </div>
      )}

      {error && <p className="tool-card-error">{error}</p>}
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
