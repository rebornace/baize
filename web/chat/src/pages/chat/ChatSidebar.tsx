import { Link } from 'react-router-dom'
import { useState } from 'react'
import { Search } from 'lucide-react'
import type { ConversationScope, ConversationSummary, Workspace } from '../../api'
import { LanguageChip } from '../../components/LanguageChip'
import { SidebarResizer } from '../../components/SidebarResizer'
import { Pencil, Pin, Archive, ArchiveRestore, Tag } from 'lucide-react'
import { Button, ThemeToggle } from '../../components/ui'
import { conversationListLabel } from '../../conversationLabel'
import { conversationUpdatedLabel } from '../../conversationUpdatedLabel'
import { useLocale } from '../../locale/LocaleContext'
import { clearControlToken } from '../../controlAuth'
import { CHAT } from '../../strings'
import { FeatureSearchButton } from '../../components/FeatureSearch'
import { BudgetChip } from './BudgetChip'
import { AttachmentManager } from './AttachmentManager'
import { MoveConversationDialog } from './MoveConversationDialog'
import { RenameConversationDialog } from './RenameConversationDialog'
import { TagsDialog } from './TagsDialog'

export type ChatSidebarProps = {
  role: string
  gateEnabled: boolean
  conversationId: string
  conversationScope: ConversationScope
  conversations: ConversationSummary[]
  onNewChat: () => void
  onSelectConversation: (id: string) => void
  onDeleteConversation: (id: string) => void
  onScopeChange: (scope: ConversationScope) => void
  onCloseDrawer: () => void
  workspaceId: string
  workspaces: Workspace[]
  onSwitchWorkspace: (id: string) => void
  onCreateWorkspace: (name: string) => Promise<unknown>
  pendingCount: number
  onOpenPending: () => void
  onOpenMessageSearch: () => void
  onExportConversation: () => void
  onMoveConversation: (id: string, workspaceId: string) => Promise<void>
  onMoveConversations: (ids: string[], workspaceId: string) => Promise<void>
  onDeleteConversations: (ids: string[]) => Promise<void>
  onRenameConversation: (id: string, title: string) => Promise<void>
  onToggleConversationPin: (id: string, pinned: boolean) => Promise<void>
  archivedView: boolean
  onToggleArchivedView: () => void
  onSetConversationArchived: (id: string, archived: boolean) => Promise<void>
  onArchiveConversations: (ids: string[], archived: boolean) => Promise<void>
  onMarkAllRead: () => Promise<void>
  onSetConversationTags: (id: string, tags: string[]) => Promise<void>
  readOnly?: boolean
}

export function ChatSidebar({
  role,
  gateEnabled,
  conversationId,
  conversationScope,
  conversations,
  onNewChat,
  onSelectConversation,
  onDeleteConversation,
  onScopeChange,
  onCloseDrawer,
  workspaceId,
  workspaces,
  onSwitchWorkspace,
  onCreateWorkspace,
  pendingCount,
  onOpenPending,
  onOpenMessageSearch,
  onExportConversation,
  onMoveConversation,
  onMoveConversations,
  onDeleteConversations,
  onRenameConversation,
  onToggleConversationPin,
  archivedView,
  onToggleArchivedView,
  onSetConversationArchived,
  onArchiveConversations,
  onMarkAllRead,
  onSetConversationTags,
  readOnly = false,
}: ChatSidebarProps) {
  const { locale } = useLocale()
  const [attachmentsOpen, setAttachmentsOpen] = useState(false)
  const [moveOpen, setMoveOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<ConversationSummary | null>(null)
  const [tagTarget, setTagTarget] = useState<ConversationSummary | null>(null)
  const [tagFilter, setTagFilter] = useState('')
  const [batch, setBatch] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [bulkMoveOpen, setBulkMoveOpen] = useState(false)
  const [archiveQuery, setArchiveQuery] = useState('')
  const moveTargets = workspaces.filter((w) => w.id !== workspaceId)

  const allTags = [...new Set(conversations.flatMap((c) => c.tags ?? []))].sort((a, b) =>
    a.localeCompare(b),
  )
  let visibleConversations = archivedView && archiveQuery.trim()
    ? conversations.filter((c) =>
        conversationListLabel(c.id, c.title)
          .toLowerCase()
          .includes(archiveQuery.trim().toLowerCase()),
      )
    : conversations
  if (tagFilter) {
    visibleConversations = visibleConversations.filter((c) =>
      (c.tags ?? []).some((tag) => tag.toLowerCase() === tagFilter.toLowerCase()),
    )
  }
  const unreadCount = visibleConversations.filter((c) => c.unread).length

  const toggleSelected = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const allSelected =
    visibleConversations.length > 0 && selected.size === visibleConversations.length
  const toggleAll = () => {
    setSelected(allSelected ? new Set() : new Set(visibleConversations.map((c) => c.id)))
  }

  const exitBatch = () => {
    setBatch(false)
    setSelected(new Set())
  }

  const deleteSelected = async () => {
    if (selected.size === 0) return
    if (!window.confirm(CHAT.batchDeleteConfirm(selected.size))) return
    await onDeleteConversations([...selected])
    exitBatch()
  }

  const archiveSelected = async () => {
    if (selected.size === 0) return
    await onArchiveConversations([...selected], !archivedView)
    exitBatch()
  }

  const toggleArchivedView = () => {
    setArchiveQuery('')
    setSelected(new Set())
    setBatch(false)
    onToggleArchivedView()
  }

  return (
    <aside className="chat-sidebar" aria-label={CHAT.conversationListAria}>
      <div className="chat-sidebar-top">
        {!readOnly ? (
          <button
            type="button"
            className="btn primary sidebar-new"
            onClick={() => {
              if (archivedView) toggleArchivedView()
              onNewChat()
              onCloseDrawer()
            }}
          >
            {CHAT.newChat}
          </button>
        ) : null}
        {!readOnly ? <FeatureSearchButton className="sidebar-search" /> : null}
        <Button
          variant="ghost"
          size="sm"
          className="sidebar-message-search"
          onClick={onOpenMessageSearch}
        >
          <Search size={14} aria-hidden="true" />
          {CHAT.messageSearchTitle}
        </Button>
        {pendingCount > 0 && !readOnly && (
          <Button
            size="sm"
            variant="secondary"
            className="sidebar-pending"
            data-testid="sidebar-pending"
            aria-label={CHAT.pendingCenterAria(pendingCount)}
            onClick={onOpenPending}
          >
            <span className="sidebar-pending-dot" aria-hidden="true" />
            {CHAT.pendingCenterTrigger(pendingCount)}
          </Button>
        )}
        <div className="workspace-picker">
          <label className="workspace-picker-label" htmlFor="workspace-select">
            {CHAT.workspaceLabel}
          </label>
          <select
            id="workspace-select"
            className="workspace-select"
            aria-label={CHAT.workspaceLabel}
            value={workspaceId}
            onChange={(e) => {
              void onSwitchWorkspace(e.target.value)
            }}
          >
            {(workspaces.length > 0 ? workspaces : [{ id: workspaceId, name: CHAT.defaultWorkspace }]).map(
              (w) => (
                <option key={w.id} value={w.id}>
                  {w.id === 'default' ? CHAT.defaultWorkspace : w.name}
                </option>
              ),
            )}
          </select>
          {!readOnly ? (
            <button
              type="button"
              className="btn secondary sm workspace-new"
              onClick={() => {
                const name = window.prompt(CHAT.newWorkspacePrompt)?.trim()
                if (!name) return
                void onCreateWorkspace(name).catch(() => undefined)
              }}
            >
              <span className="workspace-new-plus" aria-hidden="true">
                +
              </span>
              {CHAT.newWorkspace}
            </button>
          ) : null}
        </div>
        {allTags.length > 0 ? (
          <select
            className="workspace-select conversation-tag-filter"
            aria-label={CHAT.tagsFilterLabel}
            value={tagFilter}
            onChange={(e) => {
              setTagFilter(e.target.value)
              setSelected(new Set())
            }}
          >
            <option value="">{CHAT.tagsFilterAll}</option>
            {allTags.map((tag) => (
              <option key={tag} value={tag}>
                {tag}
              </option>
            ))}
          </select>
        ) : null}
        {role === 'admin' && (
          <div className="conversation-scope" role="group" aria-label={CHAT.scopeAria}>
            <button
              type="button"
              className={
                conversationScope === 'all'
                  ? 'conversation-scope-btn active'
                  : 'conversation-scope-btn'
              }
              onClick={() => onScopeChange('all')}
            >
              {CHAT.scopeAll}
            </button>
            <button
              type="button"
              className={
                conversationScope === 'mine'
                  ? 'conversation-scope-btn active'
                  : 'conversation-scope-btn'
              }
              onClick={() => onScopeChange('mine')}
            >
              {CHAT.scopeMine}
            </button>
          </div>
        )}
        {archivedView ? (
          <input
            type="search"
            className="conversation-archive-search"
            placeholder={CHAT.archivedSearchPlaceholder}
            aria-label={CHAT.archivedSearchPlaceholder}
            value={archiveQuery}
            onChange={(e) => {
              setArchiveQuery(e.target.value)
              setSelected(new Set())
            }}
          />
        ) : null}
        {batch ? (
          <div className="conversation-batch-bar" data-testid="conversation-batch-bar">
            <button type="button" className="settings-link" onClick={toggleAll}>
              {allSelected ? CHAT.batchClear : CHAT.batchSelectAll}
            </button>
            <span className="settings-muted">{CHAT.batchSelected(selected.size)}</span>
            <Button
              type="button"
              size="sm"
              variant="secondary"
              disabled={selected.size === 0 || moveTargets.length === 0}
              onClick={() => setBulkMoveOpen(true)}
            >
              {CHAT.batchMove}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="secondary"
              disabled={selected.size === 0}
              onClick={() => void archiveSelected()}
            >
              {archivedView ? CHAT.batchUnarchive : CHAT.batchArchive}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="danger"
              disabled={selected.size === 0}
              onClick={() => void deleteSelected()}
            >
              {CHAT.batchDelete}
            </Button>
            <button type="button" className="settings-link" onClick={exitBatch}>
              {CHAT.batchExit}
            </button>
          </div>
        ) : null}
        <ul className="conversation-list">
          {archivedView && visibleConversations.length === 0 ? (
            <li className="conversation-archived-empty" data-testid="archived-empty">
              {archiveQuery.trim() ? CHAT.archivedSearchEmpty : CHAT.archivedEmpty}
            </li>
          ) : null}
          {visibleConversations.map((c) => (
            <li key={c.id} className="conversation-row">
              <button
                type="button"
                aria-pressed={batch ? selected.has(c.id) : undefined}
                className={
                  (c.id === conversationId
                    ? 'conversation-item active'
                    : 'conversation-item') +
                  (batch ? ' batch-mode' : '') +
                  (batch && selected.has(c.id) ? ' batch-selected' : '')
                }
                onClick={() => {
                  if (batch) {
                    toggleSelected(c.id)
                    return
                  }
                  onSelectConversation(c.id)
                  onCloseDrawer()
                }}
              >
                {batch ? (
                  <span className="conversation-batch-check" aria-hidden="true">
                    {selected.has(c.id) ? '✓' : ''}
                  </span>
                ) : null}
                <span className="conversation-item-title">
                  {conversationListLabel(c.id, c.title)}
                </span>
                {c.tags && c.tags.length > 0 ? (
                  <span className="conversation-item-tags">
                    {c.tags.slice(0, 3).map((tag) => (
                      <span className="conversation-tag-chip" key={tag}>
                        {tag}
                      </span>
                    ))}
                  </span>
                ) : null}
                {c.unread ? (
                  <span
                    className="conversation-unread-dot"
                    title={CHAT.unread}
                    aria-label={CHAT.unread}
                  />
                ) : null}
                <span className="conversation-item-meta">
                  {c.pinned ? (
                    <Pin size={10} className="conversation-meta-pin" aria-hidden="true" />
                  ) : null}
                  {conversationUpdatedLabel(c.updated_at, locale)}
                </span>
              </button>
              {!batch && !readOnly ? (
                <span className="conversation-actions">
                  <button
                    type="button"
                    className={c.pinned ? 'conversation-pin pinned' : 'conversation-pin'}
                    title={c.pinned ? CHAT.unpinConversation : CHAT.pinConversation}
                    aria-label={c.pinned ? CHAT.unpinConversation : CHAT.pinConversation}
                    aria-pressed={!!c.pinned}
                    onClick={(e) => {
                      e.stopPropagation()
                      void onToggleConversationPin(c.id, !c.pinned)
                    }}
                  >
                    <Pin size={12} fill={c.pinned ? 'currentColor' : 'none'} aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    className="conversation-archive"
                    title={archivedView ? CHAT.unarchiveConversation : CHAT.archiveConversation}
                    aria-label={
                      archivedView
                        ? CHAT.unarchiveConversationAria(conversationListLabel(c.id, c.title))
                        : CHAT.archiveConversationAria(conversationListLabel(c.id, c.title))
                    }
                    onClick={(e) => {
                      e.stopPropagation()
                      void onSetConversationArchived(c.id, !archivedView)
                    }}
                  >
                    {archivedView ? (
                      <ArchiveRestore size={12} aria-hidden="true" />
                    ) : (
                      <Archive size={12} aria-hidden="true" />
                    )}
                  </button>
                  <button
                    type="button"
                    className="conversation-tag"
                    title={CHAT.editTags}
                    aria-label={CHAT.editTagsAria(conversationListLabel(c.id, c.title))}
                    onClick={(e) => {
                      e.stopPropagation()
                      setTagTarget(c)
                    }}
                  >
                    <Tag size={12} aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    className="conversation-rename"
                    title={CHAT.renameConversation}
                    aria-label={CHAT.renameConversationAria(conversationListLabel(c.id, c.title))}
                    onClick={(e) => {
                      e.stopPropagation()
                      setRenameTarget(c)
                    }}
                  >
                    <Pencil size={12} aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    className="conversation-delete"
                    title={CHAT.deleteConversationTitle}
                    aria-label={CHAT.deleteConversationAria(conversationListLabel(c.id, c.title))}
                    onClick={(e) => {
                      e.stopPropagation()
                      onDeleteConversation(c.id)
                    }}
                  >
                    ✕
                  </button>
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      </div>
      <div className="chat-sidebar-bottom">
        <BudgetChip enabled={role === 'admin'} />
        <div className="sidebar-chrome-row">
          <ThemeToggle />
          <LanguageChip />
        </div>
        <div className="sidebar-footer-links">
          {conversationId && !readOnly ? (
            <button
              type="button"
              className="settings-link"
              onClick={() => setAttachmentsOpen(true)}
              data-testid="attachment-manager-open"
            >
              {CHAT.attachmentManager}
            </button>
          ) : null}
          <button
            type="button"
            className="settings-link sidebar-export"
            onClick={onExportConversation}
          >
            {CHAT.exportConversation}
          </button>
          {conversationId && moveTargets.length > 0 && !readOnly ? (
            <button
              type="button"
              className="settings-link"
              onClick={() => setMoveOpen(true)}
              data-testid="move-conversation-open"
            >
              {CHAT.moveConversation}
            </button>
          ) : null}
          {conversations.length > 0 && !batch && !readOnly ? (
            <button
              type="button"
              className="settings-link"
              onClick={() => setBatch(true)}
              data-testid="conversation-batch-open"
            >
              {CHAT.batchManage}
            </button>
          ) : null}
          {!batch && unreadCount > 0 ? (
            <button
              type="button"
              className="settings-link"
              onClick={() => void onMarkAllRead()}
              data-testid="mark-all-read"
            >
              {CHAT.markAllRead(unreadCount)}
            </button>
          ) : null}
          {!batch ? (
            <button
              type="button"
              className="settings-link"
              onClick={toggleArchivedView}
              data-testid="conversation-archived-toggle"
            >
              {archivedView ? CHAT.showActiveConversations : CHAT.showArchivedConversations}
            </button>
          ) : null}
        </div>
        <div className="sidebar-footer-nav">
          {!readOnly ? (
            <Link
              to={role === 'admin' ? '/settings' : '/settings/identities'}
              className="settings-link"
            >
              {role === 'admin' ? CHAT.linkSettings : CHAT.linkAccounts}
            </Link>
          ) : (
            <Link to="/settings/viewer" className="settings-link">
              {CHAT.viewerDashboard}
            </Link>
          )}
          {gateEnabled && (
            <button
              type="button"
              className="settings-logout"
              onClick={() => {
                clearControlToken()
                window.location.assign('/ui/')
              }}
            >
              {CHAT.logout}
            </button>
          )}
        </div>
      </div>
      <AttachmentManager
        conversationId={conversationId}
        open={attachmentsOpen}
        onClose={() => setAttachmentsOpen(false)}
      />
      <MoveConversationDialog
        open={moveOpen}
        targets={moveTargets}
        onClose={() => setMoveOpen(false)}
        onMove={(workspaceId) => onMoveConversation(conversationId, workspaceId)}
      />
      <MoveConversationDialog
        open={bulkMoveOpen}
        targets={moveTargets}
        onClose={() => setBulkMoveOpen(false)}
        onMove={async (workspaceId) => {
          await onMoveConversations([...selected], workspaceId)
          exitBatch()
        }}
      />
      <RenameConversationDialog
        open={renameTarget != null}
        initialTitle={renameTarget?.title ?? ''}
        onClose={() => setRenameTarget(null)}
        onSave={(title) =>
          renameTarget ? onRenameConversation(renameTarget.id, title) : Promise.resolve()
        }
      />
      <TagsDialog
        open={tagTarget != null}
        initialTags={tagTarget?.tags ?? []}
        onClose={() => setTagTarget(null)}
        onSave={(tags) =>
          tagTarget ? onSetConversationTags(tagTarget.id, tags) : Promise.resolve()
        }
      />
      <SidebarResizer />
    </aside>
  )
}
