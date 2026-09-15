import type { Locale } from '../locale/types'
import { enPack } from './en'
import { zhPack, type StringsPack } from './zh'

export type { StringsPack }
export { zhPack, enPack }

export const packs: Record<Locale, StringsPack> = {
  'zh-CN': zhPack,
  en: enPack,
}
