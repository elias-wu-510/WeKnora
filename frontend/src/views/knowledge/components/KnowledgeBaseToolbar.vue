<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import KbUploadSourceDropdown from './KbUploadSourceDropdown.vue';

const { t } = useI18n();

const docSearchKeyword = defineModel<string>('docSearchKeyword', { required: true });
const filtersExpanded = defineModel<boolean>('filtersExpanded', { required: true });
const selectedFileType = defineModel<string>('selectedFileType', { required: true });
const selectedParseStatus = defineModel<string>('selectedParseStatus', { required: true });
const selectedSource = defineModel<string>('selectedSource', { required: true });
const updatedTimeRange = defineModel<string[]>('updatedTimeRange', { required: true });
const tagSearchQuery = defineModel<string>('tagSearchQuery', { required: true });
const viewMode = defineModel<'grid' | 'list'>('viewMode', { required: true });
const documentSortPanelVisible = defineModel<boolean>('documentSortPanelVisible', { required: true });

defineProps<{
  kbId: string;
  kbInfo: any;
  showFolderTree: boolean;
  folderTreeCollapsed: boolean;
  folderBreadcrumbs: Array<{ path: string; name: string }>;
  isFiltering: boolean;
  total: number;
  docListLoading: boolean;
  activeFilterCount: number;
  fileTypeOptions: Array<{ label: string; value: string }>;
  parseStatusOptions: Array<{ label: string; value: string }>;
  sourceOptions: Array<{ label: string; value: string }>;
  disableFutureDate: { after: Date };
  tagLoading: boolean;
  filterTagOptions: Array<{ id: string; name: string }>;
  selectedTagIds: string[];
  tagHasMore: boolean;
  tagLoadingMore: boolean;
  batchMode: boolean;
  batchDeleting: boolean;
  batchReparsing: boolean;
  batchTagging: boolean;
  batchDownloading: boolean;
  documentSortGroups: Array<{
    key: string;
    label: string;
    description: string;
    options: Array<{ value: string; labelKey: string }>;
  }>;
  selectedDocumentSort: string;
  activeDocumentSortLabel: string;
  canDownloadKnowledge: boolean;
  canMutateKnowledge: boolean;
  canUpload: boolean;
  itemCount: number;
  acceptFileTypes: string;
  supportedFileTypes: string[];
}>();

const emit = defineEmits<{
  (e: 'folder-select', path: string): void;
  (e: 'expand-folder-tree'): void;
  (e: 'search-submit'): void;
  (e: 'clear-filters'): void;
  (e: 'tag-filter-change', tagIds: string[]): void;
  (e: 'load-more-tags'): void;
  (e: 'toggle-batch'): void;
  (e: 'sort-select', value: any): void;
  (e: 'upload-files', files: File[]): void;
  (e: 'upload-url', url: string): void;
  (e: 'manual-create'): void;
}>();

const uploadSourceRef = ref<InstanceType<typeof KbUploadSourceDropdown> | null>(null);

const documentSortOptionLabel = (option: { labelKey: string }) => {
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

defineExpose({
  openUrlDialog: () => uploadSourceRef.value?.openUrlDialog(),
  openFileDialog: () => uploadSourceRef.value?.openFileDialog(),
});
</script>

<template>
  <div class="doc-filter-bar">
    <nav class="doc-folder-path" :aria-label="$t('knowledgeBase.folderTree.title')">
      <button v-if="showFolderTree && folderTreeCollapsed" type="button" class="doc-folder-path__tree-toggle"
        :aria-expanded="false" :title="$t('knowledgeBase.folderTree.expand')"
        :aria-label="$t('knowledgeBase.folderTree.expand')"
        @click="emit('expand-folder-tree')">
        <t-icon name="view-list" size="16px" />
      </button>
      <button v-if="folderBreadcrumbs.length" type="button" class="doc-folder-path__crumb" :title="kbInfo?.name" @click="emit('folder-select', '')">
        {{ kbInfo?.name }}
      </button>
      <span v-else class="doc-folder-path__crumb is-current" :title="kbInfo?.name" aria-current="page">{{ kbInfo?.name }}</span>
      <template v-for="(crumb, index) in folderBreadcrumbs" :key="crumb.path">
        <t-icon name="chevron-right" class="doc-folder-path__sep" />
        <span v-if="index === folderBreadcrumbs.length - 1" class="doc-folder-path__crumb is-current" :title="crumb.name" aria-current="page">{{ crumb.name }}</span>
        <button v-else type="button" class="doc-folder-path__crumb" :title="crumb.name" @click="emit('folder-select', crumb.path)">{{ crumb.name }}</button>
      </template>
      <span v-if="!docListLoading" class="doc-folder-path__count">{{ $t(isFiltering ? 'knowledgeBase.folderTree.filteredCount' : 'knowledgeBase.documentCount', { count: total }) }}</span>
      <t-tooltip v-if="showFolderTree && isFiltering" :content="$t('knowledgeBase.folderTree.searchingSubtree')">
        <t-icon name="info-circle" class="doc-folder-path__sep" />
      </t-tooltip>
    </nav>
    <div class="doc-filter-bar__trailing">
      <t-input v-model.trim="docSearchKeyword" :placeholder="$t('knowledgeBase.docSearchPlaceholder')"
        :aria-label="$t('knowledgeBase.docSearchPlaceholder')" clearable class="doc-search-input" @clear="emit('search-submit')"
        @enter="emit('search-submit')">
        <template #prefix-icon>
          <t-icon name="search" size="16px" />
        </template>
      </t-input>
      <t-popup v-model:visible="filtersExpanded" trigger="click" placement="bottom-right"
        overlay-class-name="document-filter-popup" :overlay-inner-style="{ padding: 0 }">
        <button type="button" class="doc-filter-toggle" :class="{ active: filtersExpanded || activeFilterCount > 0 }"
          :aria-expanded="filtersExpanded" aria-controls="document-filters">
          <t-icon name="filter" size="16px" />
          {{ $t('knowledgeBase.filters') }}
          <span v-if="activeFilterCount" class="doc-filter-count">{{ activeFilterCount }}</span>
        </button>
        <template #content>
          <section id="document-filters" class="doc-filter-panel" :aria-label="$t('knowledgeBase.filters')">
            <header class="doc-filter-panel__header">
              <strong>{{ $t('knowledgeBase.filters') }}</strong>
              <button type="button" :disabled="!activeFilterCount" @click="emit('clear-filters')">{{ $t('knowledgeBase.clearFilters') }}</button>
            </header>
            <div class="doc-filter-panel__fields">
              <div class="doc-filter-field">
                <span>{{ $t('knowledgeBase.fileTypeFilter') }}</span>
                <t-select v-model="selectedFileType" :options="fileTypeOptions" :placeholder="$t('knowledgeBase.fileTypeFilter')" clearable />
              </div>
              <div class="doc-filter-field">
                <span>{{ $t('knowledgeBase.parseStatusFilter') }}</span>
                <t-select v-model="selectedParseStatus" :options="parseStatusOptions" :placeholder="$t('knowledgeBase.parseStatusFilter')" clearable />
              </div>
              <div class="doc-filter-field">
                <span>{{ $t('knowledgeBase.sourceFilter') }}</span>
                <t-select v-model="selectedSource" :options="sourceOptions" :placeholder="$t('knowledgeBase.sourceFilter')" clearable />
              </div>
              <div class="doc-filter-field">
                <span>{{ $t('knowledgeBase.columnUpdatedAt') }}</span>
                <t-date-range-picker v-model="updatedTimeRange"
                  :placeholder="[$t('knowledgeBase.updatedTimeFrom'), $t('knowledgeBase.updatedTimeTo')]"
                  :disable-date="disableFutureDate" clearable allow-input />
              </div>
            </div>
            <div class="doc-filter-tags">
              <div class="doc-filter-tags__heading">
                <span>{{ $t('knowledgeBase.columnTag') }}<span v-if="selectedTagIds.length" class="doc-filter-tags__count">{{ selectedTagIds.length }}</span></span>
              </div>
              <t-input v-model.trim="tagSearchQuery" :placeholder="$t('knowledgeBase.tagSearchPlaceholder')" clearable>
                <template #prefix-icon><t-icon name="search" size="14px" /></template>
              </t-input>
              <div class="doc-filter-tags__list">
                <t-loading v-if="tagLoading && !filterTagOptions.length" size="small" />
                <t-checkbox v-for="tag in filterTagOptions" :key="tag.id" class="doc-filter-tag" :title="tag.name"
                  :checked="selectedTagIds.includes(tag.id)"
                  @change="(checked: boolean) => emit('tag-filter-change', checked ? [...selectedTagIds, tag.id] : selectedTagIds.filter(id => id !== tag.id))">
                  <span>{{ tag.name }}</span>
                </t-checkbox>
                <span v-if="!tagLoading && !filterTagOptions.length" class="doc-filter-tags__empty">{{ $t(tagSearchQuery ? 'knowledgeBase.tagEmptyResult' : 'knowledgeBase.noTags') }}</span>
              </div>
              <t-button v-if="tagHasMore" variant="text" size="small" :loading="tagLoadingMore" @click="emit('load-more-tags')">{{ $t('tenant.loadMore') }}</t-button>
            </div>
          </section>
        </template>
      </t-popup>
      <button v-if="viewMode === 'grid' && (canDownloadKnowledge || canMutateKnowledge) && itemCount"
        type="button" class="doc-filter-toggle doc-batch-toggle" :class="{ active: batchMode }" :aria-pressed="batchMode"
        :disabled="batchDeleting || batchReparsing || batchTagging || batchDownloading"
        @click="emit('toggle-batch')">
        <t-icon :name="batchMode ? 'close' : 'check-rectangle'" size="16px" />
        {{ $t(batchMode ? 'common.cancel' : 'menu.batchManage') }}
      </button>
      <t-popup v-model:visible="documentSortPanelVisible" trigger="click" placement="bottom-right"
        overlay-class-name="document-sort-popup" :overlay-inner-style="{ padding: 0 }">
        <template #content>
          <div class="document-sort-panel" role="menu" :aria-label="$t('knowledgeBase.sort.title')">
            <section v-for="group in documentSortGroups" :key="group.key" class="document-sort-group">
              <div class="document-sort-group__heading">
                <div class="document-sort-group__label">{{ group.label }}</div>
                <div class="document-sort-group__description">{{ group.description }}</div>
              </div>
              <div class="document-sort-group__options">
                <button v-for="option in group.options" :key="option.value" type="button"
                  class="document-sort-option"
                  :class="{ active: selectedDocumentSort === option.value }"
                  role="menuitemradio" :aria-checked="selectedDocumentSort === option.value"
                  @click.stop="emit('sort-select', option.value)">
                  <span>{{ documentSortOptionLabel(option) }}</span>
                  <t-icon v-if="selectedDocumentSort === option.value" name="check" size="14px" />
                </button>
              </div>
            </section>
          </div>
        </template>
        <button type="button" class="doc-sort-trigger" :class="{ active: documentSortPanelVisible }"
          :title="`${$t('knowledgeBase.sort.title')}: ${activeDocumentSortLabel}`"
          :aria-label="`${$t('knowledgeBase.sort.title')}: ${activeDocumentSortLabel}`">
          <t-icon name="filter-sort" size="16px" />
          <span class="doc-sort-trigger__label">
            {{ $t('knowledgeBase.sort.title') }} · {{ activeDocumentSortLabel }}
          </span>
          <t-icon name="chevron-down" size="14px" class="doc-sort-trigger__caret"
            :class="{ open: documentSortPanelVisible }" />
          </button>
        </t-popup>
      <div class="doc-view-toggle" role="group" :aria-label="$t('knowledgeBase.viewModeToggle')">
        <t-tooltip :content="$t('knowledgeBase.viewModeGrid')" placement="top">
          <button type="button" class="doc-view-toggle-btn" :class="{ active: viewMode === 'grid' }"
            :aria-label="$t('knowledgeBase.viewModeGrid')" @click="viewMode = 'grid'" :aria-pressed="viewMode === 'grid'">
            <t-icon name="view-module" size="16px" />
          </button>
        </t-tooltip>
        <t-tooltip :content="$t('knowledgeBase.viewModeList')" placement="top">
          <button type="button" class="doc-view-toggle-btn" :class="{ active: viewMode === 'list' }"
            :aria-label="$t('knowledgeBase.viewModeList')" @click="viewMode = 'list'" :aria-pressed="viewMode === 'list'">
            <t-icon name="view-list" size="16px" />
          </button>
        </t-tooltip>
      </div>
      <div v-if="canUpload" class="doc-filter-actions">
        <KbUploadSourceDropdown ref="uploadSourceRef" :accept-file-types="acceptFileTypes"
          :supported-file-types="supportedFileTypes" include-manual trigger-icon="add" :trigger-label="t('knowledgeBase.addDocument')"
          trigger-class="content-bar-icon-btn" data-guide="kb-detail-add-doc"
          :tooltip="t('knowledgeBase.addDocument')" placement="bottom-right" @files="(files: File[]) => emit('upload-files', files)"
          @url="(url: string) => emit('upload-url', url)" @manual="emit('manual-create')" />
      </div>
    </div>
  </div>
</template>

<style scoped lang="less">
// One navigation row; filters open in a single anchored panel.
.doc-filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-shrink: 0;
  padding: 0 0 16px;
  border-bottom: 1px solid var(--td-component-stroke);
  &__trailing { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .doc-search-input { width: 220px; min-width: 100px; }
  :deep(.doc-search-input .t-input) {
    background: transparent;
    border-color: var(--td-component-stroke);
    border-radius: var(--app-radius-md);
    font-size: var(--app-text-md);
  }
  .doc-filter-actions :deep(.content-bar-icon-btn) {
    height: 32px;
    padding: 0 12px;
    border-radius: var(--app-radius-md);
    background: var(--td-brand-color);
    color: var(--td-text-color-anti);
    &:hover { background: var(--td-brand-color-hover); }
  }
}
.doc-folder-path {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  min-height: 32px;
  line-height: 20px;
  flex-wrap: wrap;
  &__tree-toggle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    align-self: center;
    gap: 6px;
    height: 32px;
    padding: 0 10px;
    font: inherit;
    font-size: var(--app-text-md);
    border: 1px solid var(--td-component-stroke);
    border-radius: var(--app-radius-md);
    background: var(--td-bg-color-container);
    color: var(--td-text-color-secondary);
    cursor: pointer;
    &:hover { background: var(--td-bg-color-container-hover); color: var(--td-text-color-primary); }
  }
  &__crumb {
    max-width: 160px;
    padding: 6px 2px;
    border: 0;
    background: transparent;
    color: var(--td-text-color-secondary);
    font: inherit;
    font-size: var(--app-text-md);
    line-height: 20px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    cursor: pointer;
    &:is(button):hover { color: var(--td-brand-color); }
    &.is-current { color: var(--td-text-color-primary); font-weight: 600; cursor: default; }
  }
  &__count { padding: 6px 0; line-height: 20px; margin-left: 6px; color: var(--td-text-color-placeholder); font-size: var(--app-text-sm); font-variant-numeric: tabular-nums; white-space: nowrap; }
  &__sep, &__scope { align-self: center; flex-shrink: 0; color: var(--td-text-color-placeholder); font-size: var(--app-text-sm); }
}
.doc-filter-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 32px;
  padding: 0 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md);
  background: transparent;
  color: var(--td-text-color-secondary);
  font: inherit;
  font-size: var(--app-text-md);
  white-space: nowrap;
  cursor: pointer;
  &:hover, &.active { background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-primary); }
}
.doc-filter-count {
  display: inline-flex; align-items: center; justify-content: center; min-width: 17px; height: 17px;
  padding: 0 2px; color: var(--td-brand-color); font-weight: 600;
  font-size: var(--app-text-xs); font-variant-numeric: tabular-nums;
}
.doc-view-toggle {
  display: inline-flex;
  align-items: center;
  background: var(--td-bg-color-secondarycontainer);
  border: 1px solid transparent;
  border-radius: var(--app-radius-md);
  padding: 2px;
  .doc-view-toggle-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 26px;
    padding: 0;
    border: 0;
    border-radius: var(--app-radius-xs);
    background: transparent;
    color: var(--td-text-color-placeholder);
    cursor: pointer;
    &:hover { color: var(--td-text-color-primary); }
    &.active { background: var(--td-bg-color-container); color: var(--td-brand-color); box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08); }
  }
}
.doc-filter-panel {
  width: 340px;
  max-width: calc(100vw - 32px);
  max-height: min(600px, 80vh);
  overflow-y: auto;
  padding: 16px;
  box-sizing: border-box;
  color: var(--td-text-color-primary);
  font-size: var(--app-text-md);
  button { font: inherit; cursor: pointer; }
  &__header {
    display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px;
    strong { font-size: var(--app-text-base); font-weight: 600; }
    button { border: 0; padding: 0; background: transparent; color: var(--td-text-color-secondary); font-size: var(--app-text-sm); }
    button:disabled { color: var(--td-text-color-disabled); cursor: default; }
  }
  &__fields { display: flex; flex-direction: column; gap: 10px; }
  .doc-filter-field {
    display: grid; grid-template-columns: 72px minmax(0, 1fr); align-items: center; gap: 8px;
    > span { color: var(--td-text-color-secondary); font-size: var(--app-text-sm); }
    :deep(.t-date-range-picker) { width: 100%; }
  }
}

.document-sort-panel {
  width: 330px;
  max-width: min(330px, calc(100vw - 32px));
  padding: 6px;
  box-sizing: border-box;
  color: var(--td-text-color-primary);
}

.document-sort-group {
  padding: 7px 6px 8px;

  & + & {
    border-top: 1px solid var(--td-component-stroke);
  }

  &__heading {
    padding: 0 4px 6px;
  }

  &__label {
    font-size: var(--app-text-md);
    line-height: 20px;
    font-weight: 600;
  }

  &__description {
    margin-top: 1px;
    color: var(--td-text-color-secondary);
    font-size: var(--app-text-xs);
    line-height: 17px;
  }

  &__options {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 4px;
  }
}

.document-sort-option {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  min-width: 0;
  height: 32px;
  padding: 0 10px;
  border: 0;
  border-radius: var(--app-radius-sm);
  background: transparent;
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
  font-size: var(--app-text-md);
  cursor: pointer;

  &:hover {
    background: var(--td-bg-color-secondarycontainer);
  }

  &.active {
    background: var(--td-brand-color-light);
    color: var(--td-brand-color);
    font-weight: 500;
  }
}
.doc-filter-tags {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--td-component-stroke);
  &__heading {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;
    button { padding: 0; border: 0; background: transparent; color: var(--td-text-color-secondary); font-size: var(--app-text-sm); }
  }
  &__count { margin-left: 6px; color: var(--td-text-color-placeholder); font-variant-numeric: tabular-nums; }
  &__list { display: flex; flex-direction: column; gap: 2px; max-height: 168px; overflow-y: auto; margin-top: 8px; }
  &__empty { color: var(--td-text-color-placeholder); padding: 8px 0; }
}
.doc-filter-tag {
  display: flex; align-items: center; flex-shrink: 0; width: 100%; min-height: 28px; margin: 0;
  padding: 4px 6px; box-sizing: border-box; border-radius: var(--app-radius-xs);
  &:hover { background: var(--td-bg-color-container-hover); }
  :deep(.t-checkbox__label) { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  :deep(.t-checkbox__input) { flex-shrink: 0; }
}
.doc-batch-toggle {
  &:disabled { cursor: not-allowed; opacity: 0.5; }
  &.active { color: var(--app-selection-text); border-color: var(--td-component-border); background: var(--app-selection-bg); }
}
.doc-filter-bar button:focus-visible, .doc-filter-panel button:focus-visible {
  outline: 2px solid var(--app-focus-border); outline-offset: 2px;
}
@container doc-card-area (max-width: 780px) {
  .doc-filter-bar { flex-wrap: wrap; gap: 12px; }
  .doc-filter-bar__trailing { width: 100%; }
  .doc-filter-bar .doc-search-input { flex: 1; width: auto; }
}

.doc-sort-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  max-width: 220px;
  height: 32px;
  padding: 0 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md);
  background: transparent;
  color: var(--td-text-color-secondary);
  font: inherit;
  font-size: var(--app-text-md);
  cursor: pointer;

  &:hover,
  &.active {
    color: var(--td-text-color-primary);
    background: var(--td-bg-color-secondarycontainer);
  }

  &__label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__caret {
    flex-shrink: 0;
    transition: transform 0.2s ease;

    &.open { transform: rotate(180deg); }
  }
}

@container doc-card-area (max-width: 540px) {
  .doc-filter-bar__trailing { flex-wrap: wrap; }
  .doc-filter-bar .doc-search-input { flex: 1 0 100%; }
}
</style>