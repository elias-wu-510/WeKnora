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
export function isCKnowledgeConsole(): boolean {
  const base = (import.meta.env.BASE_URL || '/').replace(/\/+$/, '')
  return base === '/c-knowledge'
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

export function rememberPreferredTenant(id: number): void {
  if (Number.isInteger(id) && id > 0) {
    try {
      localStorage.setItem(PREFERRED_TENANT_KEY, String(id))
    } catch {
      /* 隐私模式下 localStorage 不可写，忽略 */
    }
  }
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
