<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import DocContent from '@/components/doc-content.vue';
import ContextualGuide from '@/components/ContextualGuide.vue';
import { useUIStore } from '@/stores/ui';
import { useAuthStore } from '@/stores/auth';
import { useChatResourcesStore } from '@/stores/chatResources';
import { useEditorResourcesStore } from '@/stores/editorResources';
import { isCKnowledgeConsole } from '@/utils/ckConsole';
import { getKnowledgeDetails } from '@/api/knowledge-base/index';
import KnowledgeBaseEditorModal from './KnowledgeBaseEditorModal.vue';
import KnowledgeBaseHeader from './components/KnowledgeBaseHeader.vue';
import KnowledgeBaseToolbar from './components/KnowledgeBaseToolbar.vue';
import KnowledgeDocumentPane from './components/KnowledgeDocumentPane.vue';
import KnowledgeMembersDialog from './components/KnowledgeMembersDialog.vue';
import KnowledgeUploadEntryDialog from './components/KnowledgeUploadEntryDialog.vue';
import KbFolderTree from './components/KbFolderTree.vue';
import BatchTagDialog from './components/BatchTagDialog.vue';
import WikiBrowser from './wiki/WikiBrowser.vue';
import ImageGallery from './gallery/ImageGallery.vue';
import FAQEntryManager from './components/FAQEntryManager.vue';
import { ROOT_FOLDER_PATH } from './folderTree';
import { MAX_FILE_SIZE_MB } from '@/utils';
import { useKnowledgeBaseDetail, type KnowledgeCard } from './composables/useKnowledgeBaseDetail';
import { useKnowledgePermissions } from './composables/useKnowledgePermissions';
import { useKnowledgeDocumentActions } from './composables/useKnowledgeDocumentActions';
import { useKnowledgeUploadFlow } from './composables/useKnowledgeUploadFlow';

const route = useRoute();
const router = useRouter();
const { t } = useI18n();
const uiStore = useUIStore();
const authStore = useAuthStore();
const chatResources = useChatResourcesStore();
const editorResources = useEditorResourcesStore();
const kbId = computed(() => ((route.params as any).kbId as string) || '');

const detail = useKnowledgeBaseDetail({ kbId });
const {
  kbInfo,
  kbLoading,
  docListLoading,
  isFAQ,
  isWiki,
  wikiStatus,
  wikiIsIndexing,
  wikiIndexingTip,
  knowledgeList,
  cardList,
  total,
  moreIndex,
  details,
  getfDetails,
  knowledgeScroll,
  loadKnowledgeList,
  loadKnowledgeFiles,
  loadKnowledgeBaseInfo,
  loadTags,
  loadFolderTree,
  resetPage,
  scheduleWikiStatusProbes,
  selectedTagIds,
  tagList,
  filterTagOptions,
  tagLoading,
  tagSearchQuery,
  tagHasMore,
  tagLoadingMore,
  handleTagFilterChange,
  onTagCatalogChanged,
  docSearchKeyword,
  selectedFileType,
  fileTypeOptions,
  selectedParseStatus,
  parseStatusOptions,
  selectedSource,
  sourceOptions,
  updatedTimeRange,
  filtersExpanded,
  activeFilterCount,
  clearDocumentFilters,
  disableFutureDate,
  documentSortPanelVisible,
  selectedDocumentSort,
  documentSortGroups,
  activeDocumentSortLabel,
  handleDocumentSortSelect,
  folderTree,
  folderTreeLoading,
  selectedFolderPath,
  folderTreeCollapsed,
  hasFolders,
  showFolderTree,
  isFiltering,
  showDocumentFolderPath,
  folderBreadcrumbs,
  folderOptions,
  handleFolderSelect,
  handleFolderRename,
  handleFolderTreeCollapsedChange,
  traceAvailableById,
  pendingReparseAck,
  isParseInFlight,
  probeTraceAvailable,
  handleScroll,
} = detail;

const permissions = useKnowledgePermissions({ kbId, kbInfo, isFAQ });
const { canEdit, canUpload, canManage, canManageKbMembers, canDeleteKnowledge, canMutateKnowledge, canDownloadKnowledge } = permissions;

const actions = useKnowledgeDocumentActions({
  kbId,
  kbInfo,
  isFAQ,
  cardList,
  total,
  moreIndex,
  details,
  getCardDetails: detail.getCardDetails,
  loadKnowledgeFiles,
  loadFolderTree,
  loadTags,
  resetPage,
  scheduleWikiStatusProbes,
  probeTraceAvailable,
  traceAvailableById,
  pendingReparseAck,
  isParseInFlight,
  folderOptions,
  knowledgeScroll,
  selectedDocumentSort,
  selectedFolderPath,
  selectedTagIds,
  docSearchKeyword,
  selectedFileType,
  selectedParseStatus,
  selectedSource,
  updatedTimeRange,
  canEdit,
  canDeleteKnowledge,
  canMutateKnowledge,
  canDownloadKnowledge,
});
const {
  viewMode,
  selectedIds,
  batchMode,
  toggleBatchMode,
  handleBatchCancel,
  toggleSelectRow,
  onCardGridCheckboxChange,
  toggleSelectAll,
  clearSelection,
  onDocMarqueeMouseDown,
  docMarqueeVisible,
  docMarqueeMode,
  docMarqueeBoxStyle,
  batchDeleting,
  batchReparsing,
  batchTagging,
  batchDownloading,
  handleBatchDownload,
  batchTagDialogVisible,
  batchTagPreSelectedIds,
  confirmBatchReparse,
  handleBatchTag,
  onBatchTagConfirm,
  confirmBatchDelete,
  moveMenuMode,
  moveTargetKbs,
  moveTargetsLoading,
  moveSelectedTargetName,
  moveMode,
  moveSubmitting,
  resetMoveMenu,
  moveKnowledgeIntoFolder,
  handleMoveSelectTarget,
  handleMoveBack,
  handleMoveConfirm,
  stopMovePoll,
  isCardDetails,
  docContentRef,
  closeDoc,
  openCardDetails,
  openSourceDoc,
  handleCardAction,
  handleListAction,
  openKnowledgeItem,
  syncDocumentSummaryState,
  confirmRebuildKnowledge,
  isManualDraftKnowledge,
} = actions;

// Active view: documents / wiki / graph / gallery. Wiki-graph only exist on
// wiki KBs; a stale tab from the URL falls back to documents in the header.
const validTabs = ['documents', 'wiki', 'graph', 'gallery'] as const;
type KbTab = (typeof validTabs)[number];
const initTab = validTabs.includes(route.query.tab as any) ? (route.query.tab as KbTab) : 'documents';
const activeKbTab = ref<KbTab>(initTab);

const kbViewTabs = computed(() => {
  const w = 'knowledgeEditor.wikiBrowser';
  const tabs: Array<{ key: KbTab; icon: string; label: string; tip: string; indexing?: boolean }> = [
    { key: 'documents', icon: 'file', label: t(`${w}.tabDocuments`), tip: t(`${w}.tabDocumentsTip`) },
  ];
  if (isWiki.value) {
    const indexing = wikiIsIndexing.value;
    tabs.push(
      { key: 'wiki', icon: 'book-open', label: 'Wiki', tip: indexing ? wikiIndexingTip.value : t(`${w}.tabWikiTip`), indexing },
      { key: 'graph', icon: 'relation', label: t(`${w}.tabGraph`), tip: indexing ? wikiIndexingTip.value : t(`${w}.tabGraphTip`), indexing },
    );
  }
  tabs.push({ key: 'gallery', icon: 'image', label: t(`${w}.tabGallery`), tip: t(`${w}.tabGalleryTip`) });
  return tabs;
});
const shownKbTab = computed<KbTab>(() =>
  kbViewTabs.value.some((tab) => tab.key === activeKbTab.value) ? activeKbTab.value : 'documents',
);

const onWikiStatusChange = (payload: { pendingTasks: number; isActive: boolean; pendingIssues: number }) => {
  wikiStatus.value = payload;
};
const onViewWikiInGraph = async (slug: string) => {
  // Write tab+slug first so the activeKbTab watcher's later replace
  // (which spreads route.query) preserves slug instead of clobbering it.
  await router.replace({ query: { ...route.query, tab: 'graph', slug } });
  activeKbTab.value = 'graph';
};

// Sync activeKbTab to URL query so it survives page refresh.
watch(activeKbTab, (tab) => {
  const query = { ...route.query };
  if (tab === 'documents') {
    delete query.tab;
  } else {
    query.tab = tab;
  }
  router.replace({ query });
});

const toolbarRef = ref<{ openUrlDialog?: () => void; openFileDialog?: () => boolean } | null>(null);
const membersRef = ref<{ open: () => void } | null>(null);
const openMembers = () => membersRef.value?.open();

// The editor modal's success callback resets the page and refreshes the list.
const manualEditorSuccess = ({ kbId: savedKbId }: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) => {
  if (savedKbId === kbId.value && !isFAQ.value) {
    resetPage();
    loadKnowledgeFiles(savedKbId);
    void loadFolderTree(savedKbId);
  }
};

const upload = useKnowledgeUploadFlow({
  kbId,
  kbInfo,
  kbLoading,
  isFAQ,
  canUpload,
  isWiki,
  activeKbTab,
  selectedFolderPath,
  folderOptions,
  selectedTagIds,
  supportedFileTypes: detail.supportedFileTypes,
  acceptFileTypes: detail.acceptFileTypes,
  missingStorageEngine: detail.missingStorageEngine,
  toolbarRef,
  resetPage,
  loadKnowledgeFiles,
  loadFolderTree,
  loadTags,
  scheduleWikiStatusProbes,
  manualEditorSuccess,
});
const {
  uploadEntryVisible,
  ensureDocumentKbReady,
  handleUploadSourceFiles,
  handleUploadSourceUrl,
  handleManualCreate,
  onUploadEntryDrop,
  openUploadEntryPicker,
  fileDragActive,
  handleKbDragEnter,
  handleKbDragOver,
  handleKbDragLeave,
  handleKbDrop,
} = upload;

const showKbDetailContextualGuide = computed(() => {
  return Boolean(kbId.value)
    && !isFAQ.value
    && canEdit.value
    && !docListLoading.value
    && cardList.value.length === 0;
});

const onCardMoreVisibleChange = (visible: boolean, item: KnowledgeCard) => {
  onVisibleChange(visible);
  if (visible) {
    probeTraceAvailable(item);
  }
};
const onVisibleChange = (visible: boolean) => {
  detail.onVisibleChange(visible);
  if (!visible) {
    resetMoveMenu();
  }
};

const handleOpenKBSettings = () => {
  if (!kbId.value) {
    MessagePlugin.warning(t('knowledgeEditor.messages.missingId'));
    return;
  }
  uiStore.openKBSettings(kbId.value);
};

const handleNavigateToKbList = () => {
  // C-Knowledge 的根级「知识库」面包屑回到自有首页，而不是引擎列表页。
  router.push(isCKnowledgeConsole() ? '/home' : '/platform/knowledge-bases');
};

const handleNavigateToCurrentKB = () => {
  if (!kbId.value) return;
  router.push(`/platform/knowledge-bases/${kbId.value}`);
};

const handleKnowledgeDropdownSelect = (data: { value: string }) => {
  if (!data?.value) return;
  if (data.value === kbId.value) return;
  router.push(`/platform/knowledge-bases/${data.value}`);
};

const selectKb = (id: string) => handleKnowledgeDropdownSelect({ value: id });

// Auto-open document detail when navigated with ?knowledge_id=xxx. The card
// list only contains the current folder, so resolve the document's folder
// from the detail endpoint before opening.
const pendingKnowledgeId = ref<string | null>((route.query.knowledge_id as string) || null);
let autoOpenRequest = 0;

const tryAutoOpenDocument = async () => {
  if (!pendingKnowledgeId.value) return;
  const targetId = pendingKnowledgeId.value;
  pendingKnowledgeId.value = null;
  const request = ++autoOpenRequest;
  const card = cardList.value.find((c: KnowledgeCard) => c.id === targetId);

  let target = card || ({ id: targetId } as KnowledgeCard);
  try {
    const response: any = await getKnowledgeDetails(targetId);
    if (request !== autoOpenRequest) return;
    const detailData = response?.data || response;
    if (detailData && typeof detailData === 'object') {
      target = { ...target, ...detailData, id: targetId } as KnowledgeCard;
      selectedFolderPath.value = detailData.folder_path || ROOT_FOLDER_PATH;
    }
  } catch (error) {
    console.error('Failed to resolve referenced document folder', error);
  }

  if (request !== autoOpenRequest) return;
  await nextTick();
  openCardDetails(target);
};

watch(cardList, () => {
  if (isFAQ.value) return;
  if (pendingKnowledgeId.value) {
    void tryAutoOpenDocument();
  }
});

watch(
  () => route.query.knowledge_id,
  (newId) => {
    if (typeof newId !== 'string' || !newId) return;
    pendingKnowledgeId.value = newId;
    void tryAutoOpenDocument();
  },
);

const handleOpenKnowledgeEvent = (e: Event) => {
  const payload = (e as CustomEvent<{ kbId: string; knowledgeId: string }>).detail;
  if (!payload || !payload.knowledgeId) return;
  if (payload.kbId && payload.kbId !== kbId.value) return;
  pendingKnowledgeId.value = payload.knowledgeId;
  void tryAutoOpenDocument();
};

const handleOpenURLImportDialog = (event: CustomEvent) => {
  const eventKbId = event.detail.kbId;
  if (eventKbId && eventKbId === kbId.value && !isFAQ.value) {
    if (ensureDocumentKbReady()) {
      toolbarRef.value?.openUrlDialog?.();
    }
  }
};

// Global file drops are captured by the platform shell. Route them back into
// the same confirmation flow as the page upload button so tags and per-batch
// processing settings are never skipped.
const handleKnowledgeFileDrop = (event: CustomEvent) => {
  const eventKbId = event.detail?.kbId;
  const files = Array.isArray(event.detail?.files) ? event.detail.files : [];
  if (eventKbId !== kbId.value || isFAQ.value || files.length === 0) return;
  handleUploadSourceFiles(files);
};

const getDoc = (page: number) => {
  getfDetails(details.id, page);
};

const handleKBEditorSuccess = (kbIdValue: string) => {
  chatResources.invalidateKnowledgeBaseDetail(kbIdValue);
  chatResources.invalidate('knowledgeBases');
  loadKnowledgeList();
  if (kbIdValue === kbId.value) {
    loadKnowledgeBaseInfo(kbIdValue, true);
  }
};

onMounted(() => {
  loadKnowledgeList();
  editorResources.ensureParserEngines();

  window.addEventListener('knowledgeFileUploaded', detail.handleFileUploaded as EventListener);
  window.addEventListener('openURLImportDialog', handleOpenURLImportDialog as EventListener);
  window.addEventListener('weknora:knowledge-file-drop', handleKnowledgeFileDrop as EventListener);
  window.addEventListener('weknora:open-knowledge', handleOpenKnowledgeEvent as EventListener);
});

onUnmounted(() => {
  window.removeEventListener('knowledgeFileUploaded', detail.handleFileUploaded as EventListener);
  window.removeEventListener('openURLImportDialog', handleOpenURLImportDialog as EventListener);
  window.removeEventListener('weknora:knowledge-file-drop', handleKnowledgeFileDrop as EventListener);
  window.removeEventListener('weknora:open-knowledge', handleOpenKnowledgeEvent as EventListener);
  stopMovePoll();
});
</script>

<template>
  <template v-if="!isFAQ">
    <div
      class="knowledge-layout"
      @dragenter="handleKbDragEnter"
      @dragover="handleKbDragOver"
      @dragleave="handleKbDragLeave"
      @drop="handleKbDrop"
    >
      <!-- 拖拽文件到文档区时的整页高亮；drop 仍由 platform shell 统一转交确认流程 -->
      <div v-if="fileDragActive" class="kb-drop-overlay" aria-hidden="true">
        <div class="kb-drop-overlay__card">
          <span class="kb-drop-overlay__icon">
            <t-icon name="upload" size="30px" />
          </span>
          <p class="kb-drop-overlay__title">松开即可上传到当前知识库</p>
          <p class="kb-drop-overlay__hint">会先打开上传确认，可设置标签与解析方式</p>
        </div>
      </div>

      <KnowledgeBaseHeader
        :kb-info="kbInfo"
        :kb-id="kbId"
        :knowledge-list="knowledgeList"
        :kb-view-tabs="kbViewTabs"
        :shown-kb-tab="shownKbTab"
        :can-manage="canManage"
        :unsupported-file-types="detail.unsupportedFileTypes.value"
        :missing-storage-engine="detail.missingStorageEngine.value"
        :supported-file-types="[...detail.supportedFileTypes.value]"
        :is-lite-mode="authStore.isLiteMode"
        @navigate-kb-list="handleNavigateToKbList"
        @navigate-current-kb="handleNavigateToCurrentKB"
        @select-kb="selectKb"
        @select-tab="(key: string) => activeKbTab = key as KbTab"
        @open-members="openMembers"
        @open-settings="handleOpenKBSettings"
        @go-parser-settings="detail.goToParserSettings"
      />

      <!-- Wiki Browser / Graph (shown when wiki or graph tab is active) -->
      <div v-if="isWiki && (activeKbTab === 'wiki' || activeKbTab === 'graph')" class="wiki-main-area">
        <WikiBrowser v-if="kbId" :knowledge-base-id="kbId" :view="activeKbTab === 'graph' ? 'graph' : 'browser'"
          :can-edit="canEdit" @open-source-doc="openSourceDoc" @status-change="onWikiStatusChange"
          @view-graph="onViewWikiInGraph" />
      </div>

      <!-- Image Gallery (4th tab) -->
      <ImageGallery v-if="activeKbTab === 'gallery' && kbId" :knowledge-base-id="kbId" @open-source-doc="openSourceDoc" />

      <!-- wiki/graph tabs only exist on wiki KBs; a stale tab (?tab= or one
           carried over from a previous KB) falls back to documents. -->
      <template v-if="activeKbTab === 'documents' || (!isWiki && activeKbTab !== 'gallery')">
        <div class="knowledge-main">
          <KbFolderTree v-if="showFolderTree && !folderTreeCollapsed" :tree="folderTree" :selected-path="selectedFolderPath"
            :loading="folderTreeLoading" :can-edit="canEdit" :root-label="kbInfo?.name"
            @select="handleFolderSelect" @update:collapsed="handleFolderTreeCollapsedChange"
            @rename="handleFolderRename" />
          <div class="tag-content">
            <div class="doc-card-area">
              <KnowledgeBaseToolbar
                ref="toolbarRef"
                v-model:doc-search-keyword="docSearchKeyword"
                v-model:filters-expanded="filtersExpanded"
                v-model:selected-file-type="selectedFileType"
                v-model:selected-parse-status="selectedParseStatus"
                v-model:selected-source="selectedSource"
                v-model:updated-time-range="updatedTimeRange"
                v-model:tag-search-query="tagSearchQuery"
                v-model:view-mode="viewMode"
                v-model:document-sort-panel-visible="documentSortPanelVisible"
                :kb-id="kbId"
                :kb-info="kbInfo"
                :show-folder-tree="showFolderTree"
                :folder-tree-collapsed="folderTreeCollapsed"
                :folder-breadcrumbs="folderBreadcrumbs"
                :is-filtering="isFiltering"
                :total="total"
                :item-count="cardList.length"
                :doc-list-loading="docListLoading"
                :active-filter-count="activeFilterCount"
                :file-type-options="fileTypeOptions"
                :parse-status-options="parseStatusOptions"
                :source-options="sourceOptions"
                :disable-future-date="disableFutureDate"
                :tag-loading="tagLoading"
                :filter-tag-options="filterTagOptions"
                :selected-tag-ids="selectedTagIds"
                :tag-has-more="tagHasMore"
                :tag-loading-more="tagLoadingMore"
                :batch-mode="batchMode"
                :batch-deleting="batchDeleting"
                :batch-reparsing="batchReparsing"
                :batch-tagging="batchTagging"
                :batch-downloading="batchDownloading"
                :document-sort-groups="documentSortGroups"
                :selected-document-sort="selectedDocumentSort"
                :active-document-sort-label="activeDocumentSortLabel"
                :can-download-knowledge="canDownloadKnowledge"
                :can-mutate-knowledge="canMutateKnowledge"
                :can-upload="canUpload"
                :accept-file-types="detail.acceptFileTypes.value"
                :supported-file-types="[...detail.supportedFileTypes.value]"
                @folder-select="handleFolderSelect"
                @expand-folder-tree="handleFolderTreeCollapsedChange(false)"
                @search-submit="loadKnowledgeFiles(kbId)"
                @clear-filters="clearDocumentFilters"
                @tag-filter-change="handleTagFilterChange"
                @load-more-tags="loadTags(kbId)"
                @toggle-batch="toggleBatchMode"
                @sort-select="handleDocumentSortSelect"
                @upload-files="handleUploadSourceFiles"
                @upload-url="handleUploadSourceUrl"
                @manual-create="handleManualCreate"
              />

              <KnowledgeDocumentPane
                :kb-id="kbId"
                :items="cardList"
                :view-mode="viewMode"
                :doc-list-loading="docListLoading"
                :batch-mode="batchMode"
                :selected-ids="selectedIds"
                :folder-options="folderOptions"
                :show-document-folder-path="showDocumentFolderPath"
                :can-edit="canEdit"
                :can-download-knowledge="canDownloadKnowledge"
                :can-delete-knowledge="canDeleteKnowledge"
                :can-mutate-knowledge="canMutateKnowledge"
                :trace-available-by-id="traceAvailableById"
                :move-menu-mode="moveMenuMode"
                :move-target-kbs="moveTargetKbs"
                :move-targets-loading="moveTargetsLoading"
                :move-selected-target-name="moveSelectedTargetName"
                :move-mode="moveMode"
                :move-submitting="moveSubmitting"
                :has-folders="hasFolders"
                :selected-folder-path="selectedFolderPath"
                :is-filtering="isFiltering"
                :doc-marquee-visible="docMarqueeVisible"
                :doc-marquee-mode="docMarqueeMode"
                :doc-marquee-box-style="docMarqueeBoxStyle"
                :batch-deleting="batchDeleting"
                :batch-reparsing="batchReparsing"
                :batch-tagging="batchTagging"
                :batch-downloading="batchDownloading"
                @scroll-ref="(el: any) => knowledgeScroll = el"
                @scroll="handleScroll"
                @marquee-mousedown="onDocMarqueeMouseDown"
                @open="openKnowledgeItem"
                @open-folder="handleFolderSelect"
                @move-to-folder="(item: any, path: string) => moveKnowledgeIntoFolder([item.id], path)"
                @toggle-checkbox="onCardGridCheckboxChange"
                @menu-visible-change="onCardMoreVisibleChange"
                @action="handleCardAction"
                @tags-changed="onTagCatalogChanged"
                @probe-trace="probeTraceAvailable"
                @toggle-row="toggleSelectRow"
                @toggle-all="toggleSelectAll"
                @move-select-target="handleMoveSelectTarget"
                @move-back="handleMoveBack"
                @move-confirm="handleMoveConfirm"
                @update:move-mode="(mode: any) => moveMode = mode"
                @reset-move-state="resetMoveMenu"
                @batch-cancel="handleBatchCancel"
                @batch-delete="confirmBatchDelete"
                @batch-reparse="confirmBatchReparse"
                @batch-tag="handleBatchTag"
                @batch-download="handleBatchDownload"
                @batch-select-loaded="toggleSelectAll(true)"
                @batch-move-to-folder="(path: string) => moveKnowledgeIntoFolder(Array.from(selectedIds), path)"
              />
            </div>
          </div>
        </div>
      </template>

      <!-- DocContent drawer (shared by documents tab and wiki source refs) -->
      <DocContent ref="docContentRef" :visible="isCardDetails" :details="details" :canEditKB="canEdit"
        :canDownloadKB="canDownloadKnowledge" :kbId="kbId"
        @closeDoc="closeDoc" @getDoc="getDoc" @summaryStateChange="syncDocumentSummaryState">
      </DocContent>
    </div>
  </template>
  <template v-else>
    <div class="faq-manager-wrapper">
      <FAQEntryManager v-if="kbId" :kb-id="kbId" />
    </div>
  </template>

  <!-- 知识库编辑器（创建/编辑统一组件） -->
  <KnowledgeBaseEditorModal :visible="uiStore.showKBEditorModal" :mode="uiStore.kbEditorMode"
    :kb-id="uiStore.currentKBId || undefined" :initial-type="uiStore.kbEditorType"
    @update:visible="(val) => val ? null : uiStore.closeKBEditor()" @success="handleKBEditorSuccess" />

  <ContextualGuide tour="kbDetail" :when="showKbDetailContextualGuide" />

  <!-- 批量打标签弹窗 -->
  <BatchTagDialog @tags-changed="onTagCatalogChanged" :visible="batchTagDialogVisible"
    :count="selectedIds.size" :kb-id="kbId"
    :pre-selected-tag-ids="batchTagPreSelectedIds"
    :confirm-loading="batchTagging"
    @update:visible="batchTagDialogVisible = $event" @confirm="onBatchTagConfirm" />

  <KnowledgeUploadEntryDialog
    v-model:visible="uploadEntryVisible"
    :kb-name="kbInfo?.name"
    :max-file-size-mb="MAX_FILE_SIZE_MB"
    @drop="onUploadEntryDrop"
    @pick="openUploadEntryPicker"
  />

  <!-- 成员与权限：库级授权（项目库默认需要在这里添加成员） -->
  <KnowledgeMembersDialog
    ref="membersRef"
    :kb-id="kbId"
    :kb-info="kbInfo"
    :can-manage-kb-members="canManageKbMembers"
  />
</template>

<style>
/* 下拉菜单容器样式已统一至 @/assets/dropdown-menu.less */
.document-filter-popup .t-popup__content {
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-xl);
  box-shadow: 0 8px 32px rgb(0 0 0 / 10%);
}
</style>

<style scoped lang="less">
.knowledge-layout {
  position: relative;
  display: flex;
  flex-direction: column;
  margin: 0 16px 0 0;
  gap: 16px;
  height: 100%;
  flex: 1;
  width: 100%;
  min-width: 0;
  padding: 20px 28px 0;
  box-sizing: border-box;
}

.wiki-main-area {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

// Directory navigation and the document content share the available width.
.knowledge-main {
  display: flex;
  flex: 1;
  min-height: 0;
  background: transparent;
  border: none;
}

@media (max-width: 1000px) {
  .knowledge-main {
    flex-direction: column;
    :deep(.kb-folder-tree) {
      width: 100%;
      max-height: 200px;
      padding: 0 0 12px;
      margin: 0 0 16px;
      border-right: 0;
      border-bottom: 1px solid var(--td-component-stroke);
    }
    :deep(.kb-folder-tree__header) { height: 28px; padding-bottom: 4px; }
  }
}

.tag-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: 0;
  border: none;
  overflow: hidden;
  background: transparent;
}

.doc-card-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  position: relative;
  container-type: inline-size;
  container-name: doc-card-area;
}

.faq-manager-wrapper {
  flex: 1;
  min-height: 0;
  padding: 24px 32px;
  overflow-y: auto;
  margin: 0 16px 0 4px;
}
</style>