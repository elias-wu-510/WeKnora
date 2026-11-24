import { APP_BASE } from '@/config/deployment'

/**
 * API 的基础路径。
 *
 * base（`/c-knowledge/`、`/c-knowledge-test/`、LocalHub 的 `/app/weknora/`
 * 或引擎站点 `/`）只在 `config/deployment.ts` 里解析一次；这里保留函数形态
 * 供既有调用方使用，避免再抄一份 `import.meta.env.BASE_URL` 的去尾斜杠逻辑。
 * 去尾斜杠可避免 axios 拼出 `/app/weknora//api/v1/...`（LocalHub 反代场景）。
 */
export function getApiBaseUrl(): string {
  return APP_BASE
}
