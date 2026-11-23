import { useEffect, useState } from 'react'
import type { ConversationExportFormat } from '../../api'
import { Button, Modal } from '../../components/ui'
import { CHAT } from '../../strings'

export type ConversationExportDialogProps = {
  open: boolean
  busy: boolean
  onClose: () => void
  onExport: (format: ConversationExportFormat, includeThinking: boolean) => void
}

const formats: Array<{ value: ConversationExportFormat; label: string }> = [
  { value: 'markdown', label: 'Markdown' },
  { value: 'json', label: 'JSON' },
  { value: 'html', label: 'HTML' },
]

export function ConversationExportDialog({
  open,
  busy,
  onClose,
  onExport,
}: ConversationExportDialogProps) {
  const [format, setFormat] = useState<ConversationExportFormat>('markdown')
  const [includeThinking, setIncludeThinking] = useState(false)

  useEffect(() => {
    if (!open) {
      setFormat('markdown')
      setIncludeThinking(false)
    }
  }, [open])

  return (
    <Modal
      open={open}
      title={CHAT.exportDialogTitle}
      onClose={busy ? () => {} : onClose}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            {CHAT.exportCancel}
          </Button>
          <Button variant="primary" onClick={() => onExport(format, includeThinking)} disabled={busy}>
            {busy ? CHAT.exportBusy : CHAT.exportDownload}
          </Button>
        </>
      }
    >
      <fieldset className="export-formats">
        <legend>{CHAT.exportFormat}</legend>
        {formats.map((item) => (
          <label className="export-format" key={item.value}>
            <input
              type="radio"
              name="conversation-export-format"
              value={item.value}
              checked={format === item.value}
              onChange={() => setFormat(item.value)}
            />
            <span>{item.label}</span>
          </label>
        ))}
      </fieldset>
      <label className="export-thinking">
        <input
          type="checkbox"
          checked={includeThinking}
          onChange={(e) => setIncludeThinking(e.target.checked)}
        />
        {CHAT.exportIncludeThinking}
      </label>
      <p className="export-hint">{CHAT.exportHint}</p>
    </Modal>
  )
}
