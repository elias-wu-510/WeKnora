import { computed, nextTick, onUnmounted, ref, watch, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { useUIStore } from '@/stores/ui';
import { useUploadConfirmStore } from '@/stores/uploadConfirm';
import {
  batchDeleteKnowledge,
  batchDownloadKnowledge,
  batchQueryKnowledge,
  batchReparseKnowledge,
  cancelKnowledgeParse,
  delKnowledgeDetails,
  downKnowledgeDetails,
  getKnowledgeDetails,
  getKnowledgeMoveProgress,
  listMoveTargets,
  moveKnowledge,
  moveKnowledgeToFolder,
  reparseKnowledge,
  updateKnowledgeTagBatch,
} from '@/api/knowledge-base';
import { isBatchDownloadableKnowledge, resolveKnowledgeDownloadFileName } from '../knowledgeDownloadFileName';
import { waitForKnowledgeDeletion } from '@/utils/knowledgeDeletion';
import { useMarqueeSelect } from '@/hooks/useMarqueeSelect';
import type { KnowledgeProcessOverrides } from '@/types/knowledgeProcess';
import type { KnowledgeCard } from './useKnowledgeBaseDetail';

export type DocumentAction =
  | 'download'
  | 'edit'
  | 'reparse'
  | 'cancel-parse'
  | 'move'
  | 'move-folder'
  | 'delete'
  | 'view-trace'
  | 'batch-manage';

type FolderOption = { path: string; name: string; depth: number };

/**
 * Document-level actions for the KB detail page: selection/batch, move,
 * delete/reparse/download, card & list event bridges and detail drawer state.
 */
export function useKnowledgeDocumentActions(options: {
  kbId: Ref<string>;
  kbInfo: Ref<any>;
  isFAQ: Ref<boolean>;
  cardList: Ref<KnowledgeCard[]>;
  total: Ref<number>;
  moreIndex: Ref<number>;
  details: any;
  getCardDetails: (item: any) => void;
  loadKnowledgeFiles: (kbId: string) => Promise<void>;
  loadFolderTree: (kbId: string) => Promise<void>;
  loadTags: (kbId: string, reset?: boolean) => Promise<void>;
  resetPage: () => void;
  scheduleWikiStatusProbes: () => void;
  probeTraceAvailable: (item: KnowledgeCard) => Promise<void>;
  traceAvailableById: Record<string, boolean>;
  pendingReparseAck: Ref<Set<string>>;
  isParseInFlight: (status?: string) => boolean;
  folderOptions: Ref<FolderOption[]>;
  knowledgeScroll: Ref<any>;
  selectedDocumentSort: Ref<any>;
  selectedFolderPath: Ref<string>;
  selectedTagIds: Ref<string[]>;
  docSearchKeyword: Ref<string>;
  selectedFileType: Ref<string>;
  selectedParseStatus: Ref<string>;
  selectedSource: Ref<string>;
  updatedTimeRange: Ref<string[]>;
  canEdit: Ref<boolean>;
  canDeleteKnowledge: Ref<boolean>;
  canMutateKnowledge: Ref<boolean>;
  canDownloadKnowledge: Ref<boolean>;
}) {
  const {
    kbId,
    kbInfo,
    isFAQ,
    cardList,
    total,
    moreIndex,
    details,
    getCardDetails,
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
  } = options;
  const { t } = useI18n();
  const uiStore = useUIStore();
  const uploadConfirmStore = useUploadConfirmStore();

  // View mode (grid / list) — persisted per browser
  type DocViewMode = 'grid' | 'list';
  const VIEW_MODE_KEY = 'weknora.kb.docs.viewMode';
  const initViewMode = (): DocViewMode => {
    try {
      return localStorage.getItem(VIEW_MODE_KEY) === 'grid' ? 'grid' : 'list';
    } catch { return 'list'; }
  };
  const viewMode = ref<DocViewMode>(initViewMode());
  watch(viewMode, (v) => {
    try { localStorage.setItem(VIEW_MODE_KEY, v); } catch { /* ignore */ }
  });

  // Multi-select state — shared between grid and list views.
  // Vue 3.5 tracks Set#add/delete natively, so direct mutation is reactive.
  const selectedIds = ref<Set<string>>(new Set());
  let lastSelectedIndex = -1;
  const batchDeleting = ref(false);
  const batchReparsing = ref(false);
  const batchTagging = ref(false);
  const batchDownloading = ref(false);
  let batchDownloadController: AbortController | undefined;

  const MAX_BATCH_DOWNLOAD_FILES = 200;
  const MAX_BATCH_DOWNLOAD_BYTES = 512 * 1024 * 1024;

  const handleBatchDownload = async () => {
    if (batchDownloading.value || batchDeleting.value || batchReparsing.value || batchTagging.value || selectedIds.value.size === 0) return;
    const selected = Array.from(selectedIds.value);
    const itemsById = new Map((cardList.value || []).map((item) => [item.id, item]));
    const ids = selected.filter((id) => isBatchDownloadableKnowledge(itemsById.get(id)));
    const skipped = selected.length - ids.length;
    if (ids.length === 0) {
      MessagePlugin.warning(t('knowledgeBase.batchDownloadNoFiles'));
      return;
    }
    if (ids.length > MAX_BATCH_DOWNLOAD_FILES) {
      MessagePlugin.warning(t('knowledgeBase.batchDownloadHint'));
      return;
    }
    let knownBytes = 0;
    for (const id of ids) {
      const size = Number(itemsById.get(id)?.file_size);
      if (Number.isFinite(size) && size > 0) knownBytes += size;
    }
    if (knownBytes > MAX_BATCH_DOWNLOAD_BYTES) {
      MessagePlugin.warning(t('knowledgeBase.batchDownloadTooLarge'));
      return;
    }
    if (skipped > 0) {
      MessagePlugin.warning(t('knowledgeBase.batchDownloadSkipped', { count: skipped }));
    }
    const controller = new AbortController();
    batchDownloadController = controller;
    batchDownloading.value = true;
    try {
      const blob = await batchDownloadKnowledge(kbId.value, ids, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `knowledge-files-${Date.now()}.zip`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      // 延迟释放，给浏览器留出开始保存文件的时间。
      window.setTimeout(() => URL.revokeObjectURL(url), 60000);
      MessagePlugin.success(t('knowledgeBase.batchDownloadStarted'));
    } catch (error: any) {
      if (!controller.signal.aborted) {
        MessagePlugin.error(error?.message || t('knowledgeBase.batchDownloadFailed'));
      }
    } finally {
      if (batchDownloadController === controller) {
        batchDownloading.value = false;
        batchDownloadController = undefined;
      }
    }
  };
  watch(kbId, () => batchDownloadController?.abort());
  onUnmounted(() => batchDownloadController?.abort());

  const batchTagDialogVisible = ref(false);
  const batchTagPreSelectedIds = computed(() => {
    const ids = Array.from(selectedIds.value);
    if (ids.length === 0) return [];
    const cards = ids
      .map((id) => cardList.value.find((c) => c.id === id))
      .filter((c): c is KnowledgeCard => Boolean(c));
    if (cards.length === 0) return [];
    const firstTagIds = new Set((cards[0].tags || []).map((t) => t.id));
    for (let i = 1; i < cards.length; i++) {
      const cur = new Set((cards[i].tags || []).map((t) => t.id));
      for (const tid of firstTagIds) {
        if (!cur.has(tid)) firstTagIds.delete(tid);
      }
    }
    return Array.from(firstTagIds);
  });

  const applyOptimisticBatchReparse = (ids: string[]) => {
    const idSet = new Set(ids);
    for (const card of cardList.value) {
      if (!idSet.has(card.id)) continue;
      pendingReparseAck.value.add(card.id);
      card.parse_status = 'pending';
      card.summary_status = undefined;
      card.description = '';
      delete traceAvailableById[card.id];
      traceAvailableById[card.id] = true;
    }
  };

  const syncReparseAckFromServer = (ids: string[]) => {
    for (const id of ids) {
      if (!pendingReparseAck.value.has(id)) continue;
      const card = cardList.value.find((c) => c.id === id);
      if (card && isParseInFlight(card.parse_status)) {
        pendingReparseAck.value.delete(id);
      }
    }
  };

  const awaitBatchReparseReflection = async (ids: string[]) => {
    const maxPolls = 30;
    const delayMs = 400;
    for (let i = 0; i < maxPolls && pendingReparseAck.value.size > 0; i++) {
      await loadKnowledgeFiles(kbId.value);
      syncReparseAckFromServer(ids);
      applyOptimisticBatchReparse(Array.from(pendingReparseAck.value));
      await new Promise<void>((r) => setTimeout(r, delayMs));
    }
    pendingReparseAck.value.clear();
  };

  const confirmBatchReparse = async () => {
    if (batchReparsing.value || batchDeleting.value || batchDownloading.value || selectedIds.value.size === 0) return;
    const allIds = Array.from(selectedIds.value);
    const ids = allIds.filter((id) => {
      const item = cardList.value.find((c) => c.id === id);
      return !item || !isParseInFlight(item.parse_status);
    });
    const skipped = allIds.length - ids.length;
    if (ids.length === 0) {
      MessagePlugin.info(t('knowledgeBase.rebuildInProgress'));
      return;
    }
    if (skipped > 0) {
      MessagePlugin.warning(t('knowledgeBase.batchReparseSkippedInFlight', { count: skipped }));
    }
    batchReparsing.value = true;
    try {
      const res: any = await batchReparseKnowledge(kbId.value, ids);
      if (res?.success) {
        MessagePlugin.success(t('knowledgeBase.batchReparseSuccess', { count: ids.length }));
        applyOptimisticBatchReparse(ids);
        clearSelection();
        batchMode.value = false;
        scheduleWikiStatusProbes();
        void awaitBatchReparseReflection(ids);
      } else {
        MessagePlugin.error(res?.message || t('knowledgeBase.batchReparseFailed'));
      }
    } catch (e: any) {
      MessagePlugin.error(e?.message || t('knowledgeBase.batchReparseFailed'));
    } finally {
      batchReparsing.value = false;
    }
  };

  // Move state — inline in card menu
  const moveMenuMode = ref<'normal' | 'targets' | 'confirm'>('normal');
  const moveKnowledgeId = ref('');
  const moveTargetKbs = ref<any[]>([]);
  const moveTargetsLoading = ref(false);
  const moveSelectedTargetId = ref('');
  const moveSelectedTargetName = ref('');
  const moveMode = ref<'reuse_vectors' | 'reparse'>('reuse_vectors');
  const moveSubmitting = ref(false);
  let movePollTimer: ReturnType<typeof setInterval> | null = null;

  const resetMoveMenu = () => {
    moveMenuMode.value = 'normal';
  };

  const moveKnowledgeIntoFolder = async (ids: string[], folderPath: string) => {
    if (!kbId.value || ids.length === 0) return;
    try {
      await moveKnowledgeToFolder(kbId.value, ids, folderPath);
      MessagePlugin.success(t('knowledgeBase.moveToFolder.success', { count: ids.length }));
      clearSelection();
      batchMode.value = false;
      resetPage();
      await loadKnowledgeFiles(kbId.value);
      await loadFolderTree(kbId.value);
    } catch (error: any) {
      MessagePlugin.error(error?.message || t('knowledgeBase.moveToFolder.failed'));
    }
  };

  const isCardDetails = ref(false);
  const docContentRef = ref<any>(null);
  const closeDoc = () => {
    isCardDetails.value = false;
  };
  const openCardDetails = (item: KnowledgeCard) => {
    isCardDetails.value = true;
    getCardDetails(item);
  };

  // Open source document preview from WikiBrowser
  const openSourceDoc = (knowledgeId: string) => {
    isCardDetails.value = true;
    getCardDetails({ id: knowledgeId });
  };

  const closeCardMoreMenu = (index: number) => {
    if (cardList.value?.[index]) {
      cardList.value[index].isMore = false;
    }
    moreIndex.value = -1;
  };

  const confirmDeleteKnowledge = (index: number, item: KnowledgeCard) => {
    closeCardMoreMenu(index);
    void deleteKnowledgeDocuments([item.id], () => delKnowledgeDetails(item.id), false);
  };

  const onReparseMenuClick = (index: number, item: KnowledgeCard) => {
    if (isParseInFlight(item.parse_status)) {
      MessagePlugin.info(t('knowledgeBase.rebuildInProgress'));
    }
  };

  const handleMoveKnowledge = async (item: KnowledgeCard) => {
    moveKnowledgeId.value = item.id;
    moveMenuMode.value = 'targets';
    moveTargetsLoading.value = true;
    moveTargetKbs.value = [];
    try {
      const res: any = await listMoveTargets(kbId.value);
      moveTargetKbs.value = res.data || [];
    } catch {
      moveTargetKbs.value = [];
    } finally {
      moveTargetsLoading.value = false;
    }
  };

  const handleMoveSelectTarget = (kb: any) => {
    moveSelectedTargetId.value = kb.id;
    moveSelectedTargetName.value = kb.name;
    moveMode.value = 'reuse_vectors';
    moveMenuMode.value = 'confirm';
  };

  const handleMoveBack = () => {
    if (moveMenuMode.value === 'confirm') {
      moveMenuMode.value = 'targets';
    } else {
      moveMenuMode.value = 'normal';
    }
  };

  const handleMoveConfirm = async () => {
    if (!moveSelectedTargetId.value || moveSubmitting.value) return;
    moveSubmitting.value = true;
    try {
      const res: any = await moveKnowledge({
        knowledge_ids: [moveKnowledgeId.value],
        source_kb_id: kbId.value,
        target_kb_id: moveSelectedTargetId.value,
        mode: moveMode.value,
      });
      const taskId = res.data?.task_id;
      MessagePlugin.info(t('knowledgeBase.moveStarted'));
      // Close the card menu
      moveMenuMode.value = 'normal';
      cardList.value.forEach(c => { c.isMore = false; });

      if (taskId) {
        startMovePoll(taskId);
      } else {
        moveSubmitting.value = false;
        resetPage(); // Reset page counter when reloading files after move
        loadKnowledgeFiles(kbId.value);
        void loadFolderTree(kbId.value);
      }
    } catch (e: any) {
      MessagePlugin.error(e?.message || t('knowledgeBase.moveFailed'));
      moveSubmitting.value = false;
    }
  };

  const startMovePoll = (taskId: string) => {
    if (movePollTimer) clearInterval(movePollTimer);
    movePollTimer = setInterval(async () => {
      try {
        const res: any = await getKnowledgeMoveProgress(taskId);
        const data = res.data;
        if (!data) return;
        if (data.status === 'completed') {
          stopMovePoll();
          moveSubmitting.value = false;
          const failed = data.failed || 0;
          if (failed > 0) {
            MessagePlugin.warning(t('knowledgeBase.moveCompletedWithErrors', { success: (data.processed || 0) - failed, failed }));
          } else {
            MessagePlugin.success(t('knowledgeBase.moveCompleted'));
          }
          resetPage(); // Reset page counter when reloading files after move completion
          loadKnowledgeFiles(kbId.value);
          void loadFolderTree(kbId.value);
        } else if (data.status === 'failed') {
          stopMovePoll();
          moveSubmitting.value = false;
          MessagePlugin.error(t('knowledgeBase.moveFailed'));
        }
      } catch {
        // ignore poll errors
      }
    }, 2000);
  };

  const stopMovePoll = () => {
    if (movePollTimer) {
      clearInterval(movePollTimer);
      movePollTimer = null;
    }
  };

  const manualEditorSuccess = ({ kbId: savedKbId }: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) => {
    if (savedKbId === kbId.value && !isFAQ.value) {
      resetPage(); // Reset page counter when reloading files after manual edit
      loadKnowledgeFiles(savedKbId);
      void loadFolderTree(savedKbId);
    }
  };

  const handleManualEdit = (index: number, item: KnowledgeCard) => {
    if (isFAQ.value) return;
    if (cardList.value[index]) {
      cardList.value[index].isMore = false;
    }
    uiStore.openManualEditor({
      mode: 'edit',
      kbId: item.knowledge_base_id || kbId.value,
      knowledgeId: item.id,
      onSuccess: manualEditorSuccess,
    });
  };

  // Opens ONLY the trace drawer for this card — does NOT pop the
  // document detail drawer behind it. The trace drawer attaches to
  // body so it renders independent of its host's visibility; we just
  // need `details` populated so the timeline component knows which
  // knowledge_id to fetch. getCardDetails resets details synchronously
  // then fills asynchronously, so we re-stamp the id/parse_status
  // right after the call to avoid the brief empty-id window that
  // would otherwise prevent the drawer from mounting.
  const handleViewTrace = (index: number, item: KnowledgeCard) => {
    if (cardList.value[index]) {
      cardList.value[index].isMore = false;
    }
    moreIndex.value = -1;
    getCardDetails(item);
    details.id = item.id;
    details.parse_status = item.parse_status;
    nextTick(() => {
      docContentRef.value?.openTimeline?.();
    });
  };

  const confirmRebuildKnowledge = async (index: number, item: KnowledgeCard) => {
    if (isFAQ.value) return;
    if (!canEdit.value) return;
    if (!item?.id) {
      MessagePlugin.warning(t('knowledgeEditor.messages.missingId'));
      return;
    }
    if (isParseInFlight(item.parse_status)) {
      MessagePlugin.info(t('knowledgeBase.rebuildInProgress'));
      return;
    }
    closeCardMoreMenu(index);

    // No KB context to seed the dialog defaults — fall back to a direct reparse
    // that reuses the overrides stored at upload time.
    if (!kbInfo.value) {
      await submitReparse(item.id);
      return;
    }

    // Prefill the confirm dialog with the overrides this doc was last parsed with.
    let processOverrides: KnowledgeProcessOverrides | null = item.metadata?.process_overrides ?? null;
    let fileName = item.file_name || item.title || '';
    let fileType = item.file_type || '';
    try {
      const detail: any = await getKnowledgeDetails(item.id);
      if (detail?.success && detail.data) {
        processOverrides = detail.data.metadata?.process_overrides ?? processOverrides;
        fileName = detail.data.file_name || detail.data.title || fileName;
        fileType = detail.data.file_type || fileType;
      }
    } catch {
      // fall back to the list item's fields
    }

    try {
      const result = await uploadConfirmStore.open({
        mode: 'reparse',
        kbInfo: kbInfo.value,
        reparse: { knowledgeId: item.id, fileName, fileType, processOverrides },
      });
      if (result.mode === 'reparse' && result.reparse) {
        await submitReparse(result.reparse.knowledgeId, result.processConfig);
      }
    } catch {
      // cancelled
    }
  };

  const submitReparse = async (id: string, processConfig?: KnowledgeProcessOverrides) => {
    try {
      await reparseKnowledge(id, processConfig ? { process_config: processConfig } : undefined);
      delete traceAvailableById[id];
      traceAvailableById[id] = true;
      MessagePlugin.success(t('knowledgeBase.rebuildSubmitted'));
      resetPage();
      loadKnowledgeFiles(kbId.value);
      scheduleWikiStatusProbes();
    } catch (error: any) {
      MessagePlugin.error(error?.message || t('knowledgeBase.rebuildFailed'));
    }
  };

  const syncDocumentSummaryState = (state: { id?: string; summary_status?: string; description?: string }) => {
    if (!state?.id) return;
    const card = cardList.value.find((item: KnowledgeCard) => item.id === state.id);
    if (!card) return;
    if (typeof state.summary_status === 'string' && state.summary_status) {
      card.summary_status = state.summary_status;
    }
    if (typeof state.description === 'string') {
      card.description = state.description;
    }
  };

  const toggleSelectRow = (id: string, checked: boolean, shiftKey?: boolean) => {
    const items = cardList.value || [];
    const idx = items.findIndex((i: KnowledgeCard) => i.id === id);
    if (shiftKey && lastSelectedIndex >= 0 && idx >= 0) {
      const [s, e] = idx < lastSelectedIndex
        ? [idx, lastSelectedIndex]
        : [lastSelectedIndex, idx];
      for (let i = s; i <= e; i++) {
        if (checked) selectedIds.value.add(items[i].id);
        else selectedIds.value.delete(items[i].id);
      }
    } else {
      if (checked) selectedIds.value.add(id);
      else selectedIds.value.delete(id);
    }
    lastSelectedIndex = idx;
  };

  const onCardGridCheckboxChange = (id: string, checked: boolean, ctx?: { e?: Event }) => {
    const me = ctx?.e as MouseEvent | undefined;
    toggleSelectRow(id, checked, !!me?.shiftKey);
  };

  const toggleSelectAll = (checked: boolean) => {
    if (checked) {
      for (const item of cardList.value || []) selectedIds.value.add(item.id);
    } else {
      for (const item of cardList.value || []) selectedIds.value.delete(item.id);
    }
  };

  const clearSelection = () => {
    selectedIds.value.clear();
    lastSelectedIndex = -1;
  };

  // Batch (multi-select) mode mirrors the session list's "批量管理" UX: while off,
  // no checkbox is rendered so the title doesn't jitter on hover; while on,
  // checkboxes are persistent and clicking a card toggles its selection.
  const batchMode = ref(false);
  const toggleBatchMode = () => {
    batchMode.value = !batchMode.value;
    if (!batchMode.value) clearSelection();
  };
  // "取消选择" / 退出批量管理：清空选择，并退出 grid 视图下的批量模式。
  const handleBatchCancel = () => {
    clearSelection();
    batchMode.value = false;
  };
  // 切到卡片视图时，如果列表视图里已经勾选过文档，需要自动开启批量管理模式，
  // 否则卡片视图默认不渲染 checkbox，会看不到勾选态。
  watch(viewMode, (mode) => {
    if (mode === 'grid' && selectedIds.value.size > 0) {
      batchMode.value = true;
    }
  });
  // Triggered from a card / row "..." menu — match the session-list UX where
  // the menu item simply opens batch mode (no auto-selection).
  const handleEnterBatchFromCard = (item: any) => {
    if (item) item.isMore = false;
    moreIndex.value = -1;
    clearSelection();
    batchMode.value = true;
  };
  const {
    onContainerMouseDown: onDocMarqueeMouseDown,
    marqueeVisible: docMarqueeVisible,
    marqueeMode: docMarqueeMode,
    boxStyle: docMarqueeBoxStyle,
    shouldSuppressClick: shouldSuppressDocClick,
  } = useMarqueeSelect({
    containerRef: knowledgeScroll,
    itemSelector: '.knowledge-card[data-select-id], .doc-list-row[data-select-id]',
    selectedIds,
    getItemId: (el: any) => el.dataset.selectId || null,
    enabled: computed(() => (canEdit.value || canDownloadKnowledge.value) && !isFAQ.value && cardList.value.length > 0),
    onSelectionStart: () => {
      batchMode.value = true;
    },
  });

  const isManualDraftKnowledge = (item: KnowledgeCard) =>
    item.type === 'manual' && item.parse_status === 'draft';

  const openKnowledgeItem = (item: KnowledgeCard) => {
    if (shouldSuppressDocClick()) return;
    if (canEdit.value && isManualDraftKnowledge(item)) {
      const index = cardList.value.findIndex((c) => c.id === item.id);
      if (index >= 0) {
        handleManualEdit(index, item);
        return;
      }
    }
    openCardDetails(item);
  };

  // Stop observing a previous KB when navigating, including away and back.
  let deleteGeneration = 0;
  watch(kbId, () => {
    deleteGeneration++;
    batchDeleting.value = false;
  });
  onUnmounted(() => { deleteGeneration++; });

  const deleteKnowledgeDocuments = async (
    ids: string[],
    submit: () => Promise<any>,
    batch: boolean,
  ) => {
    if (batchDeleting.value || batchReparsing.value || batchDownloading.value || ids.length === 0) return;
    const targetKbId = kbId.value;
    const generation = ++deleteGeneration;
    const isActive = () => generation === deleteGeneration && isCurrentKb(targetKbId);
    batchDeleting.value = true;
    let submitted = false;
    try {
      const res = await submit();
      if (!isActive()) return;
      if (!res?.success) {
        MessagePlugin.error(res?.message || t('knowledgeBase.batchDeleteFailed'));
        return;
      }
      submitted = true;
      MessagePlugin.info(t('knowledgeBase.deleteSubmitted'));
      if (batch) {
        clearSelection();
        batchMode.value = false;
      }
      const result = await waitForKnowledgeDeletion(
        ids,
        async (queryIds) => {
          const query = new URLSearchParams();
          queryIds.forEach(id => query.append('ids', id));
          return await batchQueryKnowledge(query.toString(), targetKbId) as any;
        },
        { isActive },
      );
      if (!isActive() || result === 'cancelled') return;
      if (result === 'completed') {
        MessagePlugin.success(batch
          ? t('knowledgeBase.batchDeleteSuccess', { count: ids.length })
          : t('knowledgeBase.deleteSuccess'));
      } else if (result === 'failed') {
        MessagePlugin.error(t('knowledgeBase.deleteTaskFailed'));
      } else {
        MessagePlugin.info(t('knowledgeBase.deletePending'));
      }
    } catch (e: any) {
      if (!isActive()) return;
      if (submitted) {
        MessagePlugin.warning(t('knowledgeBase.deleteStatusUnavailable'));
      } else {
        MessagePlugin.error(e?.message || t('knowledgeBase.batchDeleteFailed'));
      }
    } finally {
      if (isActive()) {
        batchDeleting.value = false;
        if (submitted) {
          resetPage();
          await loadKnowledgeFiles(targetKbId);
          if (isActive()) {
            void loadTags(targetKbId, true);
            void loadFolderTree(targetKbId);
          }
        }
      }
    }
  };

  const isCurrentKb = (targetKbId: string) => targetKbId === kbId.value;

  const confirmBatchDelete = () => {
    const targetKbId = kbId.value;
    const ids = Array.from(selectedIds.value);
    return deleteKnowledgeDocuments(ids, () => batchDeleteKnowledge(targetKbId, ids), true);
  };

  const handleBatchTag = () => {
    if (batchDeleting.value || batchReparsing.value || batchTagging.value || batchDownloading.value || selectedIds.value.size === 0) return;
    batchTagDialogVisible.value = true;
  };

  const onBatchTagConfirm = async (tagIds: string[]) => {
    if (batchTagging.value || selectedIds.value.size === 0) return;
    const ids = Array.from(selectedIds.value);
    const updateMap: Record<string, string[]> = {};
    for (const id of ids) {
      updateMap[id] = tagIds;
    }
    batchTagging.value = true;
    try {
      await updateKnowledgeTagBatch({ updates: updateMap });
      MessagePlugin.success(t('knowledgeBase.batchTagSuccess', { count: ids.length }));
      batchTagDialogVisible.value = false;
      clearSelection();
      batchMode.value = false;
      resetPage();
      loadKnowledgeFiles(kbId.value);
      loadTags(kbId.value, true);
    } catch (e: any) {
      MessagePlugin.error(e?.message || t('knowledgeBase.batchTagFailed'));
    } finally {
      batchTagging.value = false;
    }
  };

  const confirmCancelParseKnowledge = async (item: KnowledgeCard) => {
    if (!item?.id) return;
    try {
      await cancelKnowledgeParse(item.id);
      MessagePlugin.success(t('knowledgeBase.cancelParseSubmitted'));
      loadKnowledgeFiles(kbId.value);
    } catch (error: any) {
      MessagePlugin.error(error?.message || t('knowledgeBase.cancelParseFailed'));
    }
  };

  const downloadKnowledge = async (item: KnowledgeCard) => {
    if (!item?.id) return;
    try {
      const file = await downKnowledgeDetails(item.id);
      const objectUrl = URL.createObjectURL(file);
      const link = document.createElement('a');
      const fileName = resolveKnowledgeDownloadFileName(item);
      link.style.display = 'none';
      link.href = objectUrl;
      link.download = fileName;
      document.body.appendChild(link);
      link.click();
      nextTick(() => {
        link.remove();
        URL.revokeObjectURL(objectUrl);
      });
    } catch {
      MessagePlugin.error(t('file.downloadFailed'));
    }
  };

  // Bridge card-view actions back to existing per-card handlers.
  const handleCardAction = (action: DocumentAction, item: KnowledgeCard) => {
    const idx = (cardList.value || []).findIndex((i: KnowledgeCard) => i.id === item.id);
    if (action === 'download') return downloadKnowledge(item);
    if (action === 'edit') return handleManualEdit(idx, item);
    if (action === 'reparse') {
      if (isParseInFlight(item.parse_status)) return onReparseMenuClick(idx, item);
      return confirmRebuildKnowledge(idx, item);
    }
    if (action === 'cancel-parse') return confirmCancelParseKnowledge(item);
    if (action === 'move') return handleMoveKnowledge(item);
    if (action === 'delete') return confirmDeleteKnowledge(idx, item);
    if (action === 'view-trace') return handleViewTrace(idx, item);
    if (action === 'batch-manage') return handleEnterBatchFromCard(item);
  };

  // Bridge list-view actions back to existing per-card handlers.
  const handleListAction = (action: DocumentAction, item: KnowledgeCard) => {
    const idx = (cardList.value || []).findIndex((i: KnowledgeCard) => i.id === item.id);
    if (action === 'download') return downloadKnowledge(item);
    if (action === 'edit') return handleManualEdit(idx, item);
    if (action === 'reparse') return confirmRebuildKnowledge(idx, item);
    if (action === 'cancel-parse') return confirmCancelParseKnowledge(item);
    if (action === 'move') return handleMoveKnowledge(item);
    if (action === 'delete') return confirmDeleteKnowledge(idx, item);
    if (action === 'view-trace') return handleViewTrace(idx, item);
    if (action === 'batch-manage') return handleEnterBatchFromCard(item);
  };

  // Clear selection on filter/tag/kb change to avoid acting on hidden items.
  watch(
    [selectedTagIds, docSearchKeyword, selectedFileType, selectedParseStatus, selectedSource, updatedTimeRange, kbId],
    () => {
      clearSelection();
    },
  );
  watch(selectedFolderPath, () => clearSelection());
  watch(selectedDocumentSort, () => clearSelection());

  // After cardList reloads: stable keys rely on correct indices for shift-range; clamp anchor index.
  watch(cardList, () => {
    const items = cardList.value || [];
    const n = items.length;
    if (lastSelectedIndex >= n) {
      lastSelectedIndex = n > 0 ? n - 1 : -1;
    }
    if (moreIndex.value >= n) {
      moreIndex.value = -1;
    }
    if (selectedIds.value.size === 0) return;
    const visible = new Set(items.map((i: KnowledgeCard) => i.id));
    for (const id of selectedIds.value) {
      if (!visible.has(id)) selectedIds.value.delete(id);
    }
  }, { deep: false });

  return {
    viewMode,
    selectedIds,
    batchMode,
    toggleBatchMode,
    handleBatchCancel,
    toggleSelectRow,
    onCardGridCheckboxChange,
    toggleSelectAll,
    clearSelection,
    handleEnterBatchFromCard,
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
    handleMoveKnowledge,
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
    isManualDraftKnowledge,
    handleManualEdit,
    confirmRebuildKnowledge,
    submitReparse,
    probeTraceAvailable,
  };
}