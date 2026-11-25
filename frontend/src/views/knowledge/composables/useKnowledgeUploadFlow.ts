import { computed, nextTick, ref, watch, type Ref } from 'vue';
import { MessagePlugin } from 'tdesign-vue-next';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useUIStore } from '@/stores/ui';
import { useUploadConfirmStore, type UploadConfirmResult } from '@/stores/uploadConfirm';
import { useUploadTasksStore } from '@/stores/uploadTasks';
import { createKnowledgeFromURL } from '@/api/knowledge-base/index';
import { MAX_FILE_SIZE_BYTES, MAX_FILE_SIZE_MB } from '@/utils';
import { buildUploadFileName, ROOT_FOLDER_PATH } from '../folderTree';
import type { KnowledgeProcessOverrides } from '@/types/knowledgeProcess';

type UploadSourceHandle = {
  openUrlDialog?: () => void;
  openFileDialog?: () => boolean;
};

/**
 * Upload entry flow for the KB detail page: the toolbar's upload source
 * dropdown, the ?upload=1 delivery panel, drag feedback and hand-off to the
 * global upload queue / URL import endpoint.
 */
export function useKnowledgeUploadFlow(options: {
  kbId: Ref<string>;
  kbInfo: Ref<any>;
  kbLoading: Ref<boolean>;
  isFAQ: Ref<boolean>;
  canUpload: Ref<boolean>;
  isWiki: Ref<boolean>;
  activeKbTab: Ref<any>;
  selectedFolderPath: Ref<string>;
  folderOptions: Ref<Array<{ path: string; name: string; depth: number }>>;
  selectedTagIds: Ref<string[]>;
  supportedFileTypes: Ref<Set<string>>;
  acceptFileTypes: Ref<string>;
  missingStorageEngine: Ref<boolean>;
  toolbarRef: Ref<UploadSourceHandle | null>;
  resetPage: () => void;
  loadKnowledgeFiles: (kbId: string) => Promise<void>;
  loadFolderTree: (kbId: string) => Promise<void>;
  loadTags: (kbId: string, reset?: boolean) => Promise<void>;
  scheduleWikiStatusProbes: () => void;
  manualEditorSuccess: (payload: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) => void;
}) {
  const {
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
    supportedFileTypes,
    acceptFileTypes,
    missingStorageEngine,
    toolbarRef,
    resetPage,
    loadKnowledgeFiles,
    loadFolderTree,
    loadTags,
    scheduleWikiStatusProbes,
    manualEditorSuccess,
  } = options;
  const route = useRoute();
  const router = useRouter();
  const { t } = useI18n();
  const uiStore = useUIStore();
  const uploadConfirmStore = useUploadConfirmStore();
  const uploadTasksStore = useUploadTasksStore();

  // ── 首页 / 库卡片「上传资料」交接（?upload=1） ─────────────────────────
  // 从 /home 带参数进来时，等当前库信息与权限都就绪再直接打开文件选择器；
  // 打开后把 query 清掉，刷新页面不会重复弹窗。
  let uploadAutoOpenKbId: string | null = null;
  // 浏览器不允许「页面加载后无用户手势地弹系统文件选择器」，所以从首页带
  // ?upload=1 进来时先落在自建的投递面板里，由用户点「选择文件」或拖拽完成。
  const uploadEntryVisible = ref(false);

  const clearUploadQuery = () => {
    if (route.query.upload !== '1') return;
    const query = { ...route.query };
    delete (query as any).upload;
    void router.replace({ path: route.path, query, hash: route.hash });
  };

  // 同一个页面再次收到 ?upload=1（例如从首页重复点同一张卡片）也要能再次弹窗。
  watch(
    () => route.query.upload,
    (value) => {
      if (value === '1') uploadAutoOpenKbId = null;
    },
  );

  const ensureDocumentKbReady = () => {
    if (isFAQ.value) {
      MessagePlugin.warning(t('knowledgeBase.operationNotSupportedForType'));
      return false;
    }
    if (!kbId.value) {
      MessagePlugin.warning(t('knowledgeEditor.messages.missingId'));
      return false;
    }
    if (!kbInfo.value || !kbInfo.value.summary_model_id) {
      MessagePlugin.warning(t('knowledgeBase.notInitialized'));
      return false;
    }
    // Embedding model only required when RAG indexing is enabled
    const strategy = (kbInfo.value as any).indexing_strategy;
    const needsEmbedding = !strategy || strategy.vector_enabled || strategy.keyword_enabled;
    if (needsEmbedding && !kbInfo.value.embedding_model_id) {
      MessagePlugin.warning(t('knowledgeBase.notInitialized'));
      return false;
    }
    if (missingStorageEngine.value) {
      MessagePlugin.warning(t('knowledgeBase.missingStorageEngineUpload'));
      return false;
    }
    return true;
  };

  // Hands the batch to the global upload queue, which runs the transfers,
  // reports progress in its floating panel and refreshes this page as files land.
  const enqueueUploads = (
    files: File[],
    uploadOptions: {
      processConfig?: KnowledgeProcessOverrides;
      tagIds?: string[];
      /** Destination folder confirmed in the upload dialog; '' is the root. */
      targetFolder?: string;
    } = {},
  ) => {
    const targetKbId = kbId.value;
    if (!targetKbId || files.length === 0) return;
    const targetFolder = uploadOptions.targetFolder || ROOT_FOLDER_PATH;
    uploadTasksStore.enqueue({
      kbId: targetKbId,
      kbName: kbInfo.value?.name || '',
      targetFolder,
      tagIds: uploadOptions.tagIds,
      processConfig: uploadOptions.processConfig,
      uploads: files.map(file => ({ file, fileName: buildUploadFileName(file, targetFolder) })),
    });
    // 浮层已经给出进度，这里补一句「确实收下了」的即时确认，避免确认弹窗
    // 关闭后到第一批进度出现之间有一段没有反馈的空窗。
    MessagePlugin.success(`已加入上传队列，共 ${files.length} 个文件`);
  };

  // 传输失败时把文件名直接说出来：浮层可能被收起，用户仍需要知道是哪个文件失败。
  const notifiedFailedUploads = new Set<string>();
  watch(
    () => uploadTasksStore.items.map(item => `${item.id}:${item.transfer}`).join('|'),
    () => {
      for (const item of uploadTasksStore.items) {
        if (item.transfer !== 'failed' || notifiedFailedUploads.has(item.id)) continue;
        const batch = uploadTasksStore.batches.find(entry => entry.id === item.batchId);
        if (!batch || batch.kbId !== kbId.value) continue;
        notifiedFailedUploads.add(item.id);
        MessagePlugin.error(`「${item.name}」上传失败${item.error ? `：${item.error}` : ''}`);
      }
    },
  );

  const executeUrlImport = async (
    url: string,
    processConfig?: KnowledgeProcessOverrides,
    tagIds?: string[],
  ) => {
    const targetKbId = kbId.value;
    if (!targetKbId) {
      MessagePlugin.error(t('error.missingKbId'));
      return;
    }

    const tagIdsToUpload = tagIds && tagIds.length > 0 ? [...tagIds] : undefined;
    try {
      const responseData: any = await createKnowledgeFromURL(targetKbId, {
        url,
        tag_ids: tagIdsToUpload,
        process_config: processConfig,
      });
      window.dispatchEvent(new CustomEvent('knowledgeFileUploaded', {
        detail: { kbId: targetKbId },
      }));
      const isSuccess = responseData?.success || responseData?.code === 200 || responseData?.status === 'success' || (!responseData?.error && responseData);
      if (isSuccess) {
        MessagePlugin.success(t('knowledgeBase.urlImportSuccess'));
      } else {
        let errorMessage = t('knowledgeBase.urlImportFailed');
        if (responseData?.error?.message) {
          errorMessage = responseData.error.message;
        } else if (responseData?.message) {
          errorMessage = responseData.message;
        }
        if (responseData?.code === 'duplicate_url' || responseData?.error?.code === 'duplicate_url') {
          errorMessage = t('knowledgeBase.urlExists');
        }
        MessagePlugin.error(errorMessage);
      }
    } catch (error: any) {
      let errorMessage = error?.error?.message || error?.message || t('knowledgeBase.urlImportFailed');
      if (error?.code === 'duplicate_url') {
        errorMessage = t('knowledgeBase.urlExists');
      }
      MessagePlugin.error(errorMessage);
    }
  };

  const handleUploadConfirmResult = async (result: UploadConfirmResult) => {
    if (result.mode === 'manual') {
      return;
    }

    const files = result.files || [];
    const urls = result.urls || [];
    const processConfig = result.processConfig;
    const tagIds = result.tagIds || [];

    if (files.length > 0) {
      enqueueUploads(files, {
        processConfig,
        tagIds,
        targetFolder: result.targetFolder || ROOT_FOLDER_PATH,
      });
    }

    for (const url of urls) {
      await executeUrlImport(url, processConfig, tagIds);
    }
  };

  const openUploadConfirmDialog = async (files: File[], urls: string[] = []) => {
    if (!kbInfo.value) return;
    if (files.length === 0 && urls.length === 0) return;
    try {
      const result = await uploadConfirmStore.open({
        mode: 'file',
        kbInfo: kbInfo.value,
        tagIds: [...selectedTagIds.value],
        files,
        urls,
        acceptFileTypes: acceptFileTypes.value,
        supportedFileTypes: [...supportedFileTypes.value],
        // Pre-fill the destination with the folder being browsed; the dialog shows
        // it and lets the user pick another folder (or the root) before confirming.
        targetFolder: selectedFolderPath.value,
        folderOptions: folderOptions.value,
      });
      await handleUploadConfirmResult(result);
    } catch {
      // cancelled
    }
  };

  const handleUploadSourceFiles = (files: File[]) => {
    if (!ensureDocumentKbReady()) return;
    if (files.length === 0) return;
    // 50MB 是前端容器与服务端的实际单文件上限（外层 nginx 200m 不是瓶颈）。
    // 超限文件在选完/拖入的这一刻就拦下，避免传完后才收到一段 413 HTML。
    const oversized = files.filter(file => file.size > MAX_FILE_SIZE_BYTES);
    const accepted = files.filter(file => file.size <= MAX_FILE_SIZE_BYTES);
    if (oversized.length > 0) {
      const names = oversized.slice(0, 3).map(file => file.name).join('、');
      MessagePlugin.warning(
        `已跳过 ${oversized.length} 个超过 ${MAX_FILE_SIZE_MB}MB 的文件：${names}${oversized.length > 3 ? ' 等' : ''}`,
      );
    }
    if (accepted.length === 0) return;
    openUploadConfirmDialog(accepted);
  };

  const handleUploadSourceUrl = (url: string) => {
    if (!ensureDocumentKbReady()) return;
    openUploadConfirmDialog([], [url]);
  };

  const handleManualCreate = () => {
    if (!ensureDocumentKbReady()) return;
    uiStore.openManualEditor({
      mode: 'create',
      kbId: kbId.value,
      status: 'draft',
      onSuccess: manualEditorSuccess,
    });
  };

  const maybeAutoOpenUploadPicker = async () => {
    if (route.query.upload !== '1') return;
    // 库信息就绪后先判权限：只读成员从首页点「上传」时给出明确提示，
    // 并把参数清掉，避免地址栏一直挂着 ?upload=1 却什么也没发生。
    if (!kbLoading.value && kbInfo.value && !isFAQ.value && !canUpload.value) {
      if (uploadAutoOpenKbId !== kbId.value) {
        uploadAutoOpenKbId = kbId.value;
        MessagePlugin.warning('你没有该知识库的上传权限，请联系空间管理员');
      }
      clearUploadQuery();
      return;
    }
    if (kbLoading.value || !kbInfo.value || isFAQ.value || !canUpload.value) return;
    if (uploadAutoOpenKbId === kbId.value) return;
    uploadAutoOpenKbId = kbId.value;
    // 上传入口只存在于「文档」tab 的工具栏里：wiki 库带参数进来时先切回文档。
    if (isWiki.value) activeKbTab.value = 'documents';
    for (let attempt = 0; attempt < 3 && !toolbarRef.value; attempt += 1) {
      await nextTick();
    }
    if (!toolbarRef.value) {
      // 入口尚未渲染（极少见）；释放标记，等下次依赖变化再试。
      uploadAutoOpenKbId = null;
      return;
    }
    uploadEntryVisible.value = true;
    clearUploadQuery();
  };

  // 投递面板：选择文件或拖入文件后关闭面板，接既有上传确认流程。
  const onUploadEntryDrop = (event: DragEvent) => {
    const files = Array.from(event.dataTransfer?.files || []);
    uploadEntryVisible.value = false;
    if (files.length) handleUploadSourceFiles(files);
  };
  const openUploadEntryPicker = () => {
    if (toolbarRef.value?.openFileDialog?.()) {
      uploadEntryVisible.value = false;
    }
  };

  watch(
    [() => route.query.upload, kbLoading, canUpload, isFAQ, () => kbInfo.value?.id],
    () => { void maybeAutoOpenUploadPicker(); },
    { immediate: true },
  );

  // ── 拖拽上传的可视反馈 ──────────────────────────────────────────────
  // 真正的落盘由 platform shell 的全局 drop 处理（派发 weknora:knowledge-file-drop，
  // 本页 handleKnowledgeFileDrop 复用上传确认流程）；这里只负责遮罩与文案。
  const fileDragDepth = ref(0);
  const fileDragActive = computed(() => fileDragDepth.value > 0 && canUpload.value && !isFAQ.value);
  const dragCarriesFiles = (event: DragEvent) => {
    const types = event.dataTransfer?.types;
    return !!types && Array.from(types).includes('Files');
  };
  const handleKbDragEnter = (event: DragEvent) => {
    if (!dragCarriesFiles(event) || !canUpload.value) return;
    fileDragDepth.value += 1;
  };
  const handleKbDragOver = (event: DragEvent) => {
    if (!dragCarriesFiles(event) || !canUpload.value) return;
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy';
    if (fileDragDepth.value === 0) fileDragDepth.value = 1;
  };
  const handleKbDragLeave = (event: DragEvent) => {
    if (!dragCarriesFiles(event)) return;
    fileDragDepth.value = Math.max(0, fileDragDepth.value - 1);
  };
  const handleKbDrop = () => {
    fileDragDepth.value = 0;
  };

  return {
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
    loadKnowledgeFiles,
    loadFolderTree,
    loadTags,
    resetPage,
    scheduleWikiStatusProbes,
  };
}