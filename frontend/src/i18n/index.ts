import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN.ts'
import ruRU from './locales/ru-RU.ts'
import enUS from './locales/en-US.ts'
import koKR from './locales/ko-KR.ts'
import jaJP from './locales/ja-JP.ts'
import { BUILT_IN_DEFAULT, resolveDefaultLocale } from './resolveDefaultLocale.ts'
import { isCKnowledgeConsole } from '@/utils/ckConsole'

const baseMessages = {
  'zh-CN': zhCN,
  'en-US': enUS,
  'ru-RU': ruRU,
  'ko-KR': koKR,
  'ja-JP': jaJP
}

// C-Knowledge 是白标产品：界面文案里的引擎品牌统一换成本产品名。
// 只替换展示文本，URL（文档/仓库链接）保持原样，避免把外链换坏。
function whiteLabelMessage(value: string): string {
  return value.replace(/(https?:\/\/[^\s)]+)|WeKnora/g, (match) =>
    match.startsWith('http') ? match : 'C-Knowledge',
  )
}

function whiteLabelDeep<T>(input: T): T {
  if (typeof input === 'string') return whiteLabelMessage(input) as unknown as T
  if (Array.isArray(input)) return input.map((item) => whiteLabelDeep(item)) as unknown as T
  if (input && typeof input === 'object') {
    const out: Record<string, unknown> = {}
    for (const [key, value] of Object.entries(input as Record<string, unknown>)) {
      out[key] = whiteLabelDeep(value)
    }
    return out as unknown as T
  }
  return input
}

const messages = isCKnowledgeConsole() ? whiteLabelDeep(baseMessages) : baseMessages

// User's explicit past choice wins; otherwise use the deployment default.
const savedLocale = localStorage.getItem('locale') || resolveDefaultLocale(
  window.__RUNTIME_CONFIG__?.DEFAULT_LOCALE,
  import.meta.env.VITE_DEFAULT_LOCALE,
)

const i18n = createI18n({
  legacy: false,
  locale: savedLocale,
  fallbackLocale: BUILT_IN_DEFAULT,
  globalInjection: true,
  // Some translations intentionally embed `<strong>` markup (e.g. agent step summaries).
  // We render them via v-html with our own sanitization, so silence vue-i18n's HTML warning
  // to avoid flooding the console and slowing renders during history loads.
  warnHtmlMessage: false,
  messages
})

export default i18n
