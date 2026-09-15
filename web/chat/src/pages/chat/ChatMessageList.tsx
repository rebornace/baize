import { CornerUpLeft, GitBranch } from 'lucide-react'
import type { RefObject } from 'react'
import type { ChatMessage } from '../../api'
import { AnalysisPagePreview } from '../../components/AnalysisPagePreview'
import { MarkdownText } from '../../components/MarkdownText'
import { ThinkingBlock, MessageThinkingFallback } from '../../components/ThinkingBlock'
import { ToolCard } from '../../components/ToolCard'
import { TypewriterText } from '../../components/TypewriterText'
import { UserBubble } from '../../components/UserBubble'
import { WorkflowCard } from '../../components/WorkflowCard'
import { DropdownMenu, type MenuItem } from '../../components/ui'
import type { ChatBlock } from '../../foldEvents'
import type { ToolCatalog } from '../../friendlyTool'
import {
  isFirstAssistantMessageOfRun,
  type ToolOrWorkflowBlock,
} from '../../historyBlocks'
import { ACTIONS, WELCOME } from '../../strings'

export type ChatMessageListProps = {
  messages: ChatMessage[]
  liveBlocks: ChatBlock[]
  liveRunId: string | null
  historyPages: Record<string, string[]>
  historyBlocks: Record<string, ToolOrWorkflowBlock[]>
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
  onGoLoginSkill: (skillId: string) => void
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
  onGoLoginSkill,
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
      aria-live="polite"
      ref={scrollerRef}
      onScroll={onScroll}
    >
      <div className="messages-inner">
        {showWelcome && (
          <div className="welcome">
            <p className="welcome-title">{WELCOME.title}</p>
            <p className="welcome-sub">{WELCOME.subtitle}</p>
          </div>
        )}
        {messages.map((m, msgIndex) => {
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
          return (
            <div key={m.id} className={`msg-row ${bubbleClass}`}>
              {runHistoryBlocks && (
                <div className="msg-history-blocks" data-testid="history-blocks">
                  {runHistoryBlocks.map((b, i) => {
                    switch (b.kind) {
                      case 'tool':
                        return (
                          <ToolCard
                            key={`h-${i}`}
                            block={b}
                            catalog={toolCatalog}
                            readOnly
                          />
                        )
                      case 'workflow':
                        return <WorkflowCard key={`h-${i}`} block={b} />
                      case 'thinking':
                        return (
                          <ThinkingBlock key={`h-${i}`} block={b} readOnly />
                        )
                      default: {
                        const _exhaustive: never = b
                        return _exhaustive
                      }
                    }
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
              </div>
              {pages.length > 0 && (
                <div className="msg-analysis-pages">
                  {pages.map((url) => (
                    <AnalysisPagePreview key={url} artifactUrl={url} />
                  ))}
                </div>
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
        {liveBlocks.map((block, i) => {
          switch (block.kind) {
            case 'assistant':
              return (
                <div key={`live-a-${i}`} className="msg-row assistant">
                  <div className="msg assistant">
                    <TypewriterText text={block.text} active />
                  </div>
                </div>
              )
            case 'thinking':
              return (
                <div key={`live-th-${block.turn}-${i}`} className="msg-row tool">
                  <ThinkingBlock block={block} />
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
            case 'tool':
              return (
                <div key={`live-t-${i}`} className="msg-row tool">
                  <ToolCard
                    block={block}
                    catalog={toolCatalog}
                    onError={reportError}
                    onGoLoginSkill={onGoLoginSkill}
                  />
                </div>
              )
            case 'workflow':
              return (
                <div key={`live-w-${i}`} className="msg-row tool">
                  <WorkflowCard block={block} />
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
