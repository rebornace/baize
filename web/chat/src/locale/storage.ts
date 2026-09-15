import { isLocale, LOCALE_STORAGE_KEY, type Locale } from './types'

export function readStoredLocale(storage: Pick<Storage, 'getItem'> = localStorage): Locale | null {
  try {
    const raw = storage.getItem(LOCALE_STORAGE_KEY)
    return isLocale(raw) ? raw : null
  } catch {
    return null
  }
}

export function writeStoredLocale(
  locale: Locale,
  storage: Pick<Storage, 'setItem'> = localStorage,
): void {
  try {
    storage.setItem(LOCALE_STORAGE_KEY, locale)
  } catch {
    // ignore quota / private mode
  }
}
