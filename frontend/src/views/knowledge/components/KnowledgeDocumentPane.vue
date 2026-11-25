<script setup lang="ts">
import DocumentCardView from './DocumentCardView.vue';
import DocumentListView from './DocumentListView.vue';
import DocumentBatchBar from './DocumentBatchBar.vue';
import EmptyKnowledge from '@/components/empty-knowledge.vue';

defineProps<{
  kbId: string;
  items: any[];
  viewMode: 'grid' | 'list';
  docListLoading: boolean;
  batchMode: boolean;
  selectedIds: Set<string>;
  folderOptions: Array<{ path: string; name: string; depth: number }>;
  showDocumentFolderPath: boolean;
  canEdit: boolean;
  canDownloadKnowledge: boolean;
  canDeleteKnowledge: boolean;
  canMutateKnowledge: boolean;
  traceAvailableById: Record<string, boolean>;
  moveMenuMode: 'normal' | 'targets' | 'confirm';
  moveTargetKbs: any[];
  moveTargetsLoading: boolean;
  moveSelectedTargetName: string;
  moveMode: 'reuse_vectors' | 'reparse';
  moveSubmitting: boolean;
  hasFolders: boolean;
  selectedFolderPath: string;
  isFiltering: boolean;
  docMarqueeVisible: boolean;
  docMarqueeMode: 'add' | 'subtract';
  docMarqueeBoxStyle: Record<string, string>;
  batchDeleting: boolean;
  batchReparsing: boolean;
  batchTagging: boolean;
  batchDownloading: boolean;
}>();

const emit = defineEmits<{
  (e: 'open', item: any): void;
  (e: 'open-folder', path: string): void;
  (e: 'move-to-folder', item: any, path: string): void;
  (e: 'toggle-checkbox', id: string, checked: boolean, ctx?: { e?: Event }): void;
  (e: 'menu-visible-change', visible: boolean, item: any): void;
  (e: 'action', action: any, item: any): void;
  (e: 'tags-changed', payload?: { deletedTagId?: string }): void;
  (e: 'move-select-target', kb: any): void;
  (e: 'move-back'): void;
  (e: 'move-confirm'): void;
  (e: 'update:move-mode', mode: 'reuse_vectors' | 'reparse'): void;
  (e: 'probe-trace', item: any): void;
  (e: 'toggle-row', id: string, checked: boolean, shiftKey: boolean): void;
  (e: 'toggle-all', checked: boolean): void;
  (e: 'reset-move-state'): void;
  (e: 'scroll'): void;
  (e: 'marquee-mousedown', event: MouseEvent): void;
  (e: 'batch-cancel'): void;
  (e: 'batch-delete'): void;
  (e: 'batch-reparse'): void;
  (e: 'batch-tag'): void;
  (e: 'batch-download'): void;
  (e: 'batch-select-loaded'): void;
  (e: 'batch-move-to-folder', path: string): void;
  (e: 'scroll-ref', el: any): void;
}>();
</script>

<template>
  <div class="doc-scroll-container"
    :class="{
      'is-empty': !items.length && !docListLoading,
      'is-marquee-active': docMarqueeVisible,
    }"
    :ref="(el: any) => { if (el) emit('scroll-ref', el); }" @scroll="emit('scroll')" @mousedown="(event: MouseEvent) => emit('marquee-mousedown', event)">
    <div v-if="docMarqueeVisible" class="doc-marquee-box"
      :class="{ 'is-add': docMarqueeMode === 'add', 'is-subtract': docMarqueeMode === 'subtract' }"
      :style="docMarqueeBoxStyle" aria-hidden="true" />
    <!-- 文档骨架屏 -->
    <div v-if="docListLoading && !items.length && viewMode === 'list'" class="doc-list-skeleton">
      <div v-for="n in 8" :key="n" class="doc-list-skeleton-row">
        <t-skeleton animation="gradient" :row-col="[[{ width: '32px', height: '38px', type: 'rect' }, { width: '55%', height: '16px' }]]" />
      </div>
    </div>
    <div v-else-if="docListLoading && items.length === 0" class="doc-card-list doc-card-list-animated">
      <div v-for="n in 8" :key="'doc-skel-' + n" class="knowledge-card knowledge-card-skeleton">
        <div class="card-content">
          <div class="card-content-nav">
            <t-skeleton animation="gradient" :row-col="[{ width: '70%', height: '18px' }]" />
          </div>
          <t-skeleton animation="gradient"
            :row-col="[{ width: '100%', height: '14px' }, { width: '60%', height: '14px' }]" />
        </div>
        <div class="card-bottom">
          <t-skeleton animation="gradient"
            :row-col="[[{ width: '80px', height: '14px' }, { width: '40px', height: '18px', type: 'rect' }]]" />
        </div>
      </div>
    </div>
    <template v-else-if="items.length && viewMode === 'grid'">
      <DocumentCardView :kb-id="kbId"
        :items="items"
        :folder-options="folderOptions"
        :selected-ids="selectedIds"
        :batch-mode="batchMode"
        :can-edit="canEdit"
        :can-download="canDownloadKnowledge"
        :can-delete-knowledge="canDeleteKnowledge"
        :can-mutate-knowledge="canMutateKnowledge"
        :trace-available-by-id="traceAvailableById"
        :move-menu-mode="moveMenuMode"
        :move-target-kbs="moveTargetKbs"
        :move-targets-loading="moveTargetsLoading"
        :move-selected-target-name="moveSelectedTargetName"
        :move-mode="moveMode"
        :move-submitting="moveSubmitting"
        :show-folder-path="showDocumentFolderPath"
        @open="(item: any) => emit('open', item)"
        @open-folder="(path: string) => emit('open-folder', path)"
        @move-to-folder="(item: any, path: string) => emit('move-to-folder', item, path)"
        @toggle-checkbox="(id: string, checked: boolean, ctx?: { e?: Event }) => emit('toggle-checkbox', id, checked, ctx)"
        @menu-visible-change="(visible: boolean, item: any) => emit('menu-visible-change', visible, item)"
        @action="(action: any, item: any) => emit('action', action, item)"
        @tags-changed="(payload?: { deletedTagId?: string }) => emit('tags-changed', payload)"
        @move-select-target="(kb: any) => emit('move-select-target', kb)"
        @move-back="emit('move-back')"
        @move-confirm="emit('move-confirm')"
        @update:move-mode="(mode: any) => emit('update:move-mode', mode)"
      />
    </template>
    <template v-else-if="items.length && viewMode === 'list'">
      <DocumentListView :kb-id="kbId" :items="items" :folder-options="folderOptions"
        :selected-ids="selectedIds"
        :can-edit="canEdit" :can-download="canDownloadKnowledge"
        :can-delete-knowledge="canDeleteKnowledge" :can-mutate-knowledge="canMutateKnowledge"
        :trace-visible-ids="traceAvailableById"
        :move-menu-mode="moveMenuMode"
        :move-target-kbs="moveTargetKbs"
        :move-targets-loading="moveTargetsLoading"
        :move-selected-target-name="moveSelectedTargetName"
        :move-mode="moveMode"
        :move-submitting="moveSubmitting"
        :show-folder-path="showDocumentFolderPath"
        @open-folder="(path: string) => emit('open-folder', path)"
        @move-to-folder="(item: any, path: string) => emit('move-to-folder', item, path)"
        @open="(item: any) => emit('open', item)" @toggle-row="(id: string, checked: boolean, shiftKey: boolean) => emit('toggle-row', id, checked, shiftKey)"
        @toggle-all="(checked: boolean) => emit('toggle-all', checked)" @action="(action: any, item: any) => emit('action', action, item)"
        @probe-trace="(item: any) => emit('probe-trace', item)"
        @tags-changed="(payload?: { deletedTagId?: string }) => emit('tags-changed', payload)"
        @move-select-target="(kb: any) => emit('move-select-target', kb)"
        @move-back="emit('move-back')"
        @move-confirm="emit('move-confirm')"
        @update:move-mode="(mode: any) => emit('update:move-mode', mode)"
        @reset-move-state="emit('reset-move-state')" />
    </template>
    <template v-else-if="!docListLoading">
      <div class="doc-empty-state">
        <p v-if="hasFolders || selectedFolderPath || isFiltering" class="doc-empty-folder">
          {{ isFiltering
            ? $t('knowledgeBase.folderTree.emptySearch')
            : $t('knowledgeBase.folderTree.emptyFolder') }}
        </p>
        <EmptyKnowledge v-else />
      </div>
    </template>
  </div>
  <div class="doc-batch-bar-anchor" v-show="batchMode || selectedIds.size > 0">
    <DocumentBatchBar :count="selectedIds.size" :delete-loading="batchDeleting"
      :reparse-loading="batchReparsing" :tag-loading="batchTagging" :download-loading="batchDownloading"
      :can-download="canDownloadKnowledge" :can-mutate="canMutateKnowledge"
      :visible="batchMode || selectedIds.size > 0"
      :show-move-to-folder="canEdit" :folder-options="folderOptions"
      @cancel="emit('batch-cancel')" @delete="emit('batch-delete')" @reparse="emit('batch-reparse')"
      @batch-tag="emit('batch-tag')" @download="emit('batch-download')" @select-loaded="emit('batch-select-loaded')"
      @move-to-folder="(path: string) => emit('batch-move-to-folder', path)" />
  </div>
</template>

<style scoped lang="less">
.doc-scroll-container {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 4px;

  &.is-empty {
    display: flex;
    align-items: center;
    justify-content: center;
    overflow-y: hidden;
  }

  &.is-marquee-active {
    cursor: crosshair;
  }
}

.doc-marquee-box {
  position: absolute;
  z-index: 4;
  pointer-events: none;
  border: 1px solid var(--td-brand-color);
  background: color-mix(in srgb, var(--td-brand-color) 12%, transparent);
  border-radius: 2px;

  &.is-add {
    border-color: var(--td-brand-color);
    background: color-mix(in srgb, var(--td-brand-color) 14%, transparent);
  }

  &.is-subtract {
    border-color: var(--td-error-color-6);
    background: color-mix(in srgb, var(--td-error-color-6) 12%, transparent);
  }
}

/* Reserve space for the batch actions so the last document stays reachable. */
.doc-batch-bar-anchor {
  position: relative;
  flex-shrink: 0;
  z-index: 6;
  display: flex;
  justify-content: center;
  padding: 12px 0 0;
  pointer-events: none;

  &>* {
    pointer-events: auto;
  }
}

.doc-list-skeleton-row { padding: 16px 40px; }

@keyframes contentFadeIn {
  from {
    opacity: 0;
    transform: translateY(6px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.doc-card-list {
  box-sizing: border-box;
  display: grid;
  // 文档卡片信息量较大（标题 + 摘要 + 标签/类型），保持稍宽的最小列宽，避免一行塞太多导致内容拥挤。
  grid-template-columns: repeat(auto-fill, minmax(min(260px, 100%), 1fr));
  gap: 10px;
  align-content: flex-start;
  width: 100%;

  &.doc-card-list-animated {
    animation: contentFadeIn 0.32s ease-out;
  }
}

.knowledge-card-skeleton {
  cursor: default;

  .card-content {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding: 10px;
  }

  .card-content-nav {
    margin-bottom: 8px;
  }

  .card-bottom {
    flex-shrink: 0;
    margin-top: auto;
    width: 100%;
    padding: 0 14px;
    box-sizing: border-box;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-top: 1px solid var(--td-component-stroke);
  }
}

.doc-empty-state {
  flex: 1;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  min-height: 100%;
}

.knowledge-card {
  min-width: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--td-component-border);
  height: 164px;
  border-radius: var(--app-radius-md);
  overflow: hidden;
  box-sizing: border-box;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
  background: var(--td-bg-color-container);
  position: relative;
  cursor: pointer;
  transition: border-color var(--app-motion-base) ease, box-shadow var(--app-motion-base) ease, background-color var(--app-motion-base) ease;

  .card-content {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding: 12px;
  }

  .card-content-nav {
    flex-shrink: 0;
    display: flex;
    align-items: flex-start;
    gap: 0;
    margin-bottom: 6px;
  }

  .card-bottom {
    flex-shrink: 0;
    margin-top: auto;
    padding: 0 14px;
    box-sizing: border-box;
    height: 32px;
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: var(--td-bg-color-container);
    border-top: 1px solid var(--td-component-stroke);
  }

}

.knowledge-card:hover {
  border-color: color-mix(in srgb, var(--td-component-stroke) 55%, var(--td-brand-color));
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.07);
}
</style>