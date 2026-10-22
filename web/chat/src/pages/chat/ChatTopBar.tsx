import { Link } from 'react-router-dom'
import { Menu } from 'lucide-react'
import type { ModelProfile } from '../../api'
import { ModelThinkingDualMenu } from '../../components/ModelThinkingDualMenu'
import { CHAT } from '../../strings'

export type ChatTopBarProps = {
  onOpenDrawer: () => void
}

/** Mobile conversation-list toggle (app-mobile-bar). */
export function ChatTopBar({ onOpenDrawer }: ChatTopBarProps) {
  return (
    <div className="app-mobile-bar">
      <button
        type="button"
        className="app-menu-btn"
        aria-label={CHAT.openConversationList}
        onClick={onOpenDrawer}
      >
        <Menu size={20} aria-hidden="true" />
      </button>
    </div>
  )
}

export type ChatComposerTrailingProps = {
  role: string
  modelProfiles: ModelProfile[]
  selectedModelId: string
  thinkingLevel: string
  disabled: boolean
  onChooseModel: (id: string) => void
  onChooseThinking: (level: string) => void
}

/** Model + thinking dual menu for the composer trailing slot. */
export function ChatComposerTrailing({
  role,
  modelProfiles,
  selectedModelId,
  thinkingLevel,
  disabled,
  onChooseModel,
  onChooseThinking,
}: ChatComposerTrailingProps) {
  if (modelProfiles.length > 0) {
    return (
      <ModelThinkingDualMenu
        profiles={modelProfiles}
        modelId={selectedModelId}
        thinkingLevel={thinkingLevel}
        onChooseModel={onChooseModel}
        onChooseThinking={onChooseThinking}
        disabled={disabled}
      />
    )
  }
  if (role === 'admin') {
    return (
      <Link to="/settings/models" className="model-chip model-chip-empty">
        {CHAT.addModel}
      </Link>
    )
  }
  return (
    <span className="model-chip model-chip-empty" aria-disabled="true">
      {CHAT.noModelConfigured}
    </span>
  )
}

/** @deprecated Use ChatComposerTrailing */
export const ChatComposerToolbar = ChatComposerTrailing
