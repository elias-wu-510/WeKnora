// 嵌入態判定：管理台被 Senior AI 以 iframe 內嵌時（`?embed=1`），收起 WeKnora 侧边栏，
// 让内容区占满；独立打开（新标签页）时仍保留完整导航。
//
// 判定顺序：
//   1. window.self !== window.top —— iframe 内为 true，跨域也安全（只比较引用）；
//   2. URL 上的 embed=1 —— 兼容宿主页显式传参；
//   3. sessionStorage —— Vue Router 内部跳转会丢掉 query，命中过就记住，避免中途侧边栏“跳回来”。
const STORAGE_KEY = 'ck.embedded-view'

let cached: boolean | null = null

export interface EmbeddedViewProbe {
  self?: unknown
  top?: unknown
  search?: string
  stored?: string | null
}

/** 纯函数版本，便于单测：不读全局状态，只按入参判定。 */
export function detectEmbeddedView(probe: EmbeddedViewProbe = {}): boolean {
  const selfRef = 'self' in probe ? probe.self : (typeof window === 'undefined' ? undefined : window.self)
  const topRef = 'top' in probe ? probe.top : (typeof window === 'undefined' ? undefined : window.top)
  if (selfRef && topRef && selfRef !== topRef) return true

  const search = probe.search ?? (typeof location === 'undefined' ? '' : location.search)
  if (new URLSearchParams(search).get('embed') === '1') return true

  return probe.stored === '1'
}

/** 运行时判定（带模块级缓存 + sessionStorage 兜底）。 */
export function isEmbeddedView(): boolean {
  if (cached !== null) return cached

  let stored: string | null = null
  try {
    stored = sessionStorage.getItem(STORAGE_KEY)
  } catch {
    // 隐私模式或存储被禁用：忽略，按当前窗口判定。
  }

  cached = detectEmbeddedView({ stored })

  try {
    if (cached) sessionStorage.setItem(STORAGE_KEY, '1')
    else sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // 同上，存储不可用不影响判定结果。
  }

  return cached
}

/** 仅供测试：清空模块级缓存。 */
export function resetEmbeddedViewCache(): void {
  cached = null
}