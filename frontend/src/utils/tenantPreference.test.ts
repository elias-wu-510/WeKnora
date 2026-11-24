import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  PREFERRED_TENANT_KEY,
  SELECTED_TENANT_KEY,
  applyTenantPreference,
  resolvePreferredTenant,
  type TenantPreferenceStorage,
} from './tenantPreference.ts'

function memoryStorage(initial: Record<string, string> = {}): TenantPreferenceStorage {
  const data = new Map(Object.entries(initial))
  return {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      data.set(key, value)
    },
  }
}

const memberships = [
  { tenant_id: 1, tenant_name: 'Home' },
  { tenant_id: 10000, tenant_name: 'Shared' },
]

test('本机显式选择优先于服务端 preferred 与配置默认值', () => {
  const storage = memoryStorage({
    [PREFERRED_TENANT_KEY]: '10000',
    [SELECTED_TENANT_KEY]: '1',
  })
  const selection = resolvePreferredTenant({
    console: true,
    storage,
    preferredId: 1,
    configDefaultId: 1,
    memberships,
    homeTenantId: 1,
  })
  assert.deepEqual(selection, { id: 10000, name: 'Shared', source: 'user' })
})

test('无本机选择时采用服务端 preferred_tenant_id', () => {
  const selection = resolvePreferredTenant({
    console: true,
    storage: memoryStorage(),
    preferredId: '10000',
    activeTenantId: '1',
    memberships,
    homeTenantId: 1,
  })
  assert.deepEqual(selection, { id: 10000, name: 'Shared', source: 'server' })
})

test('服务端 preferred 已不是成员时，成员校验失败并回落到下一个可用层', () => {
  const selection = resolvePreferredTenant({
    console: true,
    storage: memoryStorage(),
    preferredId: 7777,
    configDefaultId: 10000,
    memberships,
    homeTenantId: 1,
  })
  assert.deepEqual(selection, { id: 10000, name: 'Shared', source: 'config' })
})

test('配置默认共享空间仅在严格成员时采用', () => {
  const notMember = resolvePreferredTenant({
    console: true,
    storage: memoryStorage(),
    configDefaultId: 10000,
    memberships: [{ tenant_id: 1, tenant_name: 'Home' }],
    homeTenantId: 1,
  })
  assert.deepEqual(notMember, { id: null, name: null, source: 'home' })

  const member = resolvePreferredTenant({
    console: true,
    storage: memoryStorage(),
    configDefaultId: 10000,
    memberships,
    homeTenantId: 1,
  })
  assert.deepEqual(member, { id: 10000, name: 'Shared', source: 'config' })
})

test('无配置且无偏好时降级为不预设空间，不盲写 10000', () => {
  const selection = resolvePreferredTenant({
    console: true,
    storage: memoryStorage(),
    configDefaultId: null,
    memberships,
    homeTenantId: 1,
  })
  assert.equal(selection.id, null)
  assert.equal(selection.source, 'home')
})

test('旧版 weknora_selected_tenant_id 在服务端 preferred 缺失时兜底', () => {
  const selection = resolvePreferredTenant({
    console: true,
    storage: memoryStorage({ [SELECTED_TENANT_KEY]: '10000' }),
    memberships,
    homeTenantId: 1,
  })
  assert.deepEqual(selection, { id: 10000, name: 'Shared', source: 'stored' })
})

test('成员列表缺失时不误判服务端 preferred 为非法', () => {
  const selection = resolvePreferredTenant({
    console: true,
    storage: memoryStorage(),
    preferredId: 10000,
    homeTenantId: 1,
  })
  assert.deepEqual(selection, { id: 10000, name: null, source: 'server' })
})

test('全部缺失且活跃空间不同于 home 时保留活跃空间', () => {
  const selection = resolvePreferredTenant({
    console: true,
    storage: memoryStorage(),
    activeTenantId: 10000,
    activeTenantName: 'Shared',
    homeTenantId: 1,
    memberships,
  })
  assert.deepEqual(selection, { id: 10000, name: 'Shared', source: 'active' })
})

test('引擎站点沿用旧行为，不消费共享空间默认值', () => {
  const selection = resolvePreferredTenant({
    console: false,
    storage: memoryStorage({ [PREFERRED_TENANT_KEY]: '10000' }),
    configDefaultId: 10000,
    activeTenantId: 1,
    homeTenantId: 1,
    memberships,
  })
  assert.deepEqual(selection, { id: null, name: null, source: 'home' })
})

test('applyTenantPreference 命中本机选择时不额外请求 /auth/config', async () => {
  const storage = memoryStorage({ [PREFERRED_TENANT_KEY]: '10000' })
  const written: Array<{ id: number | null; name: string | null }> = []
  const selection = await applyTenantPreference({
    console: true,
    storage,
    preferredId: 1,
    homeTenantId: 1,
    memberships,
    setSelectedTenant: (id, name) => written.push({ id, name }),
    loadConfigDefault: () => {
      throw new Error('不应加载配置默认值')
    },
  })
  assert.equal(selection.id, 10000)
  assert.deepEqual(written, [{ id: 10000, name: 'Shared' }])
})

test('applyTenantPreference 在前三层未命中时注入配置默认值并写入 store', async () => {
  const written: Array<{ id: number | null; name: string | null }> = []
  const selection = await applyTenantPreference({
    console: true,
    storage: memoryStorage(),
    homeTenantId: 1,
    memberships,
    setSelectedTenant: (id, name) => written.push({ id, name }),
    loadConfigDefault: async () => 10000,
  })
  assert.deepEqual(selection, { id: 10000, name: 'Shared', source: 'config' })
  assert.deepEqual(written, [{ id: 10000, name: 'Shared' }])
})
