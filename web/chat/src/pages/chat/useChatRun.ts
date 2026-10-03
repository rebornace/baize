import {
  useEffect,
  useRef,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from 'react'
import {
  cancelRun,
  createRun,
  fileToAttachment,
  isImageAttachment,
  listMessages,
  type Attachment,
  type ChatMessage,
  type ModelProfile,
  type ThinkingLevel,
} from '../../api'
import { buildLocalPreview } from '../../localAttachments'
import { buildRunOptions, visionGate } from '../../modelSelect'
import { CHAT } from '../../strings'
import type { ChatLiveControls } from './useChatSession'
import type { UsageMeta } from '../../foldEvents'
import type { ToolOrWorkflowBlock } from '../../historyBlocks'
import { useChatRunStream } from './useChatRunStream'

const IDLE_SYNC_MS = 2000

export type UseChatRunArgs = {
  role: string
  agentId: string
  conversationId: string
  conversationIdRef: MutableRefObject<string>
  setMessages: Dispatch<SetStateAction<ChatMessage[]>>
  setHistoryPages: Dispatch<SetStateAction<Record<string, string[]>>>
  setHistoryBlocks: Dispatch<SetStateAction<Record<string, ToolOrWorkflowBlock[]>>>
  setHistoryUsage: Dispatch<SetStateAction<Record<string, UsageMeta>>>
  fetchedRunsRef: MutableRefObject<Set<string>>
  mergeHistoryPages: (runId: string, urls: string[]) => void
  refreshConversations: () => Promise<void>
  setComposerDraft: Dispatch<SetStateAction<string | undefined>>
  modelProfiles: ModelProfile[]
  selectedModelId: string
  thinkingLevel: string
  supportsVision: boolean
  scrollToBottom: (behavior?: ScrollBehavior) => void
  reportError: (e: unknown) => void
  pushToast: (t: { tone: 'success' | 'error' | 'info'; title: string; detail?: string }) => void
  liveRef: MutableRefObject<ChatLiveControls>
  setVisionWarning: (msg: string | null) => void
  workspaceId: string
}

export function useChatRun({
  role,
  agentId,
  conversationId,
  conversationIdRef,
  setMessages,
  setHistoryPages,
  setHistoryBlocks,
  setHistoryUsage,
  fetchedRunsRef,
  mergeHistoryPages,
  refreshConversations,
  setComposerDraft,
  modelProfiles,
  selectedModelId,
  thinkingLevel,
  supportsVision,
  scrollToBottom,
  reportError,
  pushToast,
  liveRef,
  setVisionWarning,
  workspaceId,
}: UseChatRunArgs) {
  const stream = useChatRunStream({
    conversationIdRef,
    setMessages,
    mergeHistoryPages,
    setHistoryBlocks,
    setHistoryUsage,
    refreshConversations,
  })

  const {
    liveEvents,
    liveRunId,
    setLiveRunId,
    busy,
    setBusy,
    status,
    setStatus,
    lastEventIndexRef,
    liveRunIdRef,
    writeLiveEvents,
    stopPoll,
    stopStream,
    resetLive,
    finishLiveRun,
    attachRun,
    restoreLiveRun,
  } = stream

  // Keep liveRef in sync every render so session callbacks see latest controls.
  liveRef.current = {
    busy,
    liveRunId,
    setBusy,
    stopStream,
    stopPoll,
    resetLive,
    attachRun,
  }

  // Reload transcript only when the open conversation changes. Restoring a
  // live run must not sit in this effect's deps: a new callback identity would
  // clear the live fold and leave just the user bubble until listMessages
  // returns.
  const restoreLiveRunRef = useRef(restoreLiveRun)
  restoreLiveRunRef.current = restoreLiveRun

  useEffect(() => {
    let cancelled = false
    stopStream()
    stopPoll()
    resetLive()
    setHistoryPages({})
    setHistoryBlocks({})
    setHistoryUsage({})
    fetchedRunsRef.current = new Set()

    const id = conversationId
    void (async () => {
      try {
        const msgs = await listMessages(id)
        if (cancelled || conversationIdRef.current !== id) return
        setMessages(msgs)
        requestAnimationFrame(() => scrollToBottom('auto'))
        await restoreLiveRunRef.current(id, msgs)
      } catch {
        // Background conversation load: a transient failure stays silent;
        // the idle sync retries listMessages every 2s.
      }
    })()

    return () => {
      cancelled = true
      stopStream()
      stopPoll()
    }
    // stopStream / setters are stable; conversationId is the reload trigger.
  }, [conversationId])

  // Idle sync: weixin (and other external) inbound turns append messages / create
  // runs without this tab knowing. Poll while a conversation is open so /ui
  // picks them up without a manual refresh.
  useEffect(() => {
    const id = conversationId
    let cancelled = false
    let fingerprint = ''

    const tick = async () => {
      if (cancelled || conversationIdRef.current !== id) return
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
        return
      }
      try {
        await refreshConversations()
        // While this tab is streaming, the live fold is the source of truth.
        // Polling listMessages would insert the persisted assistant (or replace
        // the optimistic user row) above the live blocks and shuffle order.
        if (liveRunIdRef.current) return
        const msgs = await listMessages(id)
        if (cancelled || conversationIdRef.current !== id) return
        const next =
          msgs.length === 0
            ? '0'
            : `${msgs.length}:${msgs[msgs.length - 1]?.id ?? ''}:${msgs[msgs.length - 1]?.run_id ?? ''}`
        if (next !== fingerprint) {
          fingerprint = next
          setMessages(msgs)
        }
        await restoreLiveRun(id, msgs)
      } catch {
        /* ignore transient poll errors */
      }
    }

    const timer = window.setInterval(() => {
      void tick()
    }, IDLE_SYNC_MS)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [conversationId, conversationIdRef, liveRunIdRef, refreshConversations, restoreLiveRun, setMessages])

  // Returns false when the message is rejected up-front (no model / vision
  // gate / attachment failure) so Composer keeps the draft text and files;
  // true once the message has been accepted (optimistic row appended), even if
  // the run creation fails afterwards.
  const onSend = async (text: string, files: File[]): Promise<boolean> => {
    const sentConversationId = conversationId

    // No model configured: block up-front. Guidance is role-specific so an
    // operator is never pointed at the admin-only model settings page.
    if (modelProfiles.length === 0) {
      setBusy(false)
      setStatus('')
      pushToast({
        tone: 'error',
        title: role === 'admin' ? CHAT.noModelAdmin : CHAT.noModelOperator,
      })
      return false
    }

    setBusy(true)
    setStatus(CHAT.sending)

    // Build attachments from selected files. Image attachments are gated by
    // the active model choice: Auto routes to a vision model when available,
    // while a manual pick is honored exactly (a text-only manual choice on an
    // image turn is rejected up-front rather than silently rerouted).
    let attachments: Attachment[] | undefined
    if (files.length > 0) {
      try {
        const built = await Promise.all(files.map((f) => fileToAttachment(f)))
        const gate = visionGate(
          modelProfiles,
          selectedModelId,
          built.some((a) => isImageAttachment(a.media_type)),
          supportsVision,
        )
        if (!gate.allowed) {
          setBusy(false)
          setStatus('')
          // Image capability mismatch is shown as a modal, not a toast, so the
          // user keeps their draft/attachments and the message is not rerouted.
          // Operators without settings access get contact-admin guidance.
          setVisionWarning(
            role === 'admin'
              ? (gate.message ?? CHAT.visionWarningFallback)
              : CHAT.visionBlockedOperator,
          )
          return false
        }
        attachments = built
      } catch (err) {
        setBusy(false)
        setStatus('')
        reportError(err)
        return false
      }
    }

    // Gates passed: the message is accepted. Only now drop the rollback draft
    // source so a rejected send keeps the composer contents.
    setComposerDraft(undefined)

    // Optimistic bubble renders exactly what was sent: typed text plus inline
    // image / file-card previews backed by transient blob: URLs (no redundant
    // "（附件：…）" note). The server version replaces it on run end.
    const preview = buildLocalPreview(text, files)
    const userBubble = preview.content

    setMessages((prev) => [
      ...prev,
      {
        id: `local_${Date.now()}`,
        conversation_id: sentConversationId,
        role: 'user',
        content: userBubble,
        created_at: new Date().toISOString(),
      },
    ])
    writeLiveEvents([])
    lastEventIndexRef.current = -1
    requestAnimationFrame(() => scrollToBottom('smooth'))

    try {
      const runOptions = buildRunOptions(selectedModelId, {
        attachments,
        ...(thinkingLevel ? { thinkingLevel: thinkingLevel as ThinkingLevel } : {}),
        workspaceId,
      })
      const created = await createRun(agentId, text, sentConversationId, runOptions)
      // Model choice is persisted via onChooseModel; do not reset after send.
      await refreshConversations()
      // The conversation switched mid-flight: the message was already accepted,
      // so report acceptance even though this view no longer tracks the run.
      if (conversationIdRef.current !== sentConversationId) return true
      setMessages((prev) => {
        const copy = [...prev]
        for (let i = copy.length - 1; i >= 0; i--) {
          if (copy[i].role === 'user' && copy[i].id.startsWith('local_') && !copy[i].run_id) {
            copy[i] = { ...copy[i], run_id: created.run_id }
            break
          }
        }
        return copy
      })
      attachRun(created.run_id, sentConversationId, created.status)
    } catch (err) {
      // Post-acceptance failure (vision_unsupported / 5xx / network): the
      // Composer still clears; reconcile messages with the server below.
      if (conversationIdRef.current !== sentConversationId) return true
      setBusy(false)
      setLiveRunId(null)
      writeLiveEvents([])
      // friendlyError maps vision_unsupported / 5xx / network to human titles.
      reportError(err)
      setStatus('')
      try {
        const msgs = await listMessages(sentConversationId)
        if (conversationIdRef.current === sentConversationId) setMessages(msgs)
      } catch {
        /* keep optimistic row */
      }
    }
    return true
  }

  const onCancelRun = async () => {
    if (!liveRunId) return
    setStatus(CHAT.cancelling)
    try {
      await cancelRun(liveRunId)
      await finishLiveRun(conversationId)
      setStatus(CHAT.cancelled)
    } catch (e) {
      reportError(e)
      try {
        await finishLiveRun(conversationId)
      } catch {
        setBusy(false)
        setLiveRunId(null)
      }
    }
  }

  return {
    liveEvents,
    liveRunId,
    busy,
    status,
    setBusy,
    setStatus,
    stopStream,
    stopPoll,
    resetLive,
    attachRun,
    restoreLiveRun,
    finishLiveRun,
    onSend,
    onCancelRun,
  }
}
