import assert from 'node:assert/strict'
import test from 'node:test'

import {
  friendlyParseError,
  normalizeParseError,
  resolveParseStatus,
  summarizeParseError,
} from './parseStatus.ts'

test('in-flight statuses map to distinct, ordered labels', () => {
  assert.equal(resolveParseStatus({ parse_status: 'pending' }).label, '排队中')
  assert.equal(resolveParseStatus({ parse_status: 'processing' }).label, '解析中')
  assert.equal(resolveParseStatus({ parse_status: 'processing' }).spin, true)
  assert.equal(resolveParseStatus({ parse_status: 'finalizing' }).label, '收尾处理中')
  assert.equal(
    resolveParseStatus({ parse_status: 'finalizing', summary_status: 'processing' }).label,
    '生成摘要中',
  )
  assert.equal(resolveParseStatus({ parse_status: 'completed', summary_status: 'pending' }).label, '生成摘要中')
})

test('terminal statuses stay quiet and only failures / stalls are retryable', () => {
  const completed = resolveParseStatus({ parse_status: 'completed', summary_status: 'completed' })
  assert.equal(completed.label, '已完成')
  assert.equal(completed.quiet, true)
  assert.equal(completed.canRetry, false)

  const failed = resolveParseStatus({ parse_status: 'failed', error_message: 'boom' })
  assert.equal(failed.label, '解析失败')
  assert.equal(failed.tone, 'danger')
  assert.equal(failed.canRetry, true)

  assert.equal(resolveParseStatus({ parse_status: 'cancelled' }).canRetry, true)
  assert.equal(resolveParseStatus({ parse_status: 'draft' }).label, '草稿')
})

test('stalled rows win over the generic in-flight label', () => {
  const stalled = resolveParseStatus({
    parse_status: 'processing',
    stalled_minutes: 47,
    stall_state: 'stalled',
  })
  assert.equal(stalled.kind, 'stalled')
  assert.equal(stalled.label, '疑似卡住')
  assert.match(stalled.hint, /47 分钟/)
  assert.equal(stalled.canRetry, true)
  assert.equal(stalled.pulse, true)
})

test('a quiet row past the threshold is flagged even without a server verdict', () => {
  const stalled = resolveParseStatus({ parse_status: 'processing', stalled_minutes: 31 })
  assert.equal(stalled.kind, 'stalled', 'the product promises "possibly stuck" past the threshold')

  const queued = resolveParseStatus({
    parse_status: 'pending',
    stalled_minutes: 25,
    stall_state: 'queued',
  })
  assert.equal(queued.kind, 'queued')
  assert.equal(queued.label, '排队中')
})

test('error text is normalised into one sentence plus a detail block', () => {
  const summarized = summarizeParseError('Error:   parse failed\n\n\n  context deadline exceeded  ')
  assert.equal(summarized.summary, '解析超时，请稍后重试')
  assert.equal(summarized.detail, 'parse failed\n\ncontext deadline exceeded')
})

test('unknown errors keep their first sentence and long text is truncated', () => {
  const long = `${'x'.repeat(140)} happened`
  const summarized = summarizeParseError(long)
  assert.equal(summarized.summary.length, 80)
  assert.ok(summarized.summary.endsWith('…'))
  assert.equal(summarized.detail, long)
})

test('missing error text gets an actionable fallback', () => {
  for (const raw of [undefined, null, '', '   \n  ']) {
    assert.deepEqual(summarizeParseError(raw), {
      summary: '未返回具体原因，可重试或联系支持',
      detail: '',
    })
  }
})

test('friendly mappings cover the common backend failures', () => {
  assert.match(friendlyParseError('document is password protected'), /加密/)
  assert.match(friendlyParseError('unsupported file type: xyz'), /格式/)
  assert.match(friendlyParseError('read tcp 10.0.0.1: connection reset by peer'), /连接异常/)
  assert.match(friendlyParseError('no text extracted from pdf'), /有效文本/)
  assert.equal(friendlyParseError('some unexpected backend failure'), '')
  assert.equal(normalizeParseError('  a \t b  '), 'a b')
})
