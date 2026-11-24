/**
 * 部署形态的单一来源。
 *
 * 同一份前端会部署成两种形态：
 *  - 引擎自带站点（base `/`），保持 WeKnora 原品牌与导航；
 *  - C-Knowledge 子路径 / 独立域名（base `/c-knowledge/` 或
 *    `/c-knowledge-test/`，host 为 CK_HOSTS 之一），首页落在 `/home`，
 *    隐藏引擎品牌与外部链接。
 *
 * 这里只收敛「构建期/域名决定的事实」：base、CK 域名、首页、管理类路径。
 * 业务判断（登录后落点、菜单可见性、路由守卫）都消费这里，不再各自
 * 手抄一份 host 列表或路径名单。vite.config.ts 也 import 本文件，把
 * base/hosts 注入 HTML，保证 index.html / embed.html 与运行时代码同源。
 *
 * 注意：本文件不依赖任何 `@/` 别名或浏览器 API，便于 `tsx --test` 直接
 * 在 Node 下导入（`import.meta.env` 在 Node 中不存在，必须可选链）。
 */

/** 构建期 base，去掉末尾斜杠；引擎站点为 ''。 */
export const APP_BASE = (import.meta.env?.BASE_URL || '/').replace(/\/+$/, '')

/** C-Knowledge 自己的域名（独立子域形态，构建 base 为 `/`）。 */
export const CK_HOSTS = ['c-knowledge.c-smart.hk', 'c-knowledge-test.c-smart.hk'] as const

/** 是否跑在 C-Knowledge 自己的域名上。 */
export function isCKnowledgeHost(): boolean {
  if (typeof window === 'undefined') return false
  return (CK_HOSTS as readonly string[]).includes(window.location.hostname)
}

/** 是否是 C-Knowledge 控制台（子路径形态或独立域名形态）。 */
export function isCKnowledgeConsole(): boolean {
  if (APP_BASE === '/c-knowledge' || APP_BASE === '/c-knowledge-test') return true
  return isCKnowledgeHost()
}

/** C-Knowledge 控制台自有首页；引擎默认首页是知识库列表。 */
export const CK_HOME_PATH = '/home'

/**
 * 只对空间管理员开放的控制台路径前缀（不含 base，与 vue-router 的
 * `to.path` 对齐）。路由守卫、侧栏菜单、用户菜单快捷入口都消费这一份；
 * 权限的最终来源仍是服务端 RBAC，这里只是 UI 收口。
 */
export const ADMIN_ONLY_PREFIXES = [
  '/platform/artifacts',
  '/platform/agents',
  '/platform/toolbox',
  '/platform/settings',
  '/platform/organizations',
] as const

/** 路径是否属于管理类页面（精确匹配前缀本身或其子路径）。 */
export function isAdminOnlyPath(path: string): boolean {
  return ADMIN_ONLY_PREFIXES.some(
    (prefix) => path === prefix || path.startsWith(`${prefix}/`),
  )
}

/**
 * 侧栏菜单项（menuArr 的 `path` 字段，如 `artifacts`）是否属于管理类。
 * 供 menu store 与用户菜单共用，避免各自维护 key 数组。
 */
export function isAdminOnlyMenuKey(key: string): boolean {
  return isAdminOnlyPath(`/platform/${key}`)
}