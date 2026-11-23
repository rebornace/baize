import { splitChannelAttachments } from '../../channelImages'

export type GoalCandidate = {
  id: string
  text: string
  preview: string
}

const previewMax = 72
const textMaxRunes = 500

function truncateRunes(s: string, max: number): string {
  const runes = Array.from(s)
  if (runes.length <= max) return s
  return runes.slice(0, max).join('')
}

/** Recent user messages as goal pick options (newest first). */
export function goalCandidatesFromMessages(
  messages: { id: string; role: string; content: string }[],
  max = 20,
): GoalCandidate[] {
  const out: GoalCandidate[] = []
  const seen = new Set<string>()
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i]
    if (m.role !== 'user') continue
    const text = truncateRunes(splitChannelAttachments(m.content).text.replace(/\s+/g, ' ').trim(), textMaxRunes)
    if (!text) continue
    if (seen.has(text)) continue
    seen.add(text)
    const preview =
      Array.from(text).length > previewMax
        ? Array.from(text).slice(0, previewMax).join('') + '…'
        : text
    out.push({ id: m.id, text, preview })
    if (out.length >= max) break
  }
  return out
}
