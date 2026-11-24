/**
 * 租户（空间）选择的唯一实现。
 *
 * 背景：过去租户选择在 4 个调用点各写一份（main.ts / router / App.vue /
 * Login.vue），叠加 ckConsole.ts 里的旧实现和前端硬编码的共享空间
 * `10000`，三次线上事故（全部 0、独立入口 0 库、切空间 404）都出在
 * 这条链上。现在所有入口都只调用本文件：
 *
 *   resolvePreferredTenant()  纯函数，决定「这次该选中哪个空间」；
 *   applyTenantPreference()   读服务端/本地偏好 + 写入 auth store。
 *
 * 优先级（C-Knowledge 控制台）：
 *   1. ck.preferred-tenant —— 本设备上的用户显式选择（宿主 `?tenant=`
 *      或空间切换器写入）。必须高于服务端 preferred：切回 home 时服务端
 *      会清空 last_active，而偏好写入又是硬刷新前的竞态请求，若让服务端
 *      先决，刷新会把刚切到的空间覆盖回去（任务硬性行为）。
 *   2. /auth/me 的 preferred_tenant_id —— 服务端按
 *      last_active → 共享空间 → home 解析，跨设备一致。
 *   3. weknora_selected_tenant_id —— 旧版本遗留的当前选择，服务端 preferred
 *      缺失时兜底。
 *   4. /auth/config 的 shared_tenant_id —— 仅当账号严格是该空间成员时采用；
 *      不再盲写 10000（取不到配置就不预设，回落 home/活跃空间）。
 *   5. home / 当前活跃空间 —— 不覆盖，交给原有页面逻辑。
 *
 * 引擎站点（非 C-Knowledge）保持原行为：不消费共享空间默认值，只在
 * 「服务端已把活跃空间切到非 home」时保留 X-Tenant-ID 覆盖。
 */

import { isCKnowledgeConsole } from '../config/deployment'

export const PREFERRED_TENANT_KEY = 'ck.preferred-tenant'
export const SELECTED_TENANT_KEY = 'weknora_selected_tenant_id'

export interface TenantMembershipLike {
  tenant_id?: number | string | null
  tenant_name?: string | null
}

/** 只依赖 localStorage 的读写子集，单测可注入内存实现。 */
export interface TenantPreferenceStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

export type TenantSelectionSource =
  | 'user'
  | 'server'
  | 'stored'
  | 'config'
  | 'active'
  | 'home'
  | 'none'

export interface TenantSelection {
  id: number | null
  name: string | null
  /** 命中的优先级层，便于日志/单测断言；不参与 UI。 */
  source: TenantSelectionSource
}

/** 把后端/宿主给的字符串或数字租户 id 归一化成正整数，否则 null。 */
export function toTenantId(value: number | string | null | undefined): number | null {
  if (value === null || value === undefined || value === '') return null
  const id = Number(value)
  return Number.isSafeInteger(id) && id > 0 ? id : null
}

function resolveStorage(
  storage?: TenantPreferenceStorage | null,
): TenantPreferenceStorage | null {
  if (storage !== undefined) return storage
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    // 隐私模式 / 沙箱里访问 localStorage 可能抛异常
    return null
  }
}

/** 宿主 `?tenant=` 或用户切换空间时记录的本机显式选择。 */
export function preferredTenant(
  storage?: TenantPreferenceStorage | null,
): number | null {
  try {
    return toTenantId(resolveStorage(storage)?.getItem(PREFERRED_TENANT_KEY))
  } catch {
    return null
  }
}

export function rememberPreferredTenant(
  id: number | string | null | undefined,
  storage?: TenantPreferenceStorage | null,
): void {
  const tenantId = toTenantId(id)
  if (tenantId === null) return
  try {
    resolveStorage(storage)?.setItem(PREFERRED_TENANT_KEY, String(tenantId))
  } catch {
    /* 隐私模式下 localStorage 不可写，忽略 */
  }
}

/**
 * 用户在下拉菜单里主动切换空间时调用。
 *
 * 宿主 `?tenant=` 写入的 preferred 优先级高于普通选择存储；不同步这里的话，
 * 切换后的硬刷新会被宿主默认空间覆盖回去（表现为「切了没反应」）。
 * 引擎站点不写这份 key（与历史行为一致）。
 */
export function rememberUserTenantChoice(
  id: number | string,
  storage?: TenantPreferenceStorage | null,
): void {
  if (!isCKnowledgeConsole()) return
  rememberPreferredTenant(id, storage)
}

export interface ResolvePreferredTenantOptions {
  /** 服务端 `/auth/me` 的 preferred_tenant_id。 */
  preferredId?: number | string | null
  /** 本机显式选择；不传则读 ck.preferred-tenant。 */
  userPreferredId?: number | string | null
  /** 旧版 weknora_selected_tenant_id；不传则读 localStorage。 */
  storedId?: number | string | null
  /** `/auth/config` 的 shared_tenant_id；不传视为无配置。 */
  configDefaultId?: number | string | null
  activeTenantId?: number | string | null
  activeTenantName?: string | null
  homeTenantId?: number | string | null
  memberships?: TenantMembershipLike[] | null
  /** 是否 C-Knowledge 控制台；不传则按当前构建/base 判断。 */
  console?: boolean
  storage?: TenantPreferenceStorage | null
}

/**
 * 纯函数：按优先级算出这次要选中的空间。
 *
 * `memberships` 为空/缺失时无法校验，先信任本地/服务端给的 id（服务端
 * 仍会做最终鉴权）；只有「配置默认共享空间」这一层要求严格成员命中，
 * 否则非成员会带着 X-Tenant-ID 触发 403。
 */
export function resolvePreferredTenant(
  options: ResolvePreferredTenantOptions,
): TenantSelection {
  const consoleMode = options.console ?? isCKnowledgeConsole()
  const memberships = Array.isArray(options.memberships) ? options.memberships : null
  const hasMemberships = !!memberships && memberships.length > 0

  const nameOf = (id: number): string | null => {
    const match = (memberships || []).find((m) => toTenantId(m.tenant_id) === id)
    return match?.tenant_name ?? null
  }
  const isMember = (id: number): boolean => {
    if (!hasMemberships) return true
    return (memberships as TenantMembershipLike[]).some(
      (m) => toTenantId(m.tenant_id) === id,
    )
  }
  const isStrictMember = (id: number): boolean =>
    hasMemberships &&
    (memberships as TenantMembershipLike[]).some(
      (m) => toTenantId(m.tenant_id) === id,
    )

  const pick = (
    raw: number | string | null | undefined,
    source: TenantSelectionSource,
    strict = false,
  ): TenantSelection | null => {
    const id = toTenantId(raw)
    if (id === null) return null
    if (strict ? !isStrictMember(id) : !isMember(id)) return null
    return { id, name: nameOf(id), source }
  }

  const activeId = toTenantId(options.activeTenantId)
  const homeId = toTenantId(options.homeTenantId)

  if (!consoleMode) {
    // 引擎站点：沿用旧逻辑——仅当服务端已把活跃空间切到非 home 时保留覆盖。
    if (activeId !== null && homeId !== null && activeId !== homeId) {
      return { id: activeId, name: options.activeTenantName ?? null, source: 'active' }
    }
    return { id: null, name: null, source: homeId !== null ? 'home' : 'none' }
  }

  const storage = resolveStorage(options.storage)
  let storedRaw: number | string | null | undefined = options.storedId
  if (storedRaw === undefined) {
    try {
      storedRaw = storage?.getItem(SELECTED_TENANT_KEY)
    } catch {
      storedRaw = null
    }
  }
  const userPreferredRaw =
    options.userPreferredId !== undefined
      ? options.userPreferredId
      : preferredTenant(options.storage)

  const selection =
    pick(userPreferredRaw, 'user') ??
    pick(options.preferredId, 'server') ??
    pick(storedRaw, 'stored') ??
    pick(options.configDefaultId, 'config', true)
  if (selection) return selection

  if (activeId !== null && homeId !== null && activeId !== homeId) {
    return { id: activeId, name: options.activeTenantName ?? null, source: 'active' }
  }
  return { id: null, name: null, source: homeId !== null ? 'home' : 'none' }
}

/** 进程内缓存 /auth/config 的 shared_tenant_id，避免每个入口各拉一次。 */
let cachedSharedTenantId: number | null = null

export function rememberSharedTenantId(value: number | string | null | undefined): void {
  const id = toTenantId(value)
  if (id !== null) cachedSharedTenantId = id
}

/**
 * 懒加载 /auth/config 的 shared_tenant_id；失败返回 null（不预设空间）。
 * 无配置时不要回退写死 10000：非成员带 10000 会被服务端 403。
 */
export async function loadSharedTenantId(): Promise<number | null> {
  if (cachedSharedTenantId !== null) return cachedSharedTenantId
  try {
    const { getAuthConfig } = await import('@/api/auth')
    const config = await getAuthConfig()
    if (!config.success) return null
    const id = toTenantId(config.shared_tenant_id)
    if (id !== null) cachedSharedTenantId = id
    return id
  } catch {
    return null
  }
}

export interface ApplyTenantPreferenceOptions {
  preferredId?: number | string | null
  activeTenantId?: number | string | null
  activeTenantName?: string | null
  homeTenantId?: number | string | null
  memberships?: TenantMembershipLike[] | null
  console?: boolean
  /** 显式传入配置默认值；不传则（CK 下）懒加载 /auth/config。 */
  configDefaultId?: number | string | null
  setSelectedTenant: (id: number | null, name: string | null) => void
  storage?: TenantPreferenceStorage | null
  /** 单测注入点，默认 loadSharedTenantId。 */
  loadConfigDefault?: () => Promise<number | null>
}

/**
 * 读取服务端/本地偏好并把结果写进 auth store。所有进入应用（启动、路由
 * 水合、OIDC 回调、登录）的路径都调用这一个入口。
 */
export async function applyTenantPreference(
  options: ApplyTenantPreferenceOptions,
): Promise<TenantSelection> {
  const consoleMode = options.console ?? isCKnowledgeConsole()
  const resolve = (configDefaultId?: number | string | null) =>
    resolvePreferredTenant({
      preferredId: options.preferredId,
      configDefaultId,
      activeTenantId: options.activeTenantId,
      activeTenantName: options.activeTenantName,
      homeTenantId: options.homeTenantId,
      memberships: options.memberships,
      console: consoleMode,
      storage: options.storage,
    })

  let selection = resolve(options.configDefaultId)
  // 只有前三层（本机显式选择 / 服务端 / 旧选择）都没命中时才需要配置默认值；
  // 避免每次启动都为一个备选层多打一次 /auth/config。
  const needsConfig =
    selection.source === 'active' ||
    selection.source === 'home' ||
    selection.source === 'none'
  if (consoleMode && needsConfig && options.configDefaultId === undefined) {
    const configDefaultId = await (options.loadConfigDefault ?? loadSharedTenantId)()
    if (configDefaultId !== null) selection = resolve(configDefaultId)
  }

  options.setSelectedTenant(selection.id, selection.name)
  return selection
}