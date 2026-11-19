/**
 * Parse-status presentation helpers shared by the document card view, the
 * document list view and the parse-detail drawer.
 *
 * The backend reports `parse_status` plus, for in-flight rows, a
 * `last_activity_at` / `stall_state` pair (see GetKnowledgeBatch). The parent
 * page already turns those into `stalled_minutes`, so this module only deals
 * with presentation: one label, one tone and one hint per document, plus a
 * human-readable digest of the raw backend error.
 *
 * Copy is intentionally Simplified Chinese: C-Knowledge is the white-label
 * console and does not ship the WeKnora locale files.
 */

export interface ParseStatusItem {
  parse_status?: string
  summary_status?: string
  stalled_minutes?: number
  stall_state?: string
  error_message?: string
  updated_at?: string
}

export type ParseStatusKind =
  | 'queued'
  | 'processing'
  | 'finalizing'
  | 'summary'
  | 'completed'
  | 'failed'
  | 'stalled'
  | 'draft'
  | 'cancelled'
  | 'unknown'

export type ParseStatusTone = 'neutral' | 'active' | 'success' | 'danger' | 'warning'

export interface ParseStatusView {
  kind: ParseStatusKind
  tone: ParseStatusTone
  /** Short badge label. */
  label: string
  /** One sentence explaining what is happening and what to do next. */
  hint: string
  icon: string
  /** Rotating icon: work is actively moving. */
  spin: boolean
  /** Soft pulse: the row needs attention but is not progressing. */
  pulse: boolean
  /** The document has not reached a terminal state yet. */
  inFlight: boolean
  /** Retry is safe and meaningful from the UI. */
  canRetry: boolean
  /** The badge should not dominate the surface (completed rows). */
  quiet: boolean
  /** Minutes without progress, only while stalled. */
  stalledMinutes: number
}

export interface ParseErrorSummary {
  /** One readable sentence, never empty. */
  summary: string
  /** Raw backend text, normalised; empty when it adds nothing. */
  detail: string
}

const IN_FLIGHT = new Set(['pending', 'processing', 'finalizing'])
const MAX_SUMMARY_CHARS = 80

/** Collapse whitespace while keeping the original line breaks for details. */
export function normalizeParseError(raw?: string | null): string {
  return String(raw ?? '')
    .replace(/\r\n?/g, '\n')
    .replace(/[ \t]*\n[ \t]*/g, '\n')
    .replace(/[ \t]+/g, ' ')
    .replace(/\n{3,}/g, '\n\n')
    .replace(/^(?:error|failed|failure|解析失败)\s*[:：]\s*/i, '')
    .trim()
}

/**
 * Map the most common backend failure shapes onto plain Chinese so the badge
 * can stay one short sentence. Unknown errors keep their first sentence; the
 * full text is always available as `detail`.
 */
export function friendlyParseError(raw: string): string {
  const text = raw.toLowerCase()
  if (/deadline exceeded|timed?\s*out|timeout|超时/.test(text)) return '解析超时，请稍后重试'
  if (/password|encrypted|加密/.test(text)) return '文件已加密，暂时无法解析'
  if (/unsupported|not supported|invalid (?:file )?type|unknown (?:file )?type|不支持/.test(text)) {
    return '暂不支持该文件格式'
  }
  if (/(?:file|document) too large|size limit|exceeds? .*size|文件过大/.test(text)) return '文件过大，超出解析上限'
  if (/context length|too many tokens|token limit|内容过长/.test(text)) return '文档内容过长，超出模型处理上限'
  if (/no (?:valid )?(?:text|content)|empty (?:document|content|text)|extract(?:ed)? no text|未提取到/.test(text)) {
    return '未从文档中提取到有效文本'
  }
  if (/ocr|文字识别/.test(text)) return '文字识别失败，请检查文件是否清晰'
  if (/permission denied|forbidden|unauthorized|access denied|没有权限/.test(text)) return '没有权限读取该文件'
  if (/no such file|not found|源文件不存在/.test(text)) return '源文件不存在或已被删除'
  if (/connection refused|connection reset|network|dial tcp|econn|socket|bad gateway|service unavailable|gateway timeout/.test(text)) {
    return '解析服务连接异常，请稍后重试'
  }
  return ''
}

function firstSentence(raw: string): string {
  const compact = raw.replace(/\s+/g, ' ').trim()
  if (!compact) return ''
  // Prefer a real sentence boundary; fall back to the first line when the
  // backend sent a multi-line stack trace.
  const match = compact.match(/^(.{1,MAX_SUMMARY_CHARS}?)[。！？!?](?:\s|$)/)
  const candidate = match ? match[1] : compact.split(/\n/)[0]
  const trimmed = candidate.trim()
  if (trimmed.length <= MAX_SUMMARY_CHARS) return trimmed
  return `${trimmed.slice(0, MAX_SUMMARY_CHARS - 1).trimEnd()}…`
}

/**
 * Turn a raw backend error into "one sentence + details". Missing error text
 * gets an explicit, actionable fallback instead of an empty tooltip.
 */
export function summarizeParseError(raw?: string | null): ParseErrorSummary {
  const normalized = normalizeParseError(raw)
  if (!normalized) {
    return { summary: '未返回具体原因，可重试或联系支持', detail: '' }
  }
  const friendly = friendlyParseError(normalized)
  const summary = friendly || firstSentence(normalized) || '未返回具体原因，可重试或联系支持'
  const detail = normalized === summary ? '' : normalized
  return { summary, detail }
}

function stallVerdict(item: ParseStatusItem): 'queued' | 'stalled' | '' {
  const minutes = item.stalled_minutes ?? 0
  if (minutes <= 0) return ''
  if (item.stall_state === 'queued') return 'queued'
  if (item.stall_state === 'stalled') return 'stalled'
  // The server only sends stall_state when its queue probe succeeds. A row
  // that has been quiet past the threshold is still worth flagging as
  // "possibly stuck", which is what the product copy promises.
  if (IN_FLIGHT.has(item.parse_status ?? '')) return 'stalled'
  return ''
}

/**
 * Resolve one document row to everything the UI needs to render its status.
 */
export function resolveParseStatus(item?: ParseStatusItem | null): ParseStatusView {
  const row = item ?? {}
  const status = String(row.parse_status ?? '')
  const minutes = Math.max(0, Math.floor(row.stalled_minutes ?? 0))
  const inFlight = IN_FLIGHT.has(status)

  if (status === 'failed') {
    const { summary } = summarizeParseError(row.error_message)
    return {
      kind: 'failed',
      tone: 'danger',
      label: '解析失败',
      hint: `解析失败：${summary}`,
      icon: 'close-circle',
      spin: false,
      pulse: false,
      inFlight: false,
      canRetry: true,
      quiet: false,
      stalledMinutes: 0,
    }
  }

  if (status === 'cancelled') {
    return {
      kind: 'cancelled',
      tone: 'neutral',
      label: '已取消',
      hint: '解析已取消，可重新解析',
      icon: 'stop-circle',
      spin: false,
      pulse: false,
      inFlight: false,
      canRetry: true,
      quiet: false,
      stalledMinutes: 0,
    }
  }

  if (status === 'draft') {
    return {
      kind: 'draft',
      tone: 'warning',
      label: '草稿',
      hint: '草稿尚未提交解析',
      icon: 'edit',
      spin: false,
      pulse: false,
      inFlight: false,
      canRetry: false,
      quiet: false,
      stalledMinutes: 0,
    }
  }

  if (inFlight) {
    const verdict = stallVerdict(row)
    if (verdict === 'stalled') {
      return {
        kind: 'stalled',
        tone: 'warning',
        label: '疑似卡住',
        hint: `已 ${minutes} 分钟没有进展，建议重新解析`,
        icon: 'time',
        spin: false,
        pulse: true,
        inFlight: true,
        canRetry: true,
        quiet: false,
        stalledMinutes: minutes,
      }
    }
    if (verdict === 'queued' || status === 'pending') {
      return {
        kind: 'queued',
        tone: 'neutral',
        label: '排队中',
        hint: minutes > 0
          ? `已排队等待解析 ${minutes} 分钟，页面会自动刷新`
          : '已进入解析队列，页面会自动刷新',
        icon: 'time',
        spin: false,
        pulse: false,
        inFlight: true,
        canRetry: false,
        quiet: false,
        stalledMinutes: minutes,
      }
    }
    if (status === 'finalizing') {
      const summaryPending =
        row.summary_status === 'pending' || row.summary_status === 'processing'
      return {
        kind: summaryPending ? 'summary' : 'finalizing',
        tone: 'active',
        label: summaryPending ? '生成摘要中' : '收尾处理中',
        hint: summaryPending
          ? '文档已解析，正在生成摘要，页面会自动刷新'
          : '文档已解析，正在完成后续处理，页面会自动刷新',
        icon: 'loading',
        spin: true,
        pulse: false,
        inFlight: true,
        canRetry: false,
        quiet: false,
        stalledMinutes: 0,
      }
    }
    return {
      kind: 'processing',
      tone: 'active',
      label: '解析中',
      hint: '正在解析，页面会自动刷新',
      icon: 'loading',
      spin: true,
      pulse: false,
      inFlight: true,
      canRetry: false,
      quiet: false,
      stalledMinutes: 0,
    }
  }

  const summaryPending = row.summary_status === 'pending' || row.summary_status === 'processing'
  if (status === 'completed' && summaryPending) {
    return {
      kind: 'summary',
      tone: 'active',
      label: '生成摘要中',
      hint: '文档已解析，正在生成摘要，页面会自动刷新',
      icon: 'loading',
      spin: true,
      pulse: false,
      inFlight: false,
      canRetry: false,
      quiet: false,
      stalledMinutes: 0,
    }
  }

  if (status === 'completed') {
    return {
      kind: 'completed',
      tone: 'success',
      label: '已完成',
      hint: '解析已完成',
      icon: 'check-circle',
      spin: false,
      pulse: false,
      inFlight: false,
      canRetry: false,
      quiet: true,
      stalledMinutes: 0,
    }
  }

  return {
    kind: 'unknown',
    tone: 'neutral',
    label: '未知状态',
    hint: '暂时无法识别解析状态',
    icon: 'help-circle',
    spin: false,
    pulse: false,
    inFlight: false,
    canRetry: false,
    quiet: true,
    stalledMinutes: 0,
  }
}
