import { useCallback, useEffect, useRef, useState } from 'react'
import {
  getUIConfig,
  listModelProfiles,
  listSkills,
  listTools,
  type ModelProfile,
  type SkillSummary,
  type ToolInfo,
} from '../api'
import { Composer } from '../components/Composer'
import {
  Button,
  ConfirmDialog,
  Modal,
  ToastRegion,
  useToast,
} from '../components/ui'
import { foldEvents } from '../foldEvents'
import type { ToolCatalog } from '../friendlyTool'
import { useGate } from '../gateContext'
import { useLocale } from '../locale/LocaleContext'
import { loadModelChoice, resolveModelChoice, saveModelChoice } from '../modelChoice'
import { loadThinkingChoice, saveThinkingChoice } from '../thinkingChoice'
import { AUTO_MODEL_ID } from '../modelSelect'
import { CHAT, friendlyError, LOGIN_AT } from '../strings'
import { replaceMention } from '../skillMention'
import { useStickToBottom } from '../useStickToBottom'
import { useDrawer } from '../useDrawer'
import { ChatMessageList, copyText } from './chat/ChatMessageList'
import { ChatSidebar } from './chat/ChatSidebar'
import { ChatComposerToolbar, ChatTopBar } from './chat/ChatTopBar'
import { useChatRun } from './chat/useChatRun'
import { useChatSession, type ChatLiveControls } from './chat/useChatSession'

const AGENT_FALLBACK = 'ticket-agent'

export function ChatPage() {
  const { role, gateEnabled } = useGate()
  // Subscribe so chat chrome rebuilds when language changes (without full reload).
  useLocale()
  const drawer = useDrawer()
  const toast = useToast()
  const pushToast = toast.push

  const [agentId, setAgentId] = useState(AGENT_FALLBACK)
  const [skills, setSkills] = useState<SkillSummary[]>([])
  const [supportsVision, setSupportsVision] = useState(true)
  const [modelProfiles, setModelProfiles] = useState<ModelProfile[]>([])
  // Persisted model choice (localStorage via modelChoice.ts); "" only until the
  // lazy initializer runs. Auto is the server-side smart router.
  const [selectedModelId, setSelectedModelId] = useState(loadModelChoice)
  const [toolCatalog, setToolCatalog] = useState<ToolCatalog>([])
  const [visionWarning, setVisionWarning] = useState<string | null>(null)

  const liveRef = useRef<ChatLiveControls>({
    busy: false,
    liveRunId: null,
    setBusy: () => {},
    stopStream: () => {},
    stopPoll: () => {},
    resetLive: () => {},
    attachRun: () => {},
  })

  // User-initiated actions surface friendly errors via Toast. Background
  // failures (polling / idle sync / background refetches) stay silent so a
  // 700ms poll loop cannot spam toasts; their state keeps advancing on retry.
  const reportError = useCallback(
    (e: unknown) => {
      const f = friendlyError(e)
      pushToast({ tone: 'error', title: f.title, detail: f.detail })
    },
    [pushToast],
  )

  const session = useChatSession({
    role,
    reportError,
    pushToast,
    liveRef,
    agentId,
  })

  // Per-conversation thinking override (sessionStorage); '' = follow model default.
  const [thinkingLevel, setThinkingLevel] = useState(() =>
    loadThinkingChoice(session.conversationId),
  )

  // Stable scroll handle so useChatRun's conversation-switch effect does not churn.
  const scrollToBottomRef = useRef<(behavior?: ScrollBehavior) => void>(() => {})
  const scrollToBottomStable = useCallback((behavior: ScrollBehavior = 'auto') => {
    scrollToBottomRef.current(behavior)
  }, [])

  const run = useChatRun({
    role,
    agentId,
    conversationId: session.conversationId,
    conversationIdRef: session.conversationIdRef,
    setMessages: session.setMessages,
    setHistoryPages: session.setHistoryPages,
    setHistoryBlocks: session.setHistoryBlocks,
    fetchedRunsRef: session.fetchedRunsRef,
    mergeHistoryPages: session.mergeHistoryPages,
    refreshConversations: session.refreshConversations,
    setComposerDraft: session.setComposerDraft,
    modelProfiles,
    selectedModelId,
    thinkingLevel,
    supportsVision,
    scrollToBottom: scrollToBottomStable,
    reportError,
    pushToast,
    liveRef,
    setVisionWarning,
  })

  const { scrollerRef, bottomRef, onScroll, scrollToBottom } = useStickToBottom([
    session.messages,
    run.liveEvents,
    run.liveRunId,
    session.conversationId,
    session.historyPages,
    session.historyBlocks,
  ])
  scrollToBottomRef.current = scrollToBottom

  useEffect(() => {
    // One cancel flag for every mount-time fetch: under StrictMode dev double
    // mount, the first effect's stale responses must not fire setState/toasts
    // (in particular a duplicated model-stale-fallback toast).
    let cancelled = false
    void getUIConfig()
      .then((cfg) => {
        if (cancelled) return
        if (cfg.agent_id) setAgentId(cfg.agent_id)
        setSupportsVision(cfg.supports_vision !== false)
      })
      .catch(() => {
        /* keep fallback */
      })
    // Skills drive the Composer @-completion popup. GET /v0/skills is operator
    // readable; load failures just disable the popup rather than blocking chat.
    void listSkills()
      .then((res) => {
        if (!cancelled) setSkills(res.skills ?? [])
      })
      .catch(() => {
        if (!cancelled) setSkills([])
      })
    // Model profiles feed the model chip. GET /v0/settings/models is operator
    // readable; a failure (or an empty list) shows the role-appropriate empty
    // state and falls back to the default model without blocking chat.
    void listModelProfiles()
      .then((list) => {
        if (cancelled) return
        const profiles = Array.isArray(list) ? list : []
        setModelProfiles(profiles)
        // A persisted concrete choice whose model was deleted falls back to
        // Auto; persist the fallback and tell the user once.
        const resolved = resolveModelChoice(profiles)
        if (resolved.stale) {
          setSelectedModelId(AUTO_MODEL_ID)
          saveModelChoice(AUTO_MODEL_ID)
          toast.push({ tone: 'info', title: CHAT.modelStaleFallback })
        }
      })
      .catch(() => {
        if (!cancelled) setModelProfiles([])
      })
    // Tool catalog powers friendly tool names in cards. Failures degrade
    // silently: cards then show the raw tool technical names.
    void listTools()
      .then((tools: ToolInfo[]) => {
        if (cancelled) return
        setToolCatalog(
          tools.map((t) => ({
            name: t.name,
            title: t.title,
            description: t.description,
            connector_id: t.connector_id,
          })),
        )
      })
      .catch(() => {
        /* catalog missing is non-blocking */
      })
    return () => {
      cancelled = true
    }
    // toast.push is identity-stable (useCallback); effect runs once on mount.
  }, [])

  // Sticky thinking override is keyed by conversationId; switching chats resets
  // to that chat's stored choice (or '' = follow model default).
  useEffect(() => {
    setThinkingLevel(loadThinkingChoice(session.conversationId))
  }, [session.conversationId])

  /** login_required「去登录」→ 刷新 skills 后写入 @login-<id>，聚焦输入框，不自动发送。 */
  const onGoLoginSkill = async (skillId: string) => {
    let list = skills
    try {
      const res = await listSkills()
      list = res.skills ?? []
      setSkills(list)
    } catch {
      /* keep cached skills; still validate below */
    }
    if (!list.some((s) => s.id === skillId)) {
      toast.push({ tone: 'error', title: LOGIN_AT.skillMissing })
      return
    }
    const { text } = replaceMention('', 0, 0, skillId)
    session.setComposerDraft(text)
  }

  // Model choice persists immediately (localStorage); it survives sends and
  // reloads, and falls back to Auto when it no longer resolves (see mount effect).
  const onChooseModel = (id: string) => {
    setSelectedModelId(id)
    saveModelChoice(id)
  }

  const onChooseThinking = (level: string) => {
    setThinkingLevel(level)
    saveThinkingChoice(session.conversationId, level)
  }

  const onCopyMessage = (m: (typeof session.messages)[number]) => {
    void copyText(m.content).then((ok) => {
      toast.push(
        ok
          ? { tone: 'success', title: CHAT.copySuccess }
          : { tone: 'error', title: CHAT.copyFailed },
      )
    })
  }

  const liveBlocks = run.liveRunId ? foldEvents(run.liveRunId, run.liveEvents) : []
  const showWelcome = session.messages.length === 0 && liveBlocks.length === 0
  const composerDisabled = run.busy || session.historyMutating
  const showStop = Boolean(run.liveRunId && run.busy)

  return (
    <div className={`chat-shell app-with-drawer${drawer.isOpen ? ' drawer-open' : ''}`}>
      <button
        type="button"
        className="app-drawer-scrim"
        aria-label={CHAT.closeMenu}
        onClick={drawer.close}
      />
      <ChatSidebar
        role={role}
        gateEnabled={gateEnabled}
        conversationId={session.conversationId}
        conversationScope={session.conversationScope}
        conversations={session.conversations}
        onNewChat={session.onNewChat}
        onSelectConversation={session.onSelectConversation}
        onDeleteConversation={session.onDeleteConversation}
        onScopeChange={session.setConversationScope}
        onCloseDrawer={drawer.close}
      />

      <main className="chat-main">
        <ChatTopBar onOpenDrawer={drawer.open} />
        <ChatMessageList
          messages={session.messages}
          liveBlocks={liveBlocks}
          liveRunId={run.liveRunId}
          historyPages={session.historyPages}
          historyBlocks={session.historyBlocks}
          toolCatalog={toolCatalog}
          busy={run.busy}
          historyMutating={session.historyMutating}
          showWelcome={showWelcome}
          scrollerRef={scrollerRef}
          bottomRef={bottomRef}
          onScroll={onScroll}
          onRollbackUser={session.onRollbackUser}
          onRegenerate={session.onRegenerate}
          onRollbackTo={session.onRollbackTo}
          onFork={session.onFork}
          onCopyMessage={onCopyMessage}
          reportError={reportError}
          onGoLoginSkill={onGoLoginSkill}
        />

        <div className="chat-footer">
          {(run.status || showStop) && (
            <div className="chat-status-row">
              <p className="status">{run.status}</p>
              {showStop && (
                <button type="button" className="btn danger sm" onClick={() => void run.onCancelRun()}>
                  {CHAT.stop}
                </button>
              )}
            </div>
          )}

          <Composer
            disabled={composerDisabled}
            draft={session.composerDraft}
            skills={skills}
            onSend={run.onSend}
            toolbar={
              <ChatComposerToolbar
                role={role}
                modelProfiles={modelProfiles}
                selectedModelId={selectedModelId}
                thinkingLevel={thinkingLevel}
                disabled={composerDisabled}
                onChooseModel={onChooseModel}
                onChooseThinking={onChooseThinking}
              />
            }
          />
        </div>
      </main>

      <ConfirmDialog
        open={session.confirmDelete !== null}
        danger
        title={CHAT.deleteTitle}
        body={CHAT.deleteBody}
        confirmText={CHAT.deleteConfirm}
        onConfirm={() => void session.performDelete()}
        onCancel={() => session.setConfirmDelete(null)}
      />
      <Modal
        open={visionWarning !== null}
        title={CHAT.visionWarningTitle}
        onClose={() => setVisionWarning(null)}
        footer={
          <Button variant="primary" onClick={() => setVisionWarning(null)}>
            {CHAT.visionWarningAck}
          </Button>
        }
      >
        <p className="vision-warning-body">{visionWarning}</p>
      </Modal>
      <ToastRegion toasts={toast.toasts} onDismiss={toast.dismiss} />
    </div>
  )
}
