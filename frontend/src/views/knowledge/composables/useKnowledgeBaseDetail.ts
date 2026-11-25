import {
  computed,
  nextTick,
  onUnmounted,
  reactive,
  ref,
  watch,
  type ComputedRef,
} from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { useRoute, useRouter } from 'vue-router';
import { useUIStore } from '@/stores/ui';
import { useChatResourcesStore } from '@/stores/chatResources';
import { useOrganizationStore } from '@/stores/organization';
import { useEditorResourcesStore } from '@/stores/editorResources';
import useKnowledgeBase from '@/hooks/useKnowledgeBase';
import {
  batchQueryKnowledge,
  listKnowledgeTags,
  listKnowledgeFolders,
  getKnowledgeSpans,
  moveKnowledgeToFolder,
  renameKnowledgeFolder,
  type KnowledgeFolderTree,
} from '@/api/knowledge-base/index';
import { getWikiStats } from '@/api/wiki';
import type { ParserEngineInfo } from '@/api/system';
import {
  isKnowledgeParseInFlight,
  knowledgeNeedsStatusPolling,
  shouldRefreshWikiStatusAfterKnowledgePoll,
} from '../wikiStatusRefresh';
import { stalledMinutes, STALLED_POLL_INTERVAL_MS } from '@/utils/knowledgeProcessingStall';
import { knowledgeSpansPayloadHasTrace } from '@/utils/knowledgeTrace';
import {
  canMoveFolderTo,
  folderBreadcrumbs as buildFolderBreadcrumbs,
  folderPathExists as folderExistsInTree,
  isFilteringDocuments,
  ROOT_FOLDER_PATH,
} from '../folderTree';
import {
  DEFAULT_DOCUMENT_SORT,
  DOCUMENT_SORT_OPTIONS,
  getDocumentSortOption,
  getDocumentSortParams,
  type DocumentSortOption,
  type DocumentSortValue,
} from '../documentSorting';

export type KnowledgeCard = {
  id: string;
  knowledge_base_id?: string;
  parse_status: string;
  summary_status?: string;
  description?: string;
  file_name?: string;
  original_file_name?: string;
  display_name?: string;
  title?: string;
  type?: string;
  updated_at?: string;
  file_type?: string;
  file_size?: number;
  isMore?: boolean;
  metadata?: any;
  error_message?: string;
  tags?: Array<{ id: string; name: string; color?: string }>;
  last_activity_at?: string;
  // Minutes without progress while in flight; 0 unless it looks stuck.
  stalled_minutes?: number;
  // Server verdict on a quiet row: 'queued' (backlogged) or 'stalled'.
  stall_state?: string;
};

/**
 * KB detail data layer: knowledge base info, document list/pagination,
 * filters, tag catalog, folder tree, status polling and wiki indexing status.
 */
export function useKnowledgeBaseDetail(options: { kbId: ComputedRef<string> }) {
  const { kbId } = options;
  const uiStore = useUIStore();
  const chatResources = useChatResourcesStore();
  const orgStore = useOrganizationStore();
  const editorResources = useEditorResourcesStore();
  const route = useRoute();
  const router = useRouter();
  const { t } = useI18n();

  const kbInfo = ref<any>(null);
  const kbLoading = ref(false);
  const docListLoading = ref(true);
  const isFAQ = computed(() => (kbInfo.value?.type || '') === 'faq');
  const isWiki = computed(() => !!kbInfo.value?.indexing_strategy?.wiki_enabled);

  // Wiki 状态用于面包屑上的索引中指示。父组件自行拉取，避免依赖 WikiBrowser 挂载状态
  // （用户切到"文档" tab 时 WikiBrowser 会卸载，这里仍需持续反映后台索引进度）。
  const wikiStatus = ref<{ pendingTasks: number; isActive: boolean; pendingIssues: number }>({
    pendingTasks: 0,
    isActive: false,
    pendingIssues: 0,
  });
  const wikiIsIndexing = computed(() => wikiStatus.value.isActive || wikiStatus.value.pendingTasks > 0);
  const wikiIndexingTip = computed(() => {
    if (!wikiIsIndexing.value) return '';
    return t('knowledgeEditor.wikiBrowser.queueStatus', { count: wikiStatus.value.pendingTasks || 0 });
  });

  let wikiStatusTimer: ReturnType<typeof setInterval> | null = null;
  let wikiStatusProbeTimers: Array<ReturnType<typeof setTimeout>> = [];
  const stopWikiStatusPolling = () => {
    if (wikiStatusTimer) {
      clearInterval(wikiStatusTimer);
      wikiStatusTimer = null;
    }
  };
  const clearWikiStatusProbes = () => {
    wikiStatusProbeTimers.forEach(t => clearTimeout(t));
    wikiStatusProbeTimers = [];
  };
  const fetchWikiStatusOnce = async () => {
    if (!kbId.value || !isWiki.value) return;
    try {
      const res: any = await getWikiStats(kbId.value);
      const data = res?.data || res;
      if (!data) return;
      wikiStatus.value = {
        pendingTasks: data.pending_tasks || 0,
        isActive: !!data.is_active,
        pendingIssues: data.pending_issues || 0,
      };
      // 活跃时轮询，空闲时停掉定时器，避免无谓请求
      if (wikiIsIndexing.value) {
        if (!wikiStatusTimer) {
          wikiStatusTimer = setInterval(fetchWikiStatusOnce, 5000);
        }
      } else {
        stopWikiStatusPolling();
      }
    } catch (_) { /* ignore */ }
  };
  // 用户刚触发了一个上传 / reparse / URL 导入之类的动作后，后台通常要过
  // 一小段时间才会把 wiki 任务真正塞进队列；如果这时空闲轮询刚好停了，
  // 面包屑的"索引中"会延迟很久才亮起。所以这里安排几次退避重试，
  // 主动把面包屑的 loading 尽快点亮，一旦探测到任务就会走正常的 5s 轮询。
  const scheduleWikiStatusProbes = () => {
    if (!kbId.value || !isWiki.value) return;
    clearWikiStatusProbes();
    const delays = [500, 2000, 5000, 10000];
    delays.forEach(delay => {
      const timer = setTimeout(() => { fetchWikiStatusOnce(); }, delay);
      wikiStatusProbeTimers.push(timer);
    });
  };
  watch([kbId, isWiki], ([newKbId, newIsWiki]) => {
    stopWikiStatusPolling();
    clearWikiStatusProbes();
    wikiStatus.value = { pendingTasks: 0, isActive: false, pendingIssues: 0 };
    if (newKbId && newIsWiki) {
      fetchWikiStatusOnce();
    }
  }, { immediate: true });
  onUnmounted(() => {
    stopWikiStatusPolling();
    clearWikiStatusProbes();
  });

  const missingStorageEngine = computed(() => {
    if (!kbInfo.value || isFAQ.value) return false;
    // storage_backend_id is authoritative; storage_provider_config.provider is a
    // compatibility projection for older clients. Either being present means the
    // KB has a bound storage instance and uploads should not be blocked.
    if (kbInfo.value.storage_backend_id) return false;
    const spc = kbInfo.value.storage_provider_config;
    return !spc || !spc.provider;
  });
  const parserEngines = computed<ParserEngineInfo[]>(() => editorResources.parserEngines);

  const supportedFileTypes = computed<Set<string>>(() => {
    const engines = parserEngines.value;
    if (!engines.length) return new Set<string>();

    const rules: { file_types: string[]; engine: string }[] =
      kbInfo.value?.chunking_config?.parser_engine_rules || [];

    const ruleMap = new Map<string, string>();
    for (const r of rules) {
      for (const ft of r.file_types) ruleMap.set(ft, r.engine);
    }

    const available = new Set<string>();
    const availableEngineNames = new Set(
      engines.filter(e => e.Available !== false).map(e => e.Name),
    );

    for (const engine of engines) {
      for (const ft of engine.FileTypes || []) {
        if (available.has(ft)) continue;

        const explicitEngine = ruleMap.get(ft);
        if (explicitEngine) {
          if (availableEngineNames.has(explicitEngine)) available.add(ft);
        } else {
          if (engine.Available !== false) available.add(ft);
        }
      }
    }
    return available;
  });

  const acceptFileTypes = computed(() =>
    [...supportedFileTypes.value].map(t => '.' + t).join(','),
  );

  const unsupportedFileTypes = computed<string[]>(() => {
    const engines = parserEngines.value;
    if (!engines.length) return [];

    const allTypes = new Set<string>();
    for (const engine of engines) {
      for (const ft of engine.FileTypes || []) allTypes.add(ft);
    }

    const supported = supportedFileTypes.value;
    return [...allTypes].filter(ft => !supported.has(ft)).sort();
  });

  const goToParserSettings = () => {
    if (kbId.value) {
      uiStore.openKBSettings(kbId.value, 'parser');
    }
  };

  const knowledgeList = ref<Array<{ id: string; name: string; type?: string }>>([]);
  const {
    cardList,
    total,
    moreIndex,
    details,
    getKnowled,
    onVisibleChange: _onVisibleChange,
    getCardDetails,
    getfDetails,
  } = useKnowledgeBase(kbId.value);

  /** Per-knowledge cache: whether /spans has a real trace (see knowledgeSpansPayloadHasTrace). */
  const traceAvailableById = reactive<Record<string, boolean>>({});
  const traceProbeInflight = new Set<string>();

  function clearTraceAvailabilityCache() {
    for (const key of Object.keys(traceAvailableById)) {
      delete traceAvailableById[key];
    }
    traceProbeInflight.clear();
  }

  // Parse phases where the backend pipeline is still actively running
  // (primary parse OR post-process fan-out). Trace data exists and the
  // UI should treat the row as "in flight" rather than terminal.
  function isParseInFlight(status?: string): boolean {
    return isKnowledgeParseInFlight(status);
  }

  const onVisibleChange = (visible: boolean) => {
    _onVisibleChange(visible);
  };

  let knowledgeScroll = ref<any>();
  let page = 1;
  let pageSize = 35;
  let scrollLoading = false;
  const resetPage = () => { page = 1; scrollLoading = false; };
  const getPageSize = () => {
    const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
    const itemHeight = 148;
    const itemsInView = Math.floor(viewportHeight / itemHeight) * 5;
    pageSize = Math.max(35, itemsInView);
  };
  getPageSize();

  const handleScroll = () => {
    if (isFAQ.value) return;
    if (docListLoading.value) return;
    if (scrollLoading) return;
    const currentKbId = kbId.value;
    if (!currentKbId) return;
    const element = knowledgeScroll.value;
    if (element) {
      const pageNum = Math.ceil(total.value / pageSize);
      const { scrollTop, scrollHeight, clientHeight } = element;
      if (scrollTop + clientHeight >= scrollHeight - 10) {
        if (cardList.value.length < total.value && page < pageNum) {
          page++;
          scrollLoading = true;
          getKnowled({ page, page_size: pageSize, ...filterParams.value }, currentKbId).finally(() => {
            if (isCurrentKb(currentKbId)) {
              scrollLoading = false;
            }
          });
        }
      }
    }
  };

  // ── Filters ──────────────────────────────────────────────────────────
  const selectedTagIds = ref<string[]>([]);
  const selectedTagNames = ref<Record<string, string>>({});
  const tagList = ref<any[]>([]);
  // Keep selected tags reachable even when the search or loaded page changes.
  const filterTagOptions = computed(() => [
    ...tagList.value,
    ...selectedTagIds.value.filter(id => !tagList.value.some(tag => tag.id === id))
      .map(id => ({ id, name: selectedTagNames.value[id] || t('knowledgeBase.columnTag') })),
  ]);
  const tagLoading = ref(false);
  const tagSearchQuery = ref('');
  const TAG_PAGE_SIZE = 50;
  const tagPage = ref(1);
  const tagHasMore = ref(false);
  const tagLoadingMore = ref(false);
  const tagTotal = ref(0);
  let tagSearchDebounce: number | null = null;
  let docSearchDebounce: number | null = null;
  const docSearchKeyword = ref('');
  const selectedFileType = ref('');
  const fileTypeOptions = computed(() => [
    { label: t('knowledgeBase.allFileTypes'), value: '' },
    { label: 'PDF', value: 'pdf' },
    { label: 'DOCX', value: 'docx' },
    { label: 'DOC', value: 'doc' },
    { label: 'PPTX', value: 'pptx' },
    { label: 'PPT', value: 'ppt' },
    { label: 'EPUB', value: 'epub' },
    { label: 'MHTML', value: 'mhtml' },
    { label: 'TXT', value: 'txt' },
    { label: 'MD', value: 'md' },
    { label: 'URL', value: 'url' },
    { label: t('knowledgeBase.typeManual'), value: 'manual' },
    { label: 'MP3', value: 'mp3' },
    { label: 'WAV', value: 'wav' },
    { label: 'M4A', value: 'm4a' },
    { label: 'FLAC', value: 'flac' },
    { label: 'OGG', value: 'ogg' },
  ]);
  const selectedParseStatus = ref('');
  const parseStatusOptions = computed(() => [
    { label: t('knowledgeBase.allParseStatuses'), value: '' },
    { label: t('knowledgeBase.parseStatusPending'), value: 'pending' },
    { label: t('knowledgeBase.parseStatusProcessing'), value: 'processing' },
    { label: t('knowledgeBase.parseStatusCompleted'), value: 'completed' },
    { label: t('knowledgeBase.parseStatusFailed'), value: 'failed' },
    { label: t('knowledgeBase.parseStatusCancelled'), value: 'cancelled' },
    { label: t('knowledgeBase.parseStatusFinalizing'), value: 'finalizing' },
    { label: t('knowledgeBase.parseStatusDraft'), value: 'draft' },
  ]);
  const selectedSource = ref('');
  // Source filter combines ingestion channels and the "manual"/"url" virtual
  // sources that the backend routes onto the `type` column.
  const sourceOptions = computed(() => [
    { label: t('knowledgeBase.allSources'), value: '' },
    { label: t('knowledgeBase.sourceUpload'), value: 'web' },
    { label: t('knowledgeBase.sourceUrl'), value: 'url' },
    { label: t('knowledgeBase.sourceManual'), value: 'manual' },
    { label: t('knowledgeBase.sourceApi'), value: 'api' },
    { label: t('knowledgeBase.sourceBrowserExtension'), value: 'browser_extension' },
    { label: t('knowledgeBase.channelFeishu'), value: 'feishu' },
    { label: t('knowledgeBase.channelFeishuDrive'), value: 'feishu_drive' },
    { label: t('knowledgeBase.channelNotion'), value: 'notion' },
    { label: t('knowledgeBase.channelYuque'), value: 'yuque' },
    { label: t('knowledgeBase.channelConfluence'), value: 'confluence' },
    { label: t('knowledgeBase.channelGitLab'), value: 'gitlab' },
    { label: t('knowledgeBase.channelIma'), value: 'ima' },
    { label: t('knowledgeBase.channelWechat'), value: 'wechat' },
    { label: t('knowledgeBase.channelWecom'), value: 'wecom' },
    { label: t('knowledgeBase.channelDingtalk'), value: 'dingtalk' },
    { label: t('knowledgeBase.channelSlack'), value: 'slack' },
    { label: t('knowledgeBase.channelIm'), value: 'im' },
  ]);
  // Date range as [start, end] in "YYYY-MM-DD" form (t-date-range-picker default).
  const updatedTimeRange = ref<string[]>([]);
  const filtersExpanded = ref(false);
  const activeFilterCount = computed(() =>
    [selectedFileType.value, selectedParseStatus.value, selectedSource.value,
      updatedTimeRange.value?.some(Boolean)].filter(Boolean).length + selectedTagIds.value.length,
  );
  const clearDocumentFilters = () => {
    selectedFileType.value = '';
    selectedParseStatus.value = '';
    selectedSource.value = '';
    updatedTimeRange.value = [];
    handleTagFilterChange([]);
  };
  // Disable any date after today so users cannot filter into the future.
  const disableFutureDate = { after: new Date(new Date().setHours(23, 59, 59, 999)) };
  const documentSortPanelVisible = ref(false);
  const selectedDocumentSort = ref<DocumentSortValue>(DEFAULT_DOCUMENT_SORT);

  const documentSortOptionLabel = (option: DocumentSortOption) => {
    switch (option.labelKey) {
      case 'earliestUpdated':
        return t('knowledgeBase.sort.earliestUpdated');
      case 'newestCreated':
        return t('knowledgeBase.sort.newestCreated');
      case 'earliestCreated':
        return t('knowledgeBase.sort.earliestCreated');
      case 'nameAscending':
        return t('knowledgeBase.sort.nameAscending');
      case 'nameDescending':
        return t('knowledgeBase.sort.nameDescending');
      default:
        return t('knowledgeBase.sort.recentlyUpdated');
    }
  };

  const documentSortGroups = computed(() => [
    {
      key: 'updated_at',
      label: t('knowledgeBase.sort.updatedTime'),
      description: t('knowledgeBase.sort.updatedTimeDescription'),
      options: DOCUMENT_SORT_OPTIONS.filter((option) => option.sortBy === 'updated_at'),
    },
    {
      key: 'created_at',
      label: t('knowledgeBase.sort.createdTime'),
      description: t('knowledgeBase.sort.createdTimeDescription'),
      options: DOCUMENT_SORT_OPTIONS.filter((option) => option.sortBy === 'created_at'),
    },
    {
      key: 'file_name',
      label: t('knowledgeBase.sort.fileName'),
      description: t('knowledgeBase.sort.fileNameDescription'),
      options: DOCUMENT_SORT_OPTIONS.filter((option) => option.sortBy === 'file_name'),
    },
  ]);

  const activeDocumentSortLabel = computed(() => {
    const option = getDocumentSortOption(selectedDocumentSort.value);
    const group = documentSortGroups.value.find((item) => item.key === option.sortBy);
    return `${group?.label || ''} · ${documentSortOptionLabel(option)}`;
  });

  // ── Folder tree (documents uploaded as a folder keep their relative path) ──
  // The directory sidebar is now the sole folder navigation and starts expanded.
  const FOLDER_TREE_COLLAPSED_KEY = 'weknora.kbDirectorySidebarCollapsed';
  const readStoredFlag = (key: string, fallback = false) => {
    try {
      const raw = localStorage.getItem(key);
      return raw === null ? fallback : raw === 'true';
    } catch {
      return fallback;
    }
  };
  const writeStoredFlag = (key: string, value: boolean) => {
    try {
      localStorage.setItem(key, String(value));
    } catch {
      // Private-mode storage failures must not break navigation.
    }
  };
  const folderTree = ref<KnowledgeFolderTree | null>(null);
  const folderTreeLoading = ref(false);
  // The folder being browsed; ROOT_FOLDER_PATH ('') is the knowledge base top
  // level, a real node of the tree rather than a separate mode.
  const selectedFolderPath = ref<string>(ROOT_FOLDER_PATH);
  const folderTreeCollapsed = ref(readStoredFlag(FOLDER_TREE_COLLAPSED_KEY));
  const hasFolders = computed(() => (folderTree.value?.folders?.length ?? 0) > 0);
  // The folder column only earns its space once the knowledge base actually has
  // folders, so knowledge bases filled with single-file uploads look unchanged.
  const showFolderTree = computed(() => !isFAQ.value && hasFolders.value);
  // Browsing lists one folder's own contents; filtering searches its whole
  // subtree. There is no mode switch: the list follows what the user is doing.
  const isFiltering = computed(() =>
    isFilteringDocuments({
      keyword: docSearchKeyword.value,
      tagIds: selectedTagIds.value,
      fileType: selectedFileType.value,
      parseStatus: selectedParseStatus.value,
      source: selectedSource.value,
      timeRange: updatedTimeRange.value,
    }),
  );
  // A row's folder is worth showing only when the list can span folders.
  const showDocumentFolderPath = computed(() => hasFolders.value && isFiltering.value);
  const folderBreadcrumbs = computed(() => buildFolderBreadcrumbs(selectedFolderPath.value));

  const filterParams = computed(() => {
    const [start, end] = updatedTimeRange.value || [];
    return {
      tag_ids: selectedTagIds.value.length > 0 ? selectedTagIds.value.join(',') : undefined,
      keyword: docSearchKeyword.value ? docSearchKeyword.value.trim() : undefined,
      file_type: selectedFileType.value || undefined,
      parse_status: selectedParseStatus.value || undefined,
      source: selectedSource.value || undefined,
      start_time: start ? `${start} 00:00:00` : undefined,
      end_time: end ? `${end} 23:59:59` : undefined,
      ...getDocumentSortParams(selectedDocumentSort.value),
      folder_path: selectedFolderPath.value,
      // Filtering searches descendants; browsing shows this folder's documents.
      folder_recursive: isFiltering.value,
    };
  });

  const handleDocumentSortSelect = (value: DocumentSortValue) => {
    documentSortPanelVisible.value = false;
    if (selectedDocumentSort.value === value) return;

    selectedDocumentSort.value = value;
    resetPage();
    if (knowledgeScroll.value) {
      knowledgeScroll.value.scrollTop = 0;
    }
    if (kbId.value && !isFAQ.value) {
      loadKnowledgeFiles(kbId.value);
    }
  };
  const tagMap = computed<Record<string, any>>(() => {
    const map: Record<string, any> = {};
    tagList.value.forEach((tag) => {
      map[tag.id] = tag;
    });
    return map;
  });

  const loadKnowledgeFiles = (kbIdValue: string): Promise<void> => {
    if (!kbIdValue) return Promise.resolve();
    if (!isFAQ.value) {
      docListLoading.value = true;
    }
    return getKnowled(
      {
        page: 1,
        page_size: pageSize,
        ...filterParams.value,
      },
      kbIdValue,
    ).finally(() => {
      if (isCurrentKb(kbIdValue) && !isFAQ.value) {
        docListLoading.value = false;
      }
    });
  };

  const isCurrentKb = (targetKbId: string) => targetKbId === kbId.value;

  const loadFolderTree = async (kbIdValue: string) => {
    if (!kbIdValue || isFAQ.value) {
      folderTree.value = null;
      return;
    }
    folderTreeLoading.value = true;
    try {
      const res: any = await listKnowledgeFolders(kbIdValue);
      if (!isCurrentKb(kbIdValue)) return;
      folderTree.value = (res?.data as KnowledgeFolderTree) || null;
      // A folder can disappear (its last document was deleted or moved); fall
      // back to the root instead of leaving an empty, unreachable view.
      if (!folderExistsInTree(folderTree.value?.folders || [], selectedFolderPath.value)) {
        selectedFolderPath.value = ROOT_FOLDER_PATH;
      }
    } catch (error) {
      if (!isCurrentKb(kbIdValue)) return;
      console.error('Failed to load knowledge folders', error);
      folderTree.value = null;
    } finally {
      if (isCurrentKb(kbIdValue)) {
        folderTreeLoading.value = false;
      }
    }
  };

  const handleFolderSelect = (path: string) => {
    if (selectedFolderPath.value === path) return;
    selectedFolderPath.value = path;
  };

  // ── Re-filing documents and renaming folders ──
  // folder_path is display-only, so both operations are a plain column update:
  // nothing is re-parsed, re-chunked or re-embedded.

  // Flat folder list shared by every "move to folder" picker.
  const folderOptions = computed(() => {
    const result: Array<{ path: string; name: string; depth: number }> = [];
    const walk = (nodes: KnowledgeFolderTree['folders'], depth: number) => {
      nodes.forEach((node) => {
        result.push({ path: node.path, name: node.name, depth });
        walk(node.children || [], depth + 1);
      });
    };
    walk(folderTree.value?.folders || [], 0);
    return result;
  });

  const handleFolderRename = async ({ from, to }: { from: string; to: string }) => {
    if (!kbId.value || !to || from === to) return;
    if (!canMoveFolderTo(from, to)) {
      MessagePlugin.warning(t('knowledgeBase.folderTree.renameInvalid'));
      return;
    }
    try {
      const res: any = await renameKnowledgeFolder(kbId.value, from, to);
      const movedCount = res?.data?.moved_count ?? 0;
      if (movedCount === 0) {
        MessagePlugin.warning(t('knowledgeBase.folderTree.renameFailed'));
        await loadFolderTree(kbId.value);
        return;
      }
      MessagePlugin.success(t('knowledgeBase.folderTree.renameSuccess'));
      // Follow the folder to its new path so the user stays where they were.
      if (selectedFolderPath.value === from) {
        selectedFolderPath.value = to;
      } else if (selectedFolderPath.value.startsWith(`${from}/`)) {
        selectedFolderPath.value = to + selectedFolderPath.value.slice(from.length);
      }
      resetPage();
      await loadKnowledgeFiles(kbId.value);
      await loadFolderTree(kbId.value);
    } catch (error: any) {
      MessagePlugin.error(error?.message || t('knowledgeBase.folderTree.renameFailed'));
    }
  };

  const handleFolderTreeCollapsedChange = (value: boolean) => {
    folderTreeCollapsed.value = value;
    writeStoredFlag(FOLDER_TREE_COLLAPSED_KEY, value);
  };

  const loadTags = async (kbIdValue: string, reset = false) => {
    if (!kbIdValue) {
      tagList.value = [];
      tagTotal.value = 0;
      tagHasMore.value = false;
      tagPage.value = 1;
      return;
    }

    if (reset) {
      tagPage.value = 1;
      tagList.value = [];
      tagTotal.value = 0;
      tagHasMore.value = false;
    } else if (tagLoading.value || tagLoadingMore.value) {
      return;
    }

    const currentPage = tagPage.value || 1;
    tagLoading.value = currentPage === 1;
    tagLoadingMore.value = currentPage > 1;

    try {
      const res: any = await listKnowledgeTags(kbIdValue, {
        page: currentPage,
        page_size: TAG_PAGE_SIZE,
        keyword: tagSearchQuery.value || undefined,
      });
      if (!isCurrentKb(kbIdValue)) return;

      const pageData = (res?.data || {}) as {
        data?: any[];
        total?: number;
      };
      const pageTags = (pageData.data || []).map((tag: any) => ({
        ...tag,
        id: String(tag.id),
      }));

      if (currentPage === 1) {
        tagList.value = pageTags;
      } else {
        tagList.value = [...tagList.value, ...pageTags];
      }

      tagTotal.value = pageData.total || tagList.value.length;
      tagHasMore.value = tagList.value.length < tagTotal.value;
      if (tagHasMore.value) {
        tagPage.value = currentPage + 1;
      }
    } catch (error) {
      if (!isCurrentKb(kbIdValue)) return;
      console.error('Failed to load tags', error);
    } finally {
      if (isCurrentKb(kbIdValue)) {
        tagLoading.value = false;
        tagLoadingMore.value = false;
      }
    }
  };

  const handleTagFilterChange = (tagIds: string[]) => {
    selectedTagNames.value = Object.fromEntries(tagIds.map(id => [
      id, tagMap.value[id]?.name || selectedTagNames.value[id] || t('knowledgeBase.columnTag'),
    ]));
    selectedTagIds.value = tagIds;
    // 同步更新 store 中的 selectedTagIds，供 menu.vue 上传时使用
    uiStore.clearSelectedTagIds();
    tagIds.forEach(id => uiStore.toggleSelectedTagId(id));
    resetPage();
  };

  const onTagCatalogChanged = (payload?: { deletedTagId?: string }) => {
    void loadTags(kbId.value, true);
    if (payload?.deletedTagId) {
      handleTagFilterChange(selectedTagIds.value.filter(id => id !== payload.deletedTagId));
    }
    resetPage();
    void loadKnowledgeFiles(kbId.value);
  };

  const loadKnowledgeBaseInfo = async (targetKbId: string, force = false) => {
    if (!targetKbId) {
      kbInfo.value = null;
      cardList.value = [];
      total.value = 0;
      return;
    }
    kbLoading.value = true;
    try {
      const data = await chatResources.fetchKnowledgeBaseById(targetKbId, force);
      if (!isCurrentKb(targetKbId)) return;

      kbInfo.value = data;
      selectedTagIds.value = [];
      selectedTagNames.value = {};
      uiStore.clearSelectedTagIds();
      // 重置store中的标签选择状态，避免上传文档时自动带上之前选择的标签
      uiStore.clearSelectedTagIds();
      if (!isFAQ.value) {
        loadKnowledgeFiles(targetKbId);
        void loadFolderTree(targetKbId);
      } else {
        cardList.value = [];
        total.value = 0;
        folderTree.value = null;
      }
      loadTags(targetKbId, true);
    } catch (error) {
      if (!isCurrentKb(targetKbId)) return;

      console.error('Failed to load knowledge base info:', error);
      kbInfo.value = null;
      cardList.value = [];
      total.value = 0;
    } finally {
      if (isCurrentKb(targetKbId)) {
        kbLoading.value = false;
      }
    }
  };

  const loadKnowledgeList = async () => {
    try {
      await chatResources.ensureKnowledgeBases();
      const myKbs = chatResources.rawKnowledgeBases.map((item: any) => ({
        id: String(item.id),
        name: item.name,
        type: item.type || 'document',
      }));

      // Also include shared knowledge bases from orgStore
      const sharedKbs = (orgStore.sharedKnowledgeBases || [])
        .filter(s => s.knowledge_base != null)
        .map(s => ({
          id: String(s.knowledge_base.id),
          name: s.knowledge_base.name,
          type: s.knowledge_base.type || 'document',
        }));

      // Merge and deduplicate by id (my KBs take precedence)
      const myKbIds = new Set(myKbs.map(kb => kb.id));
      const uniqueSharedKbs = sharedKbs.filter(kb => !myKbIds.has(kb.id));

      knowledgeList.value = [...myKbs, ...uniqueSharedKbs];
    } catch (error) {
      console.error('Failed to load knowledge list:', error);
    }
  };

  watch(() => kbId.value, (newKbId, oldKbId) => {
    if (!newKbId) {
      kbInfo.value = null;
      cardList.value = [];
      total.value = 0;
      return;
    }
    if (newKbId === oldKbId && kbInfo.value) return;

    if (newKbId !== oldKbId) {
      clearTraceAvailabilityCache();
      cardList.value = [];
      total.value = 0;
      docListLoading.value = true;
      resetPage();
      tagSearchQuery.value = '';
      tagPage.value = 1;
      uiStore.clearSelectedTagIds();
      folderTree.value = null;
      selectedFolderPath.value = ROOT_FOLDER_PATH;
    }
    loadKnowledgeBaseInfo(newKbId);
  }, { immediate: true });

  watch(selectedTagIds, () => {
    if (kbId.value) {
      loadKnowledgeFiles(kbId.value);
    }
  }, { deep: true });

  watch(tagSearchQuery, (newVal, oldVal) => {
    if (newVal === oldVal) return;
    if (tagSearchDebounce) {
      window.clearTimeout(tagSearchDebounce);
    }
    tagSearchDebounce = window.setTimeout(() => {
      if (kbId.value) {
        loadTags(kbId.value, true);
      }
    }, 300);
  });

  // 监听文档搜索关键词变化
  watch(docSearchKeyword, (newVal, oldVal) => {
    if (newVal === oldVal) return;
    if (docSearchDebounce) {
      window.clearTimeout(docSearchDebounce);
    }
    docSearchDebounce = window.setTimeout(() => {
      if (kbId.value) {
        resetPage();
        loadKnowledgeFiles(kbId.value);
      }
    }, 300);
  });

  // 监听文件类型筛选变化
  watch(selectedFileType, (newVal, oldVal) => {
    if (newVal === oldVal) return;
    if (kbId.value) {
      resetPage();
      loadKnowledgeFiles(kbId.value);
    }
  });

  // 监听解析状态/来源/更新时间范围筛选变化（与文件类型行为一致）
  watch([selectedParseStatus, selectedSource, updatedTimeRange], () => {
    if (kbId.value) {
      resetPage();
      loadKnowledgeFiles(kbId.value);
    }
  }, { deep: true });

  // 切换目录只改变列表范围，行为与其他筛选一致。浏览态与筛选态之间的切换由各筛选项
  // 自身的 watcher 触发刷新，这里不重复请求。
  watch(selectedFolderPath, () => {
    if (!kbId.value || isFAQ.value) return;
    resetPage();
    loadKnowledgeFiles(kbId.value);
  });

  // 监听文件上传事件
  const handleFileUploaded = (event: CustomEvent) => {
    const uploadedKbId = event.detail.kbId;
    console.log('接收到文件上传事件，上传的知识库ID:', uploadedKbId, '当前知识库ID:', kbId.value);
    if (uploadedKbId && uploadedKbId === kbId.value && !isFAQ.value) {
      // Mid-batch refreshes (settled === false) only bring new rows and folders
      // in, and are skipped once the user scrolled past the first page because
      // reloading resets the list to it. The batch's final event refreshes fully.
      const midBatch = event.detail.settled === false;
      if (midBatch && page > 1) return;
      console.log('匹配当前知识库，开始刷新文件列表');
      // 如果上传的文件属于当前知识库，使用 loadKnowledgeFiles 刷新文件列表
      resetPage(); // Reset page counter when reloading files after upload
      loadKnowledgeFiles(uploadedKbId);
      void loadFolderTree(uploadedKbId);
      if (midBatch) return;
      loadTags(uploadedKbId);
      // 启动几次探测，尽快让面包屑的"索引中"亮起。
      scheduleWikiStatusProbes();
    }
  };

  // ── Status polling ───────────────────────────────────────────────────
  let timeout: ReturnType<typeof setTimeout> | null = null;
  // IDs submitted for async batch reparse; hold optimistic pending until the worker updates DB.
  const pendingReparseAck = ref<Set<string>>(new Set());

  // needsStatusPolling decides whether a card row is still "in flight"
  // enough that the doc list should keep refreshing it. Keep in sync with
  // the backend lifecycle: pending / processing are the primary parse
  // phase, finalizing is the post-process fan-out (summary / question /
  // graph extract still running), and a `completed` row whose summary
  // hasn't landed yet keeps polling so the description fills in.
  const needsStatusPolling = (item: KnowledgeCard) => {
    return knowledgeNeedsStatusPolling(item);
  };

  // Back off once every in-flight row looks stuck: nothing is moving, and a
  // page left open on it should not keep hammering the batch endpoint.
  const pollDelayFor = (items: KnowledgeCard[]) =>
    items.length > 0 && items.every(item => (item.stalled_minutes ?? 0) > 0)
      ? STALLED_POLL_INTERVAL_MS
      : 1500;

  const updateStatus = (analyzeList: KnowledgeCard[], delay = 1500) => {
    if (timeout !== null) {
      clearTimeout(timeout);
      timeout = null;
    }
    if (!analyzeList.length) return;

    let query = ``;
    for (let i = 0; i < analyzeList.length; i++) {
      query += `ids=${analyzeList[i].id}&`;
    }
    timeout = setTimeout(() => {
      batchQueryKnowledge(query).then((result: any) => {
        let shouldRefreshWikiStatus = false;
        if (result.success && result.data) {
          (result.data as KnowledgeCard[]).forEach((item: KnowledgeCard) => {
            const index = cardList.value.findIndex(card => card.id == item.id);
            if (index == -1) return;

            let parseStatus = item.parse_status;
            if (pendingReparseAck.value.has(item.id)) {
              if (isParseInFlight(item.parse_status)) {
                pendingReparseAck.value.delete(item.id);
              } else {
                parseStatus = 'pending';
              }
            }

            const card = cardList.value[index];
            card.last_activity_at = item.last_activity_at;
            card.stall_state = item.stall_state;
            card.stalled_minutes = stalledMinutes({ parse_status: parseStatus, last_activity_at: item.last_activity_at });

            if (cardList.value[index].parse_status !== parseStatus ||
              cardList.value[index].summary_status !== item.summary_status ||
              cardList.value[index].description !== item.description) {
              shouldRefreshWikiStatus ||= shouldRefreshWikiStatusAfterKnowledgePoll(
                cardList.value[index],
                { ...item, parse_status: parseStatus },
              );

              // Always update the card data
              cardList.value[index].parse_status = parseStatus;
              cardList.value[index].summary_status = item.summary_status;
              cardList.value[index].description = item.description;
              delete traceAvailableById[item.id];
            }
          });
          // A requested row the batch no longer returns is gone (deleted
          // elsewhere, replaced by a data-source sync). Left in the list it
          // kept its in-flight status, so its spinner and this poll never ended.
          const returnedIds = new Set((result.data as KnowledgeCard[]).map(item => item.id));
          const goneIds = new Set(analyzeList.map(item => item.id).filter(id => !returnedIds.has(id)));
          if (goneIds.size > 0) {
            const before = cardList.value.length;
            cardList.value = cardList.value.filter(card => !goneIds.has(card.id));
            total.value = Math.max(0, total.value - (before - cardList.value.length));
          }
        }
        if (shouldRefreshWikiStatus) {
          void fetchWikiStatusOnce();
        }
        // If there are no changes, the watch won't trigger, so we must manually poll again
        // Even if there are changes, we can manually poll again just to be safe.
        // The watch will clear this timeout if it triggers.
        const stillPending = cardList.value.filter(needsStatusPolling);
        if (stillPending.length > 0) {
          updateStatus(stillPending, pollDelayFor(stillPending));
        }
      }).catch((_err) => {
        // 错误处理
        const stillPending = cardList.value.filter(needsStatusPolling);
        if (stillPending.length > 0) {
          updateStatus(stillPending, pollDelayFor(stillPending));
        }
      });
    }, delay);
  };

  watch(() => cardList.value, (newValue) => {
    if (isFAQ.value) return;
    docListLoading.value = false;

    let analyzeList = [];
    // Filter items that need polling: parsing in progress OR summary generation in progress
    analyzeList = newValue.filter(needsStatusPolling);
    if (timeout !== null) {
      clearTimeout(timeout);
      timeout = null;
    }
    if (analyzeList.length) {
      // The deep watch refires as stalled_minutes ticks; keep the backoff.
      updateStatus(analyzeList, pollDelayFor(analyzeList));
    }
  }, { deep: true });

  const probeTraceAvailable = async (item: KnowledgeCard) => {
    const id = item.id;
    if (!id || traceProbeInflight.has(id)) return;
    if (isParseInFlight(item.parse_status)) {
      traceAvailableById[id] = true;
      return;
    }
    if (Object.prototype.hasOwnProperty.call(traceAvailableById, id)) return;
    traceProbeInflight.add(id);
    try {
      const res: any = await getKnowledgeSpans(id);
      traceAvailableById[id] = !!(res?.success && knowledgeSpansPayloadHasTrace(res.data));
    } catch {
      traceAvailableById[id] = false;
    } finally {
      traceProbeInflight.delete(id);
    }
  };

  onUnmounted(() => {
    if (timeout !== null) {
      clearTimeout(timeout);
      timeout = null;
    }
  });

  return {
    // KB info
    kbInfo,
    kbLoading,
    docListLoading,
    isFAQ,
    isWiki,
    wikiStatus,
    wikiIsIndexing,
    wikiIndexingTip,
    fetchWikiStatusOnce,
    scheduleWikiStatusProbes,
    missingStorageEngine,
    parserEngines,
    supportedFileTypes,
    acceptFileTypes,
    unsupportedFileTypes,
    goToParserSettings,
    knowledgeList,
    // Documents
    cardList,
    total,
    moreIndex,
    details,
    getKnowled,
    onVisibleChange,
    getCardDetails,
    getfDetails,
    knowledgeScroll,
    handleScroll,
    resetPage,
    loadKnowledgeFiles,
    loadKnowledgeBaseInfo,
    loadKnowledgeList,
    handleFileUploaded,
    // Filters
    selectedTagIds,
    selectedTagNames,
    tagList,
    filterTagOptions,
    tagLoading,
    tagSearchQuery,
    tagHasMore,
    tagLoadingMore,
    tagTotal,
    loadTags,
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
    documentSortOptionLabel,
    documentSortGroups,
    activeDocumentSortLabel,
    handleDocumentSortSelect,
    // Folder tree
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
    loadFolderTree,
    handleFolderTreeCollapsedChange,
    // Polling/trace
    pendingReparseAck,
    traceAvailableById,
    clearTraceAvailabilityCache,
    isParseInFlight,
    probeTraceAvailable,
  };
}