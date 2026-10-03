import type { StringsPack } from '../locales/zh'

let current: StringsPack | null = null
const listeners = new Set<(pack: StringsPack) => void>()

/** Current locale pack; must be set by LocaleProvider or tests via setPack. */
export function getPack(): StringsPack {
  if (!current) {
    throw new Error('locale pack not initialized; wrap with LocaleProvider or call setPack()')
  }
  return current
}

export function setPack(pack: StringsPack): void {
  current = pack
  for (const listener of listeners) listener(pack)
}

export function subscribePack(listener: (pack: StringsPack) => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/** Test helper: true when a pack is installed. */
export function hasPack(): boolean {
  return current !== null
}
