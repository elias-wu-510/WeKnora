/**
 * C-Knowledge 品牌化控制台。
 *
 * 同一份前端会在两种部署形态下构建：
 *  - 引擎自带站点（base `/`）：保持 WeKnora 原品牌与导航。
 *  - C-Knowledge 子路径（base `/c-knowledge/`）：面向 Senior AI 使用者的
 *    嵌入式知识库管理台，首页落在 `/home`，隐藏引擎品牌与外部链接。
 *
 * 只用构建期 base 判断，避免运行时被 `?embed=1` 等参数误伤（嵌入聊天
 * 页与嵌入管理台共用同一个 base，品牌替换对两者都成立）。
 */
export const CK_HOSTS = ['c-knowledge.c-smart.hk', 'c-knowledge-test.c-smart.hk'] as const

/** 是否跑在 C-Knowledge 自己的域名上（独立子域形态，构建 base 为 `/`）。 */
export function isCKnowledgeHost(): boolean {
  if (typeof window === 'undefined') return false
  return (CK_HOSTS as readonly string[]).includes(window.location.hostname)
}

export function isCKnowledgeConsole(): boolean {
  const base = (import.meta.env.BASE_URL || '/').replace(/\/+$/, '')
  // 子路径形态（/c-knowledge/、/c-knowledge-test/）或独立域名形态都算 CK。
  if (base === '/c-knowledge' || base === '/c-knowledge-test') return true
  return isCKnowledgeHost()
}

/** C-Knowledge 控制台自有首页；引擎默认首页是知识库列表。 */
export const CK_HOME_PATH = '/home'

/** 登录完成后的落点：无空间先去 onboarding；有空间时 C-Knowledge 回自有首页。 */
export function afterLoginPath(hasValidTenant: boolean): string {
  if (!hasValidTenant) return '/onboarding/workspace'
  return isCKnowledgeConsole() ? CK_HOME_PATH : '/platform/knowledge-bases'
}

/**
 * 共享空间偏好。
 *
 * 宿主（Senior AI）用 `?tenant=<id>` 指定要进入的共享空间，但 OIDC 回调页
 * 在另一个窗口打开、地址上没有该参数；引擎原有的“活跃空间 == 个人空间时
 * 清掉覆盖值”逻辑会把偏好抹掉，重载后 `/auth/me` 落回个人空间，管理台就
 * 会显示「全部 0」。把偏好单独记一份，后续每次进入控制台都能恢复。
 */
const PREFERRED_TENANT_KEY = 'ck.preferred-tenant'

/**
 * 独立入口的默认共享空间。
 *
 * 从 Senior AI 嵌入打开时宿主会带 `?tenant=<id>`；但用户直接访问
 * `/c-knowledge/` 完成 OIDC 登录时地址上没有任何提示，引擎会把活跃空间
 * 落回个人空间，管理台就显示「全部 0」。C-Knowledge 的知识库都放在共享
 * 空间，因此这里提供构建期默认值：只有在该账号确实是成员时才采用，
 * 避免服务端 `insufficient permissions` 403。
 */
export const CK_DEFAULT_TENANT: number = (() => {
  const raw = String(import.meta.env.VITE_CK_DEFAULT_TENANT || '10000').trim()
  const id = Number(raw)
  return Number.isInteger(id) && id > 0 ? id : 10000
})()

/** 把后端/宿主给的字符串或数字租户 id 归一化成正整数，否则 null。 */
function toTenantId(value: number | string | null | undefined): number | null {
  if (value === null || value === undefined || value === '') return null
  const id = Number(value)
  return Number.isInteger(id) && id > 0 ? id : null
}

export function rememberPreferredTenant(id: number): void {
  if (Number.isInteger(id) && id > 0) {
    try {
      localStorage.setItem(PREFERRED_TENANT_KEY, String(id))
    } catch {
      /* 隐私模式下 localStorage 不可写，忽略 */
    }
  }
}

/**
 * 用户在下拉菜单里主动切换空间时调用。
 *
 * 宿主 `?tenant=` 写入的 preferred 优先级高于普通选择存储；不同步这里的话，
 * 切换后的硬刷新会被宿主默认空间覆盖回去（表现为「切了没反应」）。
 */
export function rememberUserTenantChoice(id: number): void {
  if (!isCKnowledgeConsole()) return
  rememberPreferredTenant(id)
}

export function preferredTenant(): number | null {
  try {
    const raw = localStorage.getItem(PREFERRED_TENANT_KEY)
    if (!raw) return null
    const id = Number(raw)
    return Number.isInteger(id) && id > 0 ? id : null
  } catch {
    return null
  }
}

export interface TenantMembershipLike {
  tenant_id?: number | string | null
  tenant_name?: string | null
}

/**
 * 决定登录后要不要带 X-Tenant-ID 覆盖值。
 * 优先宿主指定的共享空间（且当前账号确实在其中），其次沿用后端的活跃空间，
 * 都没有则清空覆盖值，回到个人空间。
 */
export function resolveTenantSelection(input: {
  activeTenantId?: number | null
  activeTenantName?: string | null
  homeTenantId?: number | null
  memberships?: TenantMembershipLike[] | null
}): { id: number | null; name: string | null } {
  const { activeTenantId, activeTenantName, homeTenantId, memberships } = input
  if (isCKnowledgeConsole()) {
    const known = (id: number | null) => {
      if (!id) return null
      const match = (memberships || []).find(m => Number(m.tenant_id) === id)
      // 没有 memberships 信息时也先信任它，服务端会再做 membership 校验。
      if (match || !memberships || memberships.length === 0) {
        return { id, name: match?.tenant_name ?? null }
      }
      return null
    }
    // 宿主声明的共享空间优先。
    const fromHost = known(preferredTenant())
    if (fromHost) return fromHost
    // 其次尊重「当前已经选中的空间」：嵌入管理台由宿主预选共享空间，
    // OIDC 回调窗口是另一次页面启动，不能因为活跃空间恰好等于个人空间
    // 就把这个有效选择清掉（否则刷新后又回到「全部 0」）。
    const stored = Number(localStorage.getItem('weknora_selected_tenant_id') || '')
    const fromStore = Number.isInteger(stored) && stored > 0 ? known(stored) : null
    if (fromStore) return fromStore
    // 独立入口（无 ?tenant=、无历史偏好）的兜底：默认进共享空间。
    // 这里必须严格要求成员关系命中，不能用 known() 的「无成员列表也信任」
    // 分支，否则非成员会带着 X-Tenant-ID 触发服务端 403。
    const shared = (memberships || []).find(m => Number(m.tenant_id) === CK_DEFAULT_TENANT)
    if (shared) return { id: CK_DEFAULT_TENANT, name: shared.tenant_name ?? null }
  }
  if (
    Number.isFinite(activeTenantId) &&
    Number.isFinite(homeTenantId) &&
    activeTenantId !== homeTenantId
  ) {
    return { id: activeTenantId as number, name: activeTenantName ?? null }
  }
  return { id: null, name: null }
}

/**
 * 启动/登录兜底：成员关系已知后同步一次控制台空间选择。
 *
 * 只在 C-Knowledge 控制台生效，官方引擎站点调用方需自行跳过。与
 * `resolveTenantSelection` 的区别仅是接受后端原样的字符串 id 并做归一化，
 * 供 main.ts / 路由守卫在 `/auth/me` 返回后直接调用。
 */
export function resolveConsoleBootSelection(input: {
  activeTenantId?: number | string | null
  activeTenantName?: string | null
  homeTenantId?: number | string | null
  memberships?: TenantMembershipLike[] | null
}): { id: number | null; name: string | null } {
  if (!isCKnowledgeConsole()) return { id: null, name: null }
  return resolveTenantSelection({
    activeTenantId: toTenantId(input.activeTenantId),
    activeTenantName: input.activeTenantName ?? null,
    homeTenantId: toTenantId(input.homeTenantId),
    memberships: input.memberships,
  })
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
