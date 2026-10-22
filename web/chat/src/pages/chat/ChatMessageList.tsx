import { CornerUpLeft, GitBranch } from 'lucide-react'
import type { ReactNode, RefObject } from 'react'
import type { ChatMessage } from '../../api'
import {
  partitionHistoryActivity,
  partitionLiveActivity,
  type ActivityStepBlock,
} from '../../activitySteps'
import { ActivitySteps } from '../../components/ActivitySteps'
import { AnalysisPagePreview } from '../../components/AnalysisPagePreview'
import { MarkdownText } from '../../components/MarkdownText'
import { ThinkingBlock, MessageThinkingFallback } from '../../components/ThinkingBlock'
import { ToolCard } from '../../components/ToolCard'
import { TypewriterText } from '../../components/TypewriterText'
import { RunBindingButton } from '../../components/RunBindingButton'
import { UsageMetaLine } from '../../components/UsageMetaLine'
import { UserBubble } from '../../components/UserBubble'
import { WorkflowCard } from '../../components/WorkflowCard'
import { DropdownMenu, type MenuItem } from '../../components/ui'
import type { ChatBlock, UsageMeta } from '../../foldEvents'
import type { ToolCatalog } from '../../friendlyTool'
import { isFirstAssistantMessageOfRun, type ToolOrWorkflowBlock } from '../../historyBlocks'
import { liveAssistantKey, messageRowKey } from '../../liveCommit'
import { collectPriorEvidence } from '../../priorEvidence'
import { ACTIONS, WELCOME } from '../../strings'

function renderActivityStepBlock(
  b: ActivityStepBlock,
  key: string,
  opts: {
    catalog: ToolCatalog
    reportError: (e: unknown) => void
    onBrowserLogin?: (connectorId: string) => void
    suppressLoginActions: boolean
    cloudModelRisk: boolean
    readOnly?: boolean
    priorEvidence?: ReturnType<typeof collectPriorEvidence>
  },
): ReactNode {
  if (b.kind === 'workflow') {
    return <WorkflowCard key={key} block={b} />
  }
  return (
    <ToolCard
      key={key}
      block={b}
      catalog={opts.catalog}
      readOnly={opts.readOnly}
      onError={opts.reportError}
      onBrowserLogin={opts.onBrowserLogin}
      suppressLoginActions={opts.suppressLoginActions}
      cloudModelRisk={opts.cloudModelRisk}
      priorEvidence={opts.priorEvidence}
    />
  )
}

export type ChatMessageListProps = {
  messages: ChatMessage[]
  liveBlocks: ChatBlock[]
  liveRunId: string | null
  historyPages: Record<string, string[]>
  historyBlocks: Record<string, ToolOrWorkflowBlock[]>
  historyUsage: Record<string, UsageMeta>
  toolCatalog: ToolCatalog
  busy: boolean
  historyMutating: boolean
  showWelcome: boolean
  scrollerRef: RefObject<HTMLDivElement | null>
  bottomRef: RefObject<HTMLDivElement | null>
  onScroll: () => void
  onRollbackUser: (m: ChatMessage) => void
  onRegenerate: (m: ChatMessage) => void
  onRollbackTo: (m: ChatMessage) => void
  onFork: (m: ChatMessage) => void
  onCopyMessage: (m: ChatMessage) => void
  reportError: (e: unknown) => void
  onBrowserLogin?: (connectorId: string) => void
  /** Footer banner owns login CTAs; cards only show the message. */
  suppressLoginActions?: boolean
  cloudModelRisk?: boolean
}

// Copy with a legacy fallback for non-secure (HTTP/LAN) contexts without
// navigator.clipboard. The hidden textarea is removed synchronously so it
// never disturbs page focus.
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    /* fall through to legacy path */
  }
  const ta = document.createElement('textarea')
  try {
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    // Removed even if select()/execCommand() throws so no hidden node lingers.
    ta.remove()
  }
}

export function ChatMessageList({
  messages,
  liveBlocks,
  liveRunId,
  historyPages,
  historyBlocks,
  historyUsage,
  toolCatalog,
  busy,
  historyMutating,
  showWelcome,
  scrollerRef,
  bottomRef,
  onScroll,
  onRollbackUser,
  onRegenerate,
  onRollbackTo,
  onFork,
  onCopyMessage,
  reportError,
  onBrowserLogin,
  suppressLoginActions = false,
  cloudModelRisk = false,
}: ChatMessageListProps) {
  // Low-frequency actions live in the "more" menu. system_note additionally
  // exposes "roll back to here" at the top; it never gets a copy action.
  const buildMessageMenuItems = (m: ChatMessage): MenuItem[] => {
    const items: MenuItem[] = []
    if (m.role === 'system_note') {
      items.push({
        id: 'rollback-here',
        label: ACTIONS.rollbackHere,
        icon: <CornerUpLeft size={15} aria-hidden />,
        onSelect: () => void onRollbackTo(m),
      })
    }
    items.push({
      id: 'fork',
      label: ACTIONS.forkAsNew,
      icon: <GitBranch size={15} aria-hidden />,
      onSelect: () => void onFork(m),
    })
    return items
  }

  return (
    <div
      className="messages"
      aria-busy={busy || Boolean(liveRunId)}
      ref={scrollerRef}
      onScroll={onScroll}
    >
      <div className="messages-inner">
        {showWelcome && (
          <div className="welcome">
            <span className="welcome-mark" aria-hidden="true" />
            <p className="welcome-brand">{WELCOME.brand}</p>
            <p className="welcome-title">{WELCOME.title}</p>
            <p className="welcome-sub">{WELCOME.subtitle}</p>
          </div>
        )}
        {messages.map((m, msgIndex) => {
          if (liveRunId && m.role === 'assistant' && m.run_id === liveRunId) {
            return null
          }
          const bubbleClass =
            m.role === 'user' ? 'user' : m.role === 'system_note' ? 'system' : 'assistant'
          const persisted = !m.id.startsWith('local_')
          const runHistoryBlocks =
            m.role === 'assistant' &&
            m.run_id &&
            isFirstAssistantMessageOfRun(msgIndex, messages) &&
            historyBlocks[m.run_id]
          const hasEventThinking = Boolean(
            m.run_id && historyBlocks[m.run_id]?.some((b) => b.kind === 'thinking'),
          )
          const showMessageThinking =
            m.role === 'assistant' &&
            !hasEventThinking &&
            (Boolean(m.thinking?.trim()) || Boolean(m.thinking_redacted)) &&
            (!m.run_id || isFirstAssistantMessageOfRun(msgIndex, messages))
          // 分析页产物源自工具结果：该 run 只要有历史工具块（统一在首条
          // assistant 消息处渲染），所有 assistant 消息都不再独立出预览，
          // 避免同一 run 多条 assistant 消息时重复 iframe。
          const hasHistoryToolBlocks = Boolean(
            m.run_id &&
              historyBlocks[m.run_id]?.some(
                (b) => b.kind === 'tool' || b.kind === 'workflow',
              ),
          )
          const pages =
            m.role === 'assistant' &&
            m.run_id &&
            m.run_id !== liveRunId &&
            !hasHistoryToolBlocks
              ? historyPages[m.run_id] ?? []
              : []
          const canAct = persisted && !busy && !liveRunId && !historyMutating
          const isTurnAnchor =
            m.role === 'user' &&
            !messages.slice(msgIndex + 1).some((row) => row.role === 'user')
          return (
            <div
              key={messageRowKey(m, msgIndex, messages)}
              className={`msg-row ${bubbleClass}`}
              data-turn-anchor={isTurnAnchor ? '' : undefined}
            >
              {runHistoryBlocks && (
                <div className="msg-history-blocks" data-testid="history-blocks">
                  {partitionHistoryActivity(runHistoryBlocks).map((seg, segIdx) => {
                    if (seg.type === 'thinking') {
                      return (
                        <ThinkingBlock
                          key={`h-th-${seg.block.turn}`}
                          block={seg.block}
                          readOnly
                        />
                      )
                    }
                    const cards = seg.items.map((it, i) => {
                      if (it.kind === 'thinking') {
                        return (
                          <ThinkingBlock
                            key={`h-${segIdx}-th-${it.block.turn}`}
                            block={it.block}
                            readOnly
                          />
                        )
                      }
                      return renderActivityStepBlock(it.block, `h-${segIdx}-${i}`, {
                        catalog: toolCatalog,
                        reportError,
                        onBrowserLogin,
                        suppressLoginActions,
                        cloudModelRisk,
                        readOnly: true,
                      })
                    })
                    return (
                      <ActivitySteps
                        key={`h-steps-${segIdx}`}
                        blocks={seg.stepBlocks}
                        catalog={toolCatalog}
                      >
                        {cards}
                      </ActivitySteps>
                    )
                  })}
                </div>
              )}
              <div className={`msg ${bubbleClass}`}>
                {m.role === 'assistant' ? (
                  <>
                    {showMessageThinking && (
                      <MessageThinkingFallback
                        thinking={m.thinking}
                        redacted={m.thinking_redacted}
                      />
                    )}
                    <MarkdownText text={m.content} />
                  </>
                ) : (
                  <UserBubble content={m.content} />
                )}
                {m.role === 'user' && m.pinned_skills && m.pinned_skills.length > 0 && (
                  <div className="msg-pinned-skills" data-testid="pinned-skills">
                    {m.pinned_skills.map((id) => (
                      <span key={id} className="msg-pinned-skill">
                        {id}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              {pages.length > 0 && (
                <div className="msg-analysis-pages">
                  {pages.map((url) => (
                    <AnalysisPagePreview key={url} artifactUrl={url} />
                  ))}
                </div>
              )}
              {m.role === 'assistant' &&
                m.run_id &&
                isFirstAssistantMessageOfRun(msgIndex, messages) &&
                historyUsage[m.run_id] && (
                  <UsageMetaLine usage={historyUsage[m.run_id]} />
                )}
              {canAct && (
                <div className="msg-actions">
                  {m.role === 'user' && (
                    <>
                      <button type="button" className="btn ghost sm" onClick={() => void onRollbackUser(m)}>
                        {ACTIONS.editAndReanswer}
                      </button>
                      <button type="button" className="btn ghost sm" onClick={() => onCopyMessage(m)}>
                        {ACTIONS.copy}
                      </button>
                    </>
                  )}
                  {m.role === 'assistant' && (
                    <>
                      <button type="button" className="btn ghost sm" onClick={() => void onRegenerate(m)}>
                        {ACTIONS.regenerate}
                      </button>
                      <button type="button" className="btn ghost sm" onClick={() => onCopyMessage(m)}>
                        {ACTIONS.copy}
                      </button>
                      {m.run_id && isFirstAssistantMessageOfRun(msgIndex, messages) ? (
                        <RunBindingButton runId={m.run_id} />
                      ) : null}
                    </>
                  )}
                  <DropdownMenu
                    triggerLabel={ACTIONS.more}
                    items={buildMessageMenuItems(m)}
                  />
                </div>
              )}
            </div>
          )
        })}
        {partitionLiveActivity(liveBlocks).map((seg) => {
          if (seg.type === 'steps') {
            const cards = seg.items.map((it) => {
              if (it.kind === 'thinking') {
                return (
                  <ThinkingBlock key={`live-th-${it.block.turn}`} block={it.block} />
                )
              }
              const block = it.block
              const i = it.index
              return renderActivityStepBlock(block, `live-step-${i}`, {
                catalog: toolCatalog,
                reportError,
                onBrowserLogin,
                suppressLoginActions,
                cloudModelRisk,
                priorEvidence:
                  block.kind === 'tool' && block.status === 'waiting_human'
                    ? collectPriorEvidence(liveBlocks, i)
                    : undefined,
              })
            })
            const firstIdx = seg.items[0]?.index ?? 0
            return (
              <div key={`live-steps-${firstIdx}`} className="msg-row tool">
                <ActivitySteps blocks={seg.stepBlocks} catalog={toolCatalog}>
                  {cards}
                </ActivitySteps>
              </div>
            )
          }
          if (seg.type === 'thinking') {
            return (
              <div key={`live-th-${seg.block.turn}`} className="msg-row tool">
                <ThinkingBlock block={seg.block} />
              </div>
            )
          }
          const block = seg.block
          const i = seg.index
          switch (block.kind) {
            case 'assistant':
              return (
                <div
                  key={liveRunId ? liveAssistantKey(liveRunId, liveBlocks, i) : `live-a-${i}`}
                  className="msg-row assistant"
                >
                  <div className="msg assistant">
                    <TypewriterText text={block.text} active={Boolean(block.streaming)} />
                  </div>
                  {block.usage && <UsageMetaLine usage={block.usage} />}
                  {block.usage && liveRunId ? <RunBindingButton runId={liveRunId} /> : null}
                </div>
              )
            case 'system':
              return (
                <div key={`live-s-${i}`} className="msg-row system">
                  <div className="msg system">
                    <MarkdownText text={block.text} plain />
                  </div>
                </div>
              )
            case 'user':
              return (
                <div key={`live-u-${i}`} className="msg-row user">
                  <div className="msg user">
                    <MarkdownText text={block.text} plain />
                  </div>
                </div>
              )
            default: {
              const _exhaustive: never = block
              return _exhaustive
            }
          }
        })}
        <div ref={bottomRef} className="messages-end" aria-hidden />
      </div>
    </div>
  )
}
