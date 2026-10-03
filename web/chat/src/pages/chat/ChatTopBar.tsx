import { Link } from 'react-router-dom'
import { Menu } from 'lucide-react'
import type { ModelProfile } from '../../api'
import { LanguageChip } from '../../components/LanguageChip'
import { ModelChip } from '../../components/ModelChip'
import { ThinkingChip } from '../../components/ThinkingChip'
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

export type ChatComposerToolbarProps = {
  role: string
  modelProfiles: ModelProfile[]
  selectedModelId: string
  thinkingLevel: string
  disabled: boolean
  onChooseModel: (id: string) => void
  onChooseThinking: (level: string) => void
}

/** Model / thinking / language chips passed into Composer toolbar. */
export function ChatComposerToolbar({
  role,
  modelProfiles,
  selectedModelId,
  thinkingLevel,
  disabled,
  onChooseModel,
  onChooseThinking,
}: ChatComposerToolbarProps) {
  return (
    <>
      {modelProfiles.length > 0 ? (
        <>
          <ModelChip
            profiles={modelProfiles}
            value={selectedModelId}
            onChange={onChooseModel}
            disabled={disabled}
          />
          <ThinkingChip
            value={thinkingLevel}
            onChange={onChooseThinking}
            disabled={disabled}
          />
        </>
      ) : role === 'admin' ? (
        <Link to="/settings/models" className="model-chip model-chip-empty">
          {CHAT.addModel}
        </Link>
      ) : (
        <span className="model-chip model-chip-empty" aria-disabled="true">
          {CHAT.noModelConfigured}
        </span>
      )}
      <LanguageChip />
    </>
  )
}
