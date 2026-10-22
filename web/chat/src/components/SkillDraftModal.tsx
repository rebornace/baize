import {
  Button,
  Field,
  Modal,
  Textarea,
} from './ui'
import type { SkillDraftResponse } from '../api/eval'
import { EVAL } from '../strings'

export type SkillDraftModalProps = {
  open: boolean
  runId: string | null
  draft: SkillDraftResponse | null
  markdown: string
  onMarkdownChange: (value: string) => void
  createEvalStub: boolean
  onCreateEvalStubChange: (value: boolean) => void
  busy: boolean
  onClose: () => void
  onPolish: () => void
  onInstall: () => void
}

/** Shared dig-skill preview: timeline + editable markdown + optional polish/EVAL stub. */
export function SkillDraftModal({
  open,
  runId,
  draft,
  markdown,
  onMarkdownChange,
  createEvalStub,
  onCreateEvalStubChange,
  busy,
  onClose,
  onPolish,
  onInstall,
}: SkillDraftModalProps) {
  const title = open
    ? EVAL.digSkillTitle(draft?.source_run_id ?? runId ?? '')
    : EVAL.digSkill

  return (
    <Modal
      open={open}
      title={title}
      onClose={onClose}
      footer={
        draft ? (
          <>
            <Button type="button" variant="secondary" onClick={onClose} disabled={busy}>
              {EVAL.cancel}
            </Button>
            <Button
              type="button"
              variant="secondary"
              onClick={onPolish}
              disabled={busy}
              data-testid="skill-draft-polish"
            >
              {EVAL.digSkillPolish}
            </Button>
            <Button
              type="button"
              onClick={onInstall}
              disabled={busy}
              data-testid="skill-draft-install"
            >
              {EVAL.digSkillInstall}
            </Button>
          </>
        ) : (
          <Button type="button" variant="secondary" onClick={onClose} disabled={busy}>
            {EVAL.cancel}
          </Button>
        )
      }
    >
      {draft == null ? (
        <p className="settings-muted">{EVAL.digSkillLoading}</p>
      ) : (
        <>
          {draft.timeline.length > 0 ? (
            <Field label={EVAL.digSkillTimeline}>
              <ul className="settings-list" data-testid="eval-skill-draft-timeline">
                {draft.timeline.map((e, i) => (
                  <li key={`${e.run_id}-${e.timestamp}-${i}`} className="settings-muted">
                    <code className="settings-code-sample">{e.type}</code>
                    {' · '}
                    {e.summary}
                  </li>
                ))}
              </ul>
            </Field>
          ) : null}
          <Field label={EVAL.digSkillMarkdown}>
            <Textarea
              value={markdown}
              onChange={(e) => onMarkdownChange(e.target.value)}
              disabled={busy}
              rows={14}
              data-testid="eval-skill-draft-markdown"
            />
          </Field>
          <label className="settings-muted" style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
            <input
              type="checkbox"
              checked={createEvalStub}
              onChange={(e) => onCreateEvalStubChange(e.target.checked)}
              disabled={busy}
              data-testid="skill-draft-eval-stub"
            />
            {EVAL.digSkillEvalStub}
          </label>
        </>
      )}
    </Modal>
  )
}
