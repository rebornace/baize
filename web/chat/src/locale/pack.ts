import type { StringsPack } from '../locales/zh'

let current: StringsPack | null = null

/** Current locale pack; must be set by LocaleProvider or tests via setPack. */
export function getPack(): StringsPack {
  if (!current) {
    throw new Error('locale pack not initialized; wrap with LocaleProvider or call setPack()')
  }
  return current
}

export function setPack(pack: StringsPack): void {
  current = pack
}

/** Test helper: true when a pack is installed. */
export function hasPack(): boolean {
  return current !== null
}
