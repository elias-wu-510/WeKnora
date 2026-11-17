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
