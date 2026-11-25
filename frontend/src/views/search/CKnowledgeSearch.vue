<template>
  <div class="ck-search">
    <header class="ck-search__header">
      <p class="ck-search__eyebrow">SENIOR AI · C-KNOWLEDGE</p>
      <h1 class="ck-search__title">检索</h1>
      <p class="ck-search__subtitle">在香港建筑行业共享知识库中查找原文与出处</p>
    </header>

    <form class="ck-search__bar" @submit.prevent="runSearch">
      <t-input
        v-model="query"
        size="large"
        clearable
        placeholder="输入一句话，例如：建筑高度限制是多少？"
        :disabled="loading"
        @enter="runSearch"
      />
      <t-button theme="primary" size="large" type="submit" :loading="loading" :disabled="!query.trim()">
        检索
      </t-button>
    </form>

    <div class="ck-search__filters" role="group" aria-label="知识库筛选">
      <button
        type="button"
        class="ck-chip"
        :class="{ 'ck-chip--active': selected.length === 0 }"
        @click="selected = []"
      >
        全部
      </button>
      <button
        v-for="kb in bases"
        :key="kb.id"
        type="button"
        class="ck-chip"
        :class="{ 'ck-chip--active': selected.includes(kb.id) }"
        @click="toggleBase(kb.id)"
      >
        {{ kb.name }}
      </button>
    </div>

    <section v-if="loading" class="ck-search__state" aria-live="polite">
      <t-loading size="small" text="正在检索…" />
    </section>

    <section v-else-if="error" class="ck-search__state">
      <p class="ck-search__state-title">检索暂时不可用</p>
      <p class="ck-search__state-text">{{ error }}</p>
      <t-button variant="outline" @click="runSearch">重试</t-button>
    </section>

    <section v-else-if="hasSearched && results.length === 0" class="ck-search__state">
      <p class="ck-search__state-title">没有找到相关内容</p>
      <p class="ck-search__state-text">
        可以换个说法再试，或确认相关资料已经上传到知识库。
      </p>
    </section>

    <section v-else-if="results.length" class="ck-search__results">
      <div class="ck-search__results-head">
        <span>找到 {{ results.length }} 条结果</span>
        <span class="ck-search__results-meta">检索了 {{ searchedCount }} 个知识库</span>
      </div>
      <article v-for="(hit, index) in results" :key="`${hit.knowledge_base_id}-${hit.knowledge_id}-${hit.chunk_index}-${index}`" class="ck-hit">
        <div class="ck-hit__head">
          <a class="ck-hit__title" :href="kbHref(hit)" @click.prevent="openKb(hit)">
            {{ hit.knowledge_title || '未命名文档' }}
          </a>
          <span class="ck-hit__score">{{ formatScore(hit.score) }}</span>
        </div>
        <p class="ck-hit__content">{{ cleanSnippet(hit.content) }}</p>
        <div class="ck-hit__meta">
          <span class="ck-hit__kb">{{ hit.knowledge_base_name }}</span>
          <span class="ck-hit__tag ck-hit__tag--plain">第 {{ hit.chunk_index + 1 }} 段</span>
          <span v-if="hit.knowledge_base_scope === 'project'" class="ck-hit__tag">项目</span>
          <span v-else-if="hit.knowledge_base_scope === 'personal'" class="ck-hit__tag">私密</span>
        </div>
      </article>
    </section>

    <section v-else class="ck-search__hint">
      <p class="ck-search__state-title">试试这些问题</p>
      <div class="ck-search__samples">
        <button v-for="sample in samples" :key="sample" type="button" class="ck-sample" @click="useSample(sample)">
          {{ sample }}
        </button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listKnowledgeBases, searchKnowledgeAcrossBases } from '@/api/knowledge-base'
import type { KnowledgeSearchHit } from '@/api/knowledge-base'

interface CkKnowledgeBase {
  id: string
  name: string
}

const router = useRouter()
const bases = ref<CkKnowledgeBase[]>([])
const selected = ref<string[]>([])
const query = ref('')
const results = ref<KnowledgeSearchHit[]>([])
const searchedCount = ref(0)
const loading = ref(false)
const hasSearched = ref(false)
const error = ref('')

const samples = [
  '建筑高度限制是多少？',
  '消防安全有什么要求？',
  'OTTV 的计算方法',
]

const loadBases = async () => {
  try {
    const response: any = await listKnowledgeBases()
    bases.value = (response?.data ?? response ?? []) as CkKnowledgeBase[]
  } catch {
    bases.value = []
  }
}

const toggleBase = (id: string) => {
  const index = selected.value.indexOf(id)
  if (index >= 0) {
    selected.value = selected.value.filter(item => item !== id)
  } else {
    selected.value = [...selected.value, id]
  }
}

const runSearch = async () => {
  const text = query.value.trim()
  if (!text || loading.value) return
  loading.value = true
  error.value = ''
  try {
    const response: any = await searchKnowledgeAcrossBases({
      query_text: text,
      knowledge_base_ids: selected.value.length ? selected.value : undefined,
      top_k: 20,
    })
    const payload = response?.data ?? response ?? {}
    results.value = (payload.results ?? []) as KnowledgeSearchHit[]
    searchedCount.value = payload.searched_kb_count ?? 0
  } catch (err: any) {
    results.value = []
    error.value = err?.message || '请稍后再试'
  } finally {
    loading.value = false
    hasSearched.value = true
  }
}

const useSample = (sample: string) => {
  query.value = sample
  void runSearch()
}

// 抽取出的正文常带 PDF 的 markdown 痕迹（**加粗**、### 标题），列表里按纯文本读更顺。
const cleanSnippet = (content: string) =>
  (content || '')
    .replace(/\*\*/g, '')
    .replace(/^#{1,6}\s*/gm, '')
    .replace(/\n{3,}/g, '\n\n')
    .trim()

const formatScore = (score: number) => {
  if (typeof score !== 'number' || Number.isNaN(score)) return ''
  return score.toFixed(2)
}

const kbHref = (hit: KnowledgeSearchHit) => `/platform/knowledge-bases/${hit.knowledge_base_id}`

const openKb = (hit: KnowledgeSearchHit) => {
  router.push({ name: 'knowledgeBaseDetail', params: { kbId: hit.knowledge_base_id } })
}

onMounted(loadBases)
</script>

<style scoped lang="less">
.ck-search {
  max-width: 980px;
  margin: 0 auto;
  padding: 32px 40px 56px;
  width: 100%;
  box-sizing: border-box;
}

.ck-search__eyebrow {
  margin: 0 0 10px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.16em;
  color: var(--td-brand-color);
  opacity: 0.85;
}

.ck-search__title {
  margin: 0;
  font-size: 30px;
  line-height: 1.2;
  font-weight: 650;
  color: var(--td-text-color-primary);
}

.ck-search__subtitle {
  margin: 10px 0 0;
  font-size: 14px;
  color: var(--td-text-color-secondary);
}

.ck-search__bar {
  display: flex;
  gap: 12px;
  margin-top: 28px;
  align-items: stretch;
}

.ck-search__bar :deep(.t-input__wrap) {
  flex: 1 1 auto;
}

.ck-search__filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 16px;
}

.ck-chip {
  border: 1px solid var(--td-component-border);
  background: var(--td-bg-color-container);
  color: var(--td-text-color-secondary);
  border-radius: 999px;
  padding: 5px 14px;
  font-size: 13px;
  line-height: 1.5;
  cursor: pointer;
  transition: border-color 0.15s ease, color 0.15s ease, background-color 0.15s ease;
}

.ck-chip:hover {
  border-color: var(--td-brand-color);
  color: var(--td-brand-color);
}

.ck-chip--active {
  border-color: var(--td-brand-color);
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  font-weight: 600;
}

.ck-search__state,
.ck-search__hint {
  margin-top: 40px;
  padding: 32px;
  border: 1px dashed var(--td-component-border);
  border-radius: 14px;
  text-align: center;
}

.ck-search__state-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.ck-search__state-text {
  margin: 8px 0 16px;
  font-size: 14px;
  color: var(--td-text-color-secondary);
}

.ck-search__samples {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: center;
  margin-top: 16px;
}

.ck-sample {
  border: 1px solid var(--td-component-border);
  background: var(--td-bg-color-container);
  color: var(--td-text-color-primary);
  border-radius: 8px;
  padding: 8px 14px;
  font-size: 13px;
  cursor: pointer;
}

.ck-sample:hover {
  border-color: var(--td-brand-color);
  color: var(--td-brand-color);
}

.ck-search__results {
  margin-top: 28px;
}

.ck-search__results-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 16px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  padding-bottom: 10px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.ck-hit {
  padding: 18px 0;
  border-bottom: 1px solid var(--td-component-stroke);
}

.ck-hit__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
}

.ck-hit__title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-brand-color);
  text-decoration: none;
  overflow-wrap: anywhere;
}

.ck-hit__title:hover {
  text-decoration: underline;
}

.ck-hit__score {
  flex: 0 0 auto;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  font-variant-numeric: tabular-nums;
}

.ck-hit__content {
  margin: 8px 0 0;
  font-size: 14px;
  line-height: 1.75;
  color: var(--td-text-color-primary);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.ck-hit__meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.ck-hit__tag {
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer);
}

.ck-hit__tag--plain {
  background: transparent;
  padding: 0;
  color: var(--td-text-color-placeholder);
}

@media (max-width: 720px) {
  .ck-search {
    padding: 24px 20px 40px;
  }

  .ck-search__bar {
    flex-direction: column;
  }
}
</style>