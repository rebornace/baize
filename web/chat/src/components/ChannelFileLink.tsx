import {
  Download,
  File as FileIcon,
  FileArchive,
  FileImage,
  FileSpreadsheet,
  FileText,
  FileType2,
  Trash2,
} from 'lucide-react'
import type { ComponentType } from 'react'
import { useState } from 'react'
import { authHeaders } from '../controlAuth'
import { useGate } from '../gateContext'
import { CHAT } from '../strings'
import { deleteChannelMedia, isDeletableMediaUrl } from '../api/channelMedia'
import { PdfPreviewButton } from './PdfPreviewButton'

export interface ChannelFileLinkProps {
  name: string
  url: string
}

interface Kind {
  icon: ComponentType<{ size?: number | string; className?: string }>
  label: string
}

export function fileKind(name: string): Kind {
  const dot = name.lastIndexOf('.')
  const ext = dot > 0 ? name.slice(dot + 1).toLowerCase() : ''
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg'].includes(ext)) {
    return { icon: FileImage, label: CHAT.fileKindImage }
  }
  if (['xls', 'xlsx', 'csv'].includes(ext)) {
    return { icon: FileSpreadsheet, label: CHAT.fileKindSheet }
  }
  if (['zip', 'rar', '7z', 'tar', 'gz'].includes(ext)) {
    return { icon: FileArchive, label: CHAT.fileKindArchive }
  }
  if (['doc', 'docx', 'txt', 'md', 'pdf'].includes(ext)) {
    return { icon: FileText, label: CHAT.fileKindDoc }
  }
  if (['ppt', 'pptx'].includes(ext)) {
    return { icon: FileType2, label: CHAT.fileKindDeck }
  }
  return { icon: FileIcon, label: ext ? ext.toUpperCase() : CHAT.fileKindFile }
}

export function isPdfFile(name: string): boolean {
  return name.slice(name.lastIndexOf('.') + 1).toLowerCase() === 'pdf'
}

/**
 * Renders a download card for an inbound channel file (docx/pdf/zip/…). The
 * endpoint is conversation-ACL protected and a plain <a href> cannot send the
 * Bearer token, so we fetch with auth headers and trigger a download through a
 * blob URL (same approach as artifact pages / inline images).
 */
export function ChannelFileLink({ name, url }: ChannelFileLinkProps) {
  const { gateEnabled } = useGate()
  const kind = fileKind(name)
  const Icon = kind.icon
  const [deleted, setDeleted] = useState(false)
  const [deleteFailed, setDeleteFailed] = useState(false)
  const canDelete = isDeletableMediaUrl(url)

  const download = async () => {
    // Local optimistic preview (blob:): download the object URL directly.
    if (url.startsWith('blob:')) {
      const a = document.createElement('a')
      a.href = url
      a.download = name
      document.body.appendChild(a)
      a.click()
      a.remove()
      return
    }
    try {
      const res = await fetch(url, {
        headers: gateEnabled ? (authHeaders(true) as Record<string, string>) : undefined,
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const blob = await res.blob()
      const objectUrl = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = objectUrl
      a.download = name
      document.body.appendChild(a)
      a.click()
      a.remove()
      // Revoke after the click has been processed.
      window.setTimeout(() => URL.revokeObjectURL(objectUrl), 30_000)
    } catch {
      // Fallback: open the URL directly (works when the gate is off).
      window.open(url, '_blank', 'noopener,noreferrer')
    }
  }

  const remove = async () => {
    if (!window.confirm(CHAT.deleteAttachmentConfirm)) return
    const ok = await deleteChannelMedia(url, name, gateEnabled)
    if (ok) {
      setDeleted(true)
      setDeleteFailed(false)
    } else {
      setDeleteFailed(true)
    }
  }

  if (deleted) {
    return <span className="channel-attachment-deleted">{CHAT.attachmentDeleted}</span>
  }

  return (
    <div className="channel-file">
      <button
        type="button"
        className="channel-file-link"
        onClick={() => void download()}
        title={CHAT.clickToDownload}
      >
        <span className="channel-file-icon">
          <Icon size={18} />
        </span>
        <span className="channel-file-meta">
          <span className="channel-file-name">{name}</span>
          <span className="channel-file-kind">{kind.label}</span>
        </span>
        <Download size={16} className="channel-file-dl" aria-hidden="true" />
      </button>
      {isPdfFile(name) ? <PdfPreviewButton url={url} name={name} /> : null}
      {canDelete ? (
        <button
          type="button"
          className="channel-file-delete"
          aria-label={CHAT.deleteAttachment}
          title={CHAT.deleteAttachment}
          onClick={() => void remove()}
        >
          <Trash2 size={14} aria-hidden="true" />
        </button>
      ) : null}
      {deleteFailed ? (
        <span className="channel-attachment-error">{CHAT.deleteAttachmentFailed}</span>
      ) : null}
    </div>
  )
}
