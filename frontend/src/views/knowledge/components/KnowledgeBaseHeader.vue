<script setup lang="ts">
import KBInfoPopover from '@/components/KBInfoPopover.vue';
import KBSwitcherDropdown from '@/components/KBSwitcherDropdown.vue';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

defineProps<{
  kbInfo: any;
  kbId: string;
  knowledgeList: Array<{ id: string; name: string; type?: string }>;
  kbViewTabs: Array<{ key: string; icon: string; label: string; tip: string; indexing?: boolean }>;
  shownKbTab: string;
  canManage: boolean;
  unsupportedFileTypes: string[];
  missingStorageEngine: boolean;
  supportedFileTypes: string[];
  isLiteMode: boolean;
}>();

const emit = defineEmits<{
  (e: 'navigate-kb-list'): void;
  (e: 'navigate-current-kb'): void;
  (e: 'select-kb', id: string): void;
  (e: 'select-tab', key: string): void;
  (e: 'open-members'): void;
  (e: 'open-settings'): void;
  (e: 'go-parser-settings'): void;
}>();
</script>

<template>
  <div class="document-header">
    <div class="document-header-title">
      <div class="document-title-row">
        <h2 class="document-breadcrumb">
          <button type="button" class="breadcrumb-link" @click="emit('navigate-kb-list')">
            {{ $t('menu.knowledgeBase') }}
          </button>
          <t-icon name="chevron-right" class="breadcrumb-separator" />
          <KBSwitcherDropdown v-if="knowledgeList.length" :kb-list="knowledgeList" :current-kb-id="kbId"
            @select="(id: string) => emit('select-kb', id)">
            <button type="button" class="breadcrumb-link dropdown" :disabled="!kbId">
              <template v-if="!kbInfo">
                <t-skeleton animation="gradient" :row-col="[{ width: '120px', height: '20px' }]" />
              </template>
              <template v-else>
                <span>{{ kbInfo.name }}</span>
                <t-icon name="chevron-down" />
              </template>
            </button>
          </KBSwitcherDropdown>
          <button v-else type="button" class="breadcrumb-link" :disabled="!kbId" @click="emit('navigate-current-kb')">
            <template v-if="!kbInfo">
              <t-skeleton animation="gradient" :row-col="[{ width: '120px', height: '20px' }]" />
            </template>
            <template v-else>
              {{ kbInfo.name }}
            </template>
          </button>
          <t-icon name="chevron-right" class="breadcrumb-separator" />
          <div class="kb-view-tabs" role="tablist" :aria-label="$t('knowledgeEditor.wikiBrowser.viewTabs')">
            <t-tooltip v-for="tab in kbViewTabs" :key="tab.key" :content="tab.tip" placement="bottom">
              <button type="button" role="tab" class="kb-view-tab"
                :class="{ active: shownKbTab === tab.key, indexing: tab.indexing }"
                :aria-selected="shownKbTab === tab.key" @click="emit('select-tab', tab.key)">
                <t-loading v-if="tab.indexing" size="small" class="kb-view-tab__indicator" />
                <t-icon v-else :name="tab.icon" size="16px" />
                <span>{{ tab.label }}</span>
              </button>
            </t-tooltip>
          </div>
        </h2>
        <!-- 标题行右侧的动作锚点：聚拢"信息"和"设置"两个圆形按钮。 -->
        <div class="kb-title-actions">
          <KBInfoPopover v-if="kbInfo && !isLiteMode" :kb-info="kbInfo"
            :supported-file-types="[...supportedFileTypes]" />
          <t-tooltip v-if="kbInfo" content="成员与权限" placement="top">
            <button type="button" class="kb-settings-button kb-members-button" aria-label="成员与权限"
              @click="emit('open-members')">
              <t-icon name="usergroup" size="16px" />
            </button>
          </t-tooltip>
          <t-tooltip v-if="canManage" :content="$t('knowledgeBase.settings')" placement="top">
            <button type="button" class="kb-settings-button" :aria-label="$t('knowledgeBase.settings')" :disabled="!kbId" @click="emit('open-settings')">
              <t-icon name="setting" size="16px" />
            </button>
          </t-tooltip>
        </div>
      </div>
      <p v-if="kbInfo?.description" class="document-subtitle">{{ kbInfo.description }}</p>
      <p v-if="unsupportedFileTypes.length" class="parser-hint" @click="emit('go-parser-settings')">
        <t-icon name="info-circle" class="parser-hint-icon" />
        <span>{{ $t('knowledgeBase.unsupportedTypesHint', {
          types: unsupportedFileTypes.map((type: string) => '.' + type).join('、'),
        })
          }}</span>
        <span class="parser-hint-link">{{ $t('knowledgeBase.goToParserSettings') }} →</span>
      </p>
      <p v-if="missingStorageEngine" class="storage-engine-warning" @click="emit('open-settings')">
        <t-icon name="info-circle" class="warning-icon" />
        <span>{{ $t('knowledgeBase.missingStorageEngine') }}</span>
        <span class="warning-link">{{ $t('knowledgeBase.goToStorageSettings') }} →</span>
      </p>
    </div>
  </div>
</template>

<style scoped lang="less">
// View switch (文档 / Wiki / 图谱 / 画廊): a segmented control after the
// breadcrumb, drawn like the documents tab's view toggle so the four views
// read as siblings rather than as another breadcrumb level.
.kb-view-tabs {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  margin-left: 2px;
  padding: 3px;
  border-radius: var(--app-radius-lg);
  background: var(--td-bg-color-secondarycontainer);
}

.kb-view-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 12px;
  border: 0;
  border-radius: var(--app-radius-md);
  background: transparent;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  font-size: var(--app-text-base);
  font-weight: 500;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  transition: color var(--app-motion-fast) ease, background-color var(--app-motion-fast) ease;

  &:hover {
    color: var(--td-text-color-primary);
  }

  &.active {
    background: var(--td-bg-color-container);
    color: var(--td-brand-color);
    font-weight: 600;
    box-shadow: 0 1px 3px rgb(0 0 0 / 8%);
  }

  &:focus-visible {
    outline: 2px solid var(--app-focus-border);
    outline-offset: 1px;
  }

  &__indicator {
    display: inline-flex;
    color: var(--td-brand-color);
  }
}

// Header 样式（无底部分割线，留更多空间给下方内容区）
.document-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  flex-shrink: 0;

  .document-header-title {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .document-title-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .kb-title-actions {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
    margin-left: 4px;
  }

  .document-breadcrumb {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    row-gap: 8px;
    margin: 0;
    font-size: var(--app-text-3xl);
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .breadcrumb-link {
    border: none;
    background: transparent;
    padding: 4px 8px;
    margin: -4px -8px;
    font: inherit;
    color: var(--td-text-color-secondary);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    white-space: nowrap;
    border-radius: var(--app-radius-sm);
    transition: all var(--app-motion-instant) ease;

    &:hover:not(:disabled) {
      color: var(--td-success-color);
      background: var(--td-bg-color-container);
    }

    &:disabled {
      cursor: not-allowed;
      color: var(--td-text-color-placeholder);
    }

    &.dropdown {
      padding-right: 6px;

      :deep(.t-icon) {
        font-size: var(--app-text-base);
        transition: transform var(--app-motion-instant) ease;
      }

      &:hover:not(:disabled) {
        :deep(.t-icon) {
          transform: translateY(1px);
        }
      }
    }
  }

  .breadcrumb-separator {
    font-size: var(--app-text-base);
    color: var(--td-text-color-placeholder);
  }

  .breadcrumb-current {
    color: var(--td-text-color-primary);
    font-weight: 600;
  }

  h2 {
    margin: 0;
    color: var(--td-text-color-primary);
    font-family: var(--app-font-family);
    font-size: var(--app-text-4xl);
    font-weight: 600;
    line-height: 32px;
  }

  .document-subtitle {
    margin: 0;
    color: var(--td-text-color-placeholder);
    font-family: var(--app-font-family);
    font-size: var(--app-text-base);
    font-weight: 400;
    line-height: 20px;
  }

  .parser-hint {
    display: flex;
    align-items: center;
    gap: 4px;
    margin: 2px 0 0;
    color: var(--td-warning-color);
    font-size: var(--app-text-sm);
    line-height: 1.4;
    cursor: pointer;
    transition: color var(--app-motion-fast) ease;

    &:hover {
      color: var(--td-warning-color-active);

      .parser-hint-link {
        text-decoration: underline;
      }
    }

    .parser-hint-icon {
      font-size: var(--app-text-sm);
      flex-shrink: 0;
    }

    .parser-hint-link {
      color: var(--td-brand-color);
      margin-left: 2px;
      white-space: nowrap;
    }
  }

  .storage-engine-warning {
    display: flex;
    align-items: center;
    gap: 4px;
    margin: 2px 0 0;
    color: var(--td-warning-color);
    font-size: var(--app-text-sm);
    line-height: 1.4;
    cursor: pointer;
    transition: color var(--app-motion-fast) ease;

    &:hover {
      color: var(--td-warning-color-active);

      .warning-link {
        text-decoration: underline;
      }
    }

    .warning-icon {
      font-size: var(--app-text-sm);
      flex-shrink: 0;
    }

    .warning-link {
      color: var(--td-brand-color);
      margin-left: 2px;
      white-space: nowrap;
    }
  }
}

.kb-settings-button {
  width: 30px;
  height: 30px;
  border: none;
  border-radius: 50%;
  background: var(--td-bg-color-secondarycontainer);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  transition: all var(--app-motion-base) ease;
  padding: 0;

  &:hover:not(:disabled) {
    background: var(--td-success-color-light);
    color: var(--td-brand-color);
    box-shadow: none;
  }

  &:disabled {
    cursor: not-allowed;
    opacity: 0.4;
  }

  :deep(.t-icon) {
    font-size: var(--app-text-2xl);
  }
}
</style>