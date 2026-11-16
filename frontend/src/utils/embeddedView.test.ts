import assert from 'node:assert/strict'
import test from 'node:test'

import { detectEmbeddedView } from './embeddedView.ts'

// 管理台被 Senior AI 以 iframe 内嵌时（?embed=1）应判定为嵌入态，否则保留侧边栏。

test('iframe 内（self !== top）判定为嵌入态', () => {
  const win = {}
  assert.equal(detectEmbeddedView({ self: win, top: {} }), true)
})

test('顶层窗口（self === top）默认不是嵌入态', () => {
  const win = {}
  assert.equal(detectEmbeddedView({ self: win, top: win, search: '' }), false)
})

test('顶层窗口但 URL 带 embed=1 时判定为嵌入态', () => {
  const win = {}
  assert.equal(detectEmbeddedView({ self: win, top: win, search: '?embed=1' }), true)
})

test('embed 参数只认 1，其他值不误判', () => {
  const win = {}
  assert.equal(detectEmbeddedView({ self: win, top: win, search: '?embed=0' }), false)
  assert.equal(detectEmbeddedView({ self: win, top: win, search: '?embed=true' }), false)
})

test('路由跳转丢掉 query 后，sessionStorage 记住的嵌入态继续生效', () => {
  const win = {}
  assert.equal(detectEmbeddedView({ self: win, top: win, search: '', stored: '1' }), true)
  assert.equal(detectEmbeddedView({ self: win, top: win, search: '', stored: null }), false)
})

test('无法访问 window（如 SSR/测试环境）时不抛错', () => {
  assert.equal(detectEmbeddedView({ self: undefined, top: undefined, search: '' }), false)
})