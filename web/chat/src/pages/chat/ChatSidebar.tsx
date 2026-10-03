import { Link } from 'react-router-dom'
import type { ConversationScope, ConversationSummary, Workspace } from '../../api'
import { SidebarResizer } from '../../components/SidebarResizer'
import { ThemeToggle } from '../../components/ui'
import { conversationListLabel } from '../../conversationLabel'
import { clearControlToken } from '../../controlAuth'
import { CHAT } from '../../strings'

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
}: ChatSidebarProps) {
  return (
    <aside className="chat-sidebar" aria-label={CHAT.conversationListAria}>
      <div className="chat-sidebar-top">
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
            className="btn ghost workspace-new"
            onClick={() => {
              const name = window.prompt(CHAT.newWorkspacePrompt)?.trim()
              if (!name) return
              void onCreateWorkspace(name).catch(() => undefined)
            }}
          >
            {CHAT.newWorkspace}
          </button>
        </div>
        <button
          type="button"
          className="btn ghost sidebar-new"
          onClick={() => {
            onNewChat()
            onCloseDrawer()
          }}
        >
          {CHAT.newChat}
        </button>
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
                {conversationListLabel(c.id, c.title)}
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
        <ThemeToggle />
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
      <SidebarResizer />
    </aside>
  )
}
