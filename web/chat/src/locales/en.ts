import { zhPack, type StringsPack } from './zh'

/**
 * Temporary en pack for task 3: Chinese base with a few English probes
 * so LocaleProvider / live-string tests can detect switches.
 * Full English lands in task 4.
 */
export const enPack = {
  ...zhPack,
  AUTO_LABEL: 'Auto select',
  ACTIONS: {
    ...zhPack.ACTIONS,
    copy: 'Copy',
  },
  CHAT: {
    ...zhPack.CHAT,
    copySuccess: 'Copied',
  },
} as StringsPack
