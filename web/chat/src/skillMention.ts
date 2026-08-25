// Lightweight client-side parsing of @id and /id skill mentions, aligned with
// the server-side skillparse.Parse rules. The server remains the authority for
// run creation; these helpers only drive completion UI and inline highlighting.
//
// Backend rule (internal/skillparse/parse.go):
//   mentionRe = `(^|[\s])[@/]([a-zA-Z0-9][a-zA-Z0-9_-]*)\b`

const MENTION_RE = /(^|[\s])[@/]([a-zA-Z0-9][a-zA-Z0-9_-]*)\b/g
const ID_CHAR = /[a-zA-Z0-9_-]/
const ID_START = /[a-zA-Z0-9]/

export interface MentionMatch {
  /** Index of the trigger character (@ or /). */
  start: number
  /** Index just past the matched id (exclusive). */
  end: number
  /** The trigger character, '@' or '/'. */
  trigger: string
  /** The matched skill id (without the trigger). */
  id: string
}

/** Find all @id and /id mentions in text, mirroring the server regex. */
export function findMentions(text: string): MentionMatch[] {
  const out: MentionMatch[] = []
  MENTION_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = MENTION_RE.exec(text)) !== null) {
    const prefixLen = m[1].length
    const triggerStart = m.index + prefixLen
    const idStart = triggerStart + 1
    out.push({
      start: triggerStart,
      end: idStart + m[2].length,
      trigger: text[triggerStart],
      id: m[2],
    })
    // Guard against zero-length matches looping forever.
    if (m[0].length === 0) MENTION_RE.lastIndex++
  }
  return out
}

export interface ActiveMention {
  /** Index of the trigger character. */
  start: number
  /** Caret index (exclusive end of the query). */
  end: number
  /** The trigger character, '@' or '/'. */
  trigger: string
  /** The partial id typed so far (may be empty when only the trigger is present). */
  query: string
}

/**
 * Detect whether the caret sits inside an @id or /id mention being typed. Used
 * to surface the skill completion popup. Mirrors the server's "trigger must be
 * at start or preceded by whitespace" rule so the popup only opens for real
 * mentions, not for `@` in the middle of a word.
 */
export function activeMention(text: string, caret: number): ActiveMention | null {
  if (caret <= 0) return null
  let i = caret - 1
  while (i >= 0 && ID_CHAR.test(text[i])) i--
  if (i < 0) return null
  const trigger = text[i]
  if (trigger !== '@' && trigger !== '/') return null
  if (i > 0 && !/\s/.test(text[i - 1])) return null
  const query = text.slice(i + 1, caret)
  if (query.length > 0 && !ID_START.test(query[0])) return null
  return { start: i, end: caret, trigger, query }
}

/**
 * Replace the active mention at [start, end) with the selected skill id and
 * return the new text plus the caret position to place after the inserted id.
 * A trailing space is added so the user can keep typing the message.
 */
export function replaceMention(
  text: string,
  start: number,
  end: number,
  id: string,
): { text: string; caret: number } {
  const before = text.slice(0, start)
  const after = text.slice(end)
  const inserted = `@${id} `
  const next = before + inserted + after
  return { text: next, caret: before.length + inserted.length }
}
