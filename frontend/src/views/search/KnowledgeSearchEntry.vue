<template>
  <CKnowledgeSearch v-if="isCk" />
  <div v-else class="ck-search-entry" />
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import CKnowledgeSearch from './CKnowledgeSearch.vue'
import { isCKnowledgeConsole } from '@/config/deployment'

const route = useRoute()
const router = useRouter()
const isCk = computed(() => isCKnowledgeConsole())

onMounted(() => {
  if (isCk.value) return
  // 官方控制台仍保留「全局命令面板」语义：旧路径重定向过去。
  const q = typeof route.query.q === 'string' ? route.query.q : ''
  router.replace({
    path: '/platform/knowledge-bases',
    query: q ? { cmdk: q } : { cmdk: '' },
  })
})
</script>

<style scoped>
.ck-search-entry {
  min-height: 40vh;
}
</style>