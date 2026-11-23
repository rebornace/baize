import { Link } from 'react-router-dom'
import { Search } from 'lucide-react'
import type { ConversationScope, ConversationSummary, Workspace } from '../../api'
import { LanguageChip } from '../../components/LanguageChip'
import { SidebarResizer } from '../../components/SidebarResizer'
import { Button, ThemeToggle } from '../../components/ui'
import { conversationListLabel } from '../../conversationLabel'
import { conversationUpdatedLabel } from '../../conversationUpdatedLabel'
import { useLocale } from '../../locale/LocaleContext'
import { clearControlToken } from '../../controlAuth'
import { CHAT } from '../../strings'
import { FeatureSearchButton } from '../../components/FeatureSearch'

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
}: ChatSidebarProps) {
  const { locale } = useLocale()

  return (
    <aside className="chat-sidebar" aria-label={CHAT.conversationListAria}>
      <div className="chat-sidebar-top">
        <button
          type="button"
          className="btn primary sidebar-new"
          onClick={() => {
            onNewChat()
            onCloseDrawer()
          }}
        >
          {CHAT.newChat}
        </button>
        <FeatureSearchButton className="sidebar-search" />
        <Button
          variant="ghost"
          size="sm"
          className="sidebar-message-search"
          onClick={onOpenMessageSearch}
        >
          <Search size={14} aria-hidden="true" />
          {CHAT.messageSearchTitle}
        </Button>
        {pendingCount > 0 && (
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
        </div>
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
        <ul className="conversation-list">
          {conversations.map((c) => (
            <li key={c.id} className="conversation-row">
              <button
                type="button"
                className={
                  c.id === conversationId
                    ? 'conversation-item active'
                    : 'conversation-item'
                }
                onClick={() => {
                  onSelectConversation(c.id)
                  onCloseDrawer()
                }}
              >
                <span className="conversation-item-title">
                  {conversationListLabel(c.id, c.title)}
                </span>
                <span className="conversation-item-meta">
                  {conversationUpdatedLabel(c.updated_at, locale)}
                </span>
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
            </li>
          ))}
        </ul>
      </div>
      <div className="chat-sidebar-bottom">
        <div className="sidebar-chrome-row">
          <ThemeToggle />
          <LanguageChip />
        </div>
        <div className="sidebar-footer-links">
          <button
            type="button"
            className="settings-link sidebar-export"
            onClick={onExportConversation}
          >
            {CHAT.exportConversation}
          </button>
          <Link
            to={role === 'admin' ? '/settings' : '/settings/identities'}
            className="settings-link"
          >
            {role === 'admin' ? CHAT.linkSettings : CHAT.linkAccounts}
          </Link>
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
      <SidebarResizer />
    </aside>
  )
}
