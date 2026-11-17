<template>
  <div class="ck-home">
    <header class="ck-home__header">
      <div class="ck-home__heading">
        <p class="ck-home__eyebrow">SENIOR AI · C-KNOWLEDGE</p>
        <h1 class="ck-home__title">知识库</h1>
        <p class="ck-home__subtitle">香港建筑行业共享知识空间，由 Senior AI 团队维护</p>
      </div>
      <div class="ck-home__actions">
        <t-button variant="outline" :loading="loading" @click="load">
          <template #icon><t-icon name="refresh" /></template>
          刷新
        </t-button>
        <t-button theme="primary" @click="goManage">知识库管理</t-button>
      </div>
    </header>

    <section class="ck-home__stats" aria-label="知识库概览">
      <div class="ck-stat">
        <span class="ck-stat__label">知识库</span>
        <strong class="ck-stat__value">{{ loading ? '—' : bases.length }}</strong>
        <span class="ck-stat__hint">当前空间可访问</span>
      </div>
      <div class="ck-stat">
        <span class="ck-stat__label">文档</span>
        <strong class="ck-stat__value">{{ loading ? '—' : documentCount }}</strong>
        <span class="ck-stat__hint">已入库资料总数</span>
      </div>
      <div class="ck-stat">
        <span class="ck-stat__label">最近更新</span>
        <strong class="ck-stat__value ck-stat__value--sm">{{ loading ? '—' : lastUpdatedLabel }}</strong>
        <span class="ck-stat__hint">以知识库资料为准</span>
      </div>
    </section>

    <section class="ck-home__section">
      <div class="ck-home__section-head">
        <h2 class="ck-home__section-title">全部知识库</h2>
        <span class="ck-home__count">{{ loading ? '' : `共 ${bases.length} 个` }}</span>
      </div>

      <div v-if="loading" class="ck-grid">
        <div v-for="n in 3" :key="n" class="ck-card ck-card--skeleton">
          <t-skeleton :row-col="[{ width: '40%' }, { width: '80%' }, { width: '60%' }]" animation="gradient" />
        </div>
      </div>

      <div v-else-if="error" class="ck-empty">
        <p class="ck-empty__title">暂时无法载入知识库</p>
        <p class="ck-empty__text">{{ error }}</p>
        <t-button variant="outline" @click="load">重试</t-button>
      </div>

      <div v-else-if="!bases.length" class="ck-empty">
        <p class="ck-empty__title">当前空间还没有知识库</p>
        <p class="ck-empty__text">进入管理台新建知识库后，这里会自动出现。</p>
        <t-button theme="primary" @click="goManage">前往管理台</t-button>
      </div>

      <div v-else class="ck-grid">
        <article
          v-for="kb in bases"
          :key="kb.id"
          class="ck-card"
          tabindex="0"
          role="link"
          @click="openBase(kb)"
          @keydown.enter.prevent="openBase(kb)"
        >
          <div class="ck-card__top">
            <span class="ck-card__icon" :class="`ck-card__icon--${kb.type || 'document'}`">
              <t-icon :name="kb.type === 'faq' ? 'chat-bubble' : 'folder'" />
            </span>
            <span v-if="kb.type === 'faq'" class="ck-card__badge">问答</span>
          </div>
          <h3 class="ck-card__name">{{ kb.name }}</h3>
          <p class="ck-card__desc">{{ kb.description || '暂无描述' }}</p>
          <div class="ck-card__meta">
            <span>{{ countLabel(kb) }}</span>
            <span v-if="kb.updated_at">更新于 {{ formatDate(kb.updated_at) }}</span>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listKnowledgeBases } from '@/api/knowledge-base'

interface CkKnowledgeBase {
  id: string
  name: string
  description?: string
  type?: 'document' | 'faq'
  knowledge_count?: number
  chunk_count?: number
  updated_at?: string
}

const router = useRouter()
const bases = ref<CkKnowledgeBase[]>([])
const loading = ref(true)
const error = ref('')

const documentCount = computed(() =>
  bases.value.reduce((total, kb) => total + Number(kb.knowledge_count || 0), 0),
)

const lastUpdatedLabel = computed(() => {
  const stamps = bases.value
    .map((kb) => (kb.updated_at ? new Date(kb.updated_at).getTime() : 0))
    .filter((time) => Number.isFinite(time) && time > 0)
  if (!stamps.length) return '—'
  return formatDate(new Date(Math.max(...stamps)).toISOString())
})

const countLabel = (kb: CkKnowledgeBase): string => {
  if (kb.type === 'faq') return `${Number(kb.chunk_count || 0)} 组问答`
  return `${Number(kb.knowledge_count || 0)} 个文档`
}

const formatDate = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return `${date.getFullYear()}/${String(date.getMonth() + 1).padStart(2, '0')}/${String(date.getDate()).padStart(2, '0')}`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const response = (await listKnowledgeBases()) as unknown as {
      success?: boolean
      data?: CkKnowledgeBase[]
      message?: string
    }
    if (response?.success === false) {
      error.value = response.message || '管理台返回了错误状态'
      bases.value = []
      return
    }
    bases.value = Array.isArray(response?.data) ? response.data : []
  } catch (err: any) {
    error.value = err?.message || '网络异常，请稍后重试'
    bases.value = []
  } finally {
    loading.value = false
  }
}

function openBase(kb: CkKnowledgeBase) {
  router.push(`/platform/knowledge-bases/${kb.id}`)
}

function goManage() {
  router.push('/platform/knowledge-bases')
}

onMounted(load)
</script>

<style scoped lang="less">
.ck-home {
  max-width: 1180px;
  margin: 0 auto;
  padding: 32px 40px 56px;
  width: 100%;
  box-sizing: border-box;
}

.ck-home__header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;
}

.ck-home__eyebrow {
  margin: 0 0 10px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.16em;
  color: var(--td-brand-color);
  opacity: 0.85;
}

.ck-home__title {
  margin: 0;
  font-size: 30px;
  line-height: 1.2;
  font-weight: 650;
  color: var(--td-text-color-primary);
}

.ck-home__subtitle {
  margin: 10px 0 0;
  font-size: 14px;
  color: var(--td-text-color-secondary);
}

.ck-home__actions {
  display: flex;
  gap: 12px;
}

.ck-home__stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  margin: 28px 0 36px;
}

.ck-stat {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 20px 22px;
  border-radius: 14px;
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}

.ck-stat__label {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.ck-stat__value {
  font-size: 28px;
  line-height: 1.15;
  font-weight: 650;
  color: var(--td-text-color-primary);
  font-variant-numeric: tabular-nums;
}

.ck-stat__value--sm {
  font-size: 20px;
  padding-top: 4px;
}

.ck-stat__hint {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.ck-home__section {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.ck-home__section-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
}

.ck-home__section-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.ck-home__count {
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.ck-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.ck-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 168px;
  padding: 20px;
  border-radius: 14px;
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  cursor: pointer;
  transition: border-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
  box-sizing: border-box;
}

.ck-card:hover,
.ck-card:focus-visible {
  border-color: var(--td-brand-color-light-active);
  box-shadow: 0 10px 26px rgba(15, 23, 42, 0.08);
  transform: translateY(-1px);
  outline: none;
}

.ck-card--skeleton {
  cursor: default;
  gap: 14px;
}

.ck-card--skeleton:hover {
  border-color: var(--td-component-stroke);
  box-shadow: none;
  transform: none;
}

.ck-card__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.ck-card__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  font-size: 20px;
}

.ck-card__icon--faq {
  background: rgba(245, 158, 11, 0.12);
  color: #b45309;
}

.ck-card__badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 999px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
}

.ck-card__name {
  margin: 0;
  font-size: 16px;
  font-weight: 620;
  line-height: 1.4;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.ck-card__desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  min-height: 39px;
}

.ck-card__meta {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.ck-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 48px 24px;
  border-radius: 14px;
  border: 1px dashed var(--td-component-stroke);
  background: var(--td-bg-color-container);
  text-align: center;
}

.ck-empty__title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.ck-empty__text {
  margin: 0 0 6px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

@media (max-width: 900px) {
  .ck-home {
    padding: 24px 20px 40px;
  }

  .ck-home__stats {
    grid-template-columns: 1fr;
  }
}
</style>
