/**
 * C-Knowledge 品牌化控制台的兼容入口。
 *
 * 部署形态判定已收敛到 `@/config/deployment`，租户（空间）选择收敛到
 * `@/utils/tenantPreference`。本文件只保留 re-export（让既有 import 不至于
 * 立刻断裂）以及品牌/支持链接这类控制台专属内容，不再承载任何判定逻辑。
 */

import { CK_HOME_PATH, isCKnowledgeConsole } from '../config/deployment'

export {
  APP_BASE,
  CK_HOSTS,
  CK_HOME_PATH,
  isAdminOnlyMenuKey,
  isAdminOnlyPath,
  isCKnowledgeConsole,
  isCKnowledgeHost,
  ADMIN_ONLY_PREFIXES,
} from '../config/deployment'

export {
  PREFERRED_TENANT_KEY,
  preferredTenant,
  rememberPreferredTenant,
  rememberUserTenantChoice,
  toTenantId,
  type TenantMembershipLike,
  type TenantPreferenceStorage,
} from './tenantPreference'

/** 登录完成后的落点：无空间先去 onboarding；有空间时 C-Knowledge 回自有首页。 */
export function afterLoginPath(hasValidTenant: boolean): string {
  if (!hasValidTenant) return '/onboarding/workspace'
  return isCKnowledgeConsole() ? CK_HOME_PATH : '/platform/knowledge-bases'
}

/**
 * C-Knowledge 的支持入口。上游引擎的帮助文档 / 报错提单入口指向
 * github.com/Tencent/WeKnora，白标产品里必须换成 Senior AI 自己的支持渠道。
 */
export const CK_SUPPORT_URL = 'https://llm-ai.c-smart.hk/senior-ai/settings#help'
export const CK_SUPPORT_EMAIL = 'support@senior-ai.dev'

/**
 * 隐藏仍然指向原厂仓库的文档/提单入口（设置弹窗、集成页等都是懒加载的，
 * 所以用一个 body 级观察器兜住）。支持入口已由 `CK_SUPPORT_URL` 单独提供。
 */
export function hideUpstreamBrandLinks(): () => void {
  if (!isCKnowledgeConsole() || typeof document === 'undefined') return () => {}
  const selector = 'a[href*="github.com/Tencent/WeKnora"]'
  const hide = (root: ParentNode) => {
    root.querySelectorAll<HTMLAnchorElement>(selector).forEach((el) => {
      el.style.display = 'none'
    })
  }
  hide(document)
  const observer = new MutationObserver((records) => {
    for (const record of records) {
      record.addedNodes.forEach((node) => {
        if (!(node instanceof HTMLElement)) return
        if (node.matches(selector)) node.style.display = 'none'
        hide(node)
      })
    }
  })
  observer.observe(document.body, { childList: true, subtree: true })
  return () => observer.disconnect()
}