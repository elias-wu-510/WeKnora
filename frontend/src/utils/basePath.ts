/**
 * 部署前缀工具。
 *
 * 同一份前端会以不同 base 构建：官方站点是 `/`，C-Knowledge 是
 * `/c-knowledge/`（测试车道 `/c-knowledge-test/`）。所有「整页跳转」的
 * 绝对路径都必须经过这里，否则会跳到站点根、被 nginx 404。
 */

/** 归一化后的构建 base，不带结尾斜杠；根路径返回空串。 */
export function appBasePath(): string {
  return (import.meta.env.BASE_URL || '/').replace(/\/+$/, '')
}

/** 把应用内绝对路径（如 `/login`）拼上构建 base。 */
export function withAppBase(path: string): string {
  const normalized = path.startsWith('/') ? path : `/${path}`
  return `${appBasePath()}${normalized}`
}

/** 当前 location.pathname 是否落在某个应用内路径下（含其子路径）。 */
export function isPathAtOrUnder(path: string): boolean {
  const target = withAppBase(path)
  const current = window.location.pathname
  return current === target || current.startsWith(`${target}/`)
}